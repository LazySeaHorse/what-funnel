"""Offline evaluation harness for the cascade router and the grounded-answer (KB) stage.

Runs the real code path (local pre-checks, router prompt/schema, deterministic gates, and the KB
stage with its citation and groundedness gates) over labelled JSONL sets against any
OpenAI-compatible chat-completions endpoint, and reports precision/recall per route, canned-reply
precision, false-canned rate, handoff and injection recall, silent drops, KB answer quality and
hallucinations, a confusion matrix and cost/latency. Results are reported per slice: "seen" (the
original menus, whose not_for notes were written next to the cases, so near-miss scores are an upper
bound), "heldout" (unseen menus with generic notes) and "kb" (knowledge fixtures).

    EVAL_BASE_URL=https://.../v1 EVAL_API_KEY=... EVAL_MODEL=gemini-3.1-flash-lite \\
        python -m eval.harness            # all slices, default files in this directory

Run from services/ai-answer-svc with PYTHONPATH=.:../../packages/python (see README).

Files in this directory:
    cases.jsonl           "seen" slice (menus.json: dental, bakery, bikes)
    cases_heldout.jsonl   "heldout" slice (menus_heldout.json: salon, gym, plumber; some cases reuse seen menus)
    cases_kb.jsonl        "kb" slice (kb.json holds knowledge concepts per menu)
    kb.json               {menu_id: [{"title", "body_text"}]}; ALL concepts of a menu are given to the KB
                          stage (no embedding retrieval in this harness)

Case format (one JSON object per line):
    id                      unique case id (unique across all files)
    menu                    key into the menus (the account's FAQ menu)
    history                 optional prior turns: [{"role": "customer"|"agent", "text": "..."}]
    bubbles                 1-3 customer messages of one conversation (or "text" for a single one)
    first_message           optional bool; default: true when there is no history
    expected_route          greeting | faq | kb | handoff | ignore
    expected_faq_id         FAQ key in the menu (only for expected_route == faq), else null
    expected_handoff_reason none | spam | needs_human | prompt_injection | other
    also_ok_routes          optional extra routes that are acceptable (judgement calls)
    kind                    free-form label (paraphrase, typo, near_miss, follow_up, ...)
    kb_expect               optional {"answerable": bool, "must_include": [...], "must_not_include": [...]}
                            for cases that exercise the KB stage; strings are matched case-insensitively

Scoring: a prediction is lenient-correct when its route equals expected_route or is in also_ok_routes
(and, for faq, the FAQ matches); strict-correct when it equals expected_route. The predicted route is
the FINAL outcome after the deterministic gates: "faq" = a canned reply would be sent verbatim; "kb" =
a grounded answer was produced (otherwise the KB stage hands off); "ignore" = a message was dropped
silently (only possible for a bare acknowledgement); a soft review flag counts as "handoff".
"""

from __future__ import annotations

import argparse
import asyncio
import hashlib
import json
import math
import os
import sys
import uuid
from collections import Counter
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any, Optional

from control import transcript_within_byte_budget
from kb_rag import answer_from_sources, concept_sources, faq_sources
from router import (
    PROMPT_VERSION,
    RouterResult,
    apply_gates,
    build_menu,
    local_precheck,
    run_router,
)

ROUTES = ("greeting", "faq", "kb", "handoff", "ignore")
ESCALATION_REASONS = ("needs_human", "prompt_injection", "spam")
DEFAULT_MODEL = "gemini-3.1-flash-lite"
HERE = Path(__file__).parent
DEFAULT_CASE_FILES = ("cases.jsonl", "cases_heldout.jsonl", "cases_kb.jsonl")
DEFAULT_MENU_FILES = ("menus.json", "menus_heldout.json")
SLICE_BY_STEM = {"cases": "seen", "cases_heldout": "heldout", "cases_kb": "kb"}


# ---------------------------------------------------------------------------
# Loading
# ---------------------------------------------------------------------------

@dataclass
class Case:
    id: str
    menu: str
    bubbles: list[str]
    expected_route: str
    expected_faq_id: Optional[str] = None
    expected_handoff_reason: str = "none"
    also_ok_routes: tuple[str, ...] = ()
    history: list[dict] = field(default_factory=list)
    first_message: bool = True
    kind: str = ""
    slice: str = "seen"
    kb_expect: Optional[dict] = None


def load_cases(path: str | Path, slice_name: Optional[str] = None) -> list[Case]:
    path = Path(path)
    slice_name = slice_name or SLICE_BY_STEM.get(path.stem, path.stem)
    cases: list[Case] = []
    seen: set[str] = set()
    for line_number, line in enumerate(path.read_text().splitlines(), start=1):
        line = line.strip()
        if not line or line.startswith("#"):
            continue
        raw = json.loads(line)
        bubbles = raw.get("bubbles") or ([raw["text"]] if raw.get("text") else [])
        if not bubbles:
            raise ValueError(f"{path}:{line_number}: case has no bubbles/text")
        route = raw["expected_route"]
        if route not in ROUTES:
            raise ValueError(f"{path}:{line_number}: unknown expected_route {route!r}")
        case_id = str(raw["id"])
        if case_id in seen:
            raise ValueError(f"{path}:{line_number}: duplicate id {case_id}")
        seen.add(case_id)
        history = raw.get("history") or []
        cases.append(
            Case(
                id=case_id,
                menu=raw["menu"],
                bubbles=[str(b) for b in bubbles],
                expected_route=route,
                expected_faq_id=raw.get("expected_faq_id"),
                expected_handoff_reason=raw.get("expected_handoff_reason") or "none",
                also_ok_routes=tuple(raw.get("also_ok_routes") or ()),
                history=history,
                first_message=bool(raw.get("first_message", not history)),
                kind=raw.get("kind", ""),
                slice=slice_name,
                kb_expect=raw.get("kb_expect"),
            )
        )
    return cases


def load_all_cases(paths: list[str | Path]) -> list[Case]:
    cases: list[Case] = []
    ids: set[str] = set()
    for path in paths:
        for case in load_cases(path):
            if case.id in ids:
                raise ValueError(f"duplicate case id across files: {case.id}")
            ids.add(case.id)
            cases.append(case)
    return cases


def load_menus(*paths: str | Path) -> dict[str, dict]:
    """menus.json: {menu_id: {"business": str, "faqs": [{key, question, answer, examples, not_for}]}}.
    Several files are merged; a menu id may only be defined once."""
    merged: dict[str, dict] = {}
    for path in paths:
        for menu_id, spec in json.loads(Path(path).read_text()).items():
            if menu_id in merged:
                raise ValueError(f"menu {menu_id} defined twice")
            merged[menu_id] = spec
    return merged


def load_kb(path: str | Path) -> dict[str, list[dict]]:
    return json.loads(Path(path).read_text())


def menu_entries(menus: dict[str, dict], menu_id: str):
    """Build router FaqEntry objects plus a code -> FAQ key map for one menu."""
    spec = menus[menu_id]
    rows = [
        {
            "id": uuid.uuid5(uuid.NAMESPACE_URL, f"{menu_id}/{faq['key']}"),
            "canonical_question": faq["question"],
            "answer_text": faq["answer"],
            "trigger_phrases": faq.get("examples", []),
            "not_for": faq.get("not_for", ""),
        }
        for faq in spec["faqs"]
    ]
    menu = build_menu(rows)
    key_by_code = {entry.code: spec["faqs"][index]["key"] for index, entry in enumerate(menu)}
    return menu, key_by_code


# ---------------------------------------------------------------------------
# Running
# ---------------------------------------------------------------------------

@dataclass
class Prediction:
    route: str = "handoff"
    faq_id: Optional[str] = None
    handoff_reason: str = "none"
    detail: str = ""
    latency_ms: int = 0
    prompt_tokens: int = 0
    completion_tokens: int = 0
    error: Optional[str] = None
    raw_route: Optional[str] = None
    flagged: str = ""  # soft review flag reason (prompt_injection | ignore_rejected), if any
    kb_answer: Optional[str] = None
    kb_reason: str = ""
    kb_latency_ms: int = 0


@dataclass
class CaseResult:
    case: Case
    prediction: Prediction

    @property
    def route_ok(self) -> bool:
        p, c = self.prediction, self.case
        if p.route == c.expected_route:
            if p.route == "faq":
                return p.faq_id == c.expected_faq_id
            return True
        return p.route in c.also_ok_routes

    @property
    def strict_ok(self) -> bool:
        p, c = self.prediction, self.case
        return p.route == c.expected_route and (p.route != "faq" or p.faq_id == c.expected_faq_id)


async def predict(
    case: Case,
    client: Any,
    model: str,
    menus: dict[str, dict],
    kb: Optional[dict[str, list[dict]]] = None,
) -> Prediction:
    """Run the production path for one case (no database). With `kb`, the grounded-answer stage runs
    on every case the gates send to it, with the menu's FAQ answers plus all of its concepts."""
    menu, key_by_code = menu_entries(menus, case.menu)
    pre = local_precheck(case.bubbles, case.first_message, has_history=bool(case.history))
    if pre is not None:
        if pre.kind == "greeting":
            return Prediction(route="greeting", detail=pre.detail)
        return Prediction(
            route="handoff",
            handoff_reason="needs_human" if pre.handoff_kind == "escalation" else "other",
            detail=pre.detail,
        )

    history = transcript_within_byte_budget([(t["role"], t["text"]) for t in case.history], 1500)
    result: RouterResult = await run_router(client, model, menu, history, case.bubbles, max_tokens=150)
    outcome = apply_gates(result, menu, case.bubbles)
    prediction = Prediction(
        latency_ms=result.latency_ms,
        prompt_tokens=result.usage.get("prompt_tokens", 0),
        completion_tokens=result.usage.get("completion_tokens", 0),
        error=result.error,
        detail=outcome.detail,
    )
    if result.decision is not None:
        prediction.raw_route = result.decision.route
        prediction.handoff_reason = result.decision.handoff_reason
    if outcome.kind == "canned":
        prediction.route, prediction.faq_id = "faq", key_by_code[outcome.faq.code]
        prediction.handoff_reason = "none"
    elif outcome.kind == "ignore":
        prediction.route = "ignore"
    elif outcome.kind == "kb":
        prediction.route = "kb"
        if kb is not None:
            sources = faq_sources(menu) + concept_sources(kb.get(case.menu, []))
            answer = await answer_from_sources(client, model, sources, history, case.bubbles, max_tokens=400)
            prediction.kb_reason = answer.reason
            prediction.kb_latency_ms = answer.latency_ms
            prediction.prompt_tokens += answer.usage.get("prompt_tokens", 0)
            prediction.completion_tokens += answer.usage.get("completion_tokens", 0)
            if answer.answer is not None:
                prediction.kb_answer = answer.answer
            else:  # production hands the message to a human
                prediction.route, prediction.handoff_reason = "handoff", "other"
                prediction.detail = f"kb:{answer.reason}"
    else:  # handoff (including fail-closed errors and soft flags)
        prediction.route = "handoff"
        prediction.flagged = outcome.flag_reason if outcome.handoff_kind == "flag" else ""
        if prediction.handoff_reason == "none":
            prediction.handoff_reason = "other"
    return prediction


class CachingClient:
    """Wraps a provider client with a JSONL replay cache keyed by the request, to save quota, and
    paces live calls (`min_interval` seconds apart) for rate-limited keys. Only successes are cached."""

    def __init__(self, inner: Any, path: str = "", min_interval: float = 0.0):
        self.inner, self.path, self.min_interval = inner, path, min_interval
        self.store: dict[str, dict] = {}
        if path and os.path.exists(path):
            for line in open(path):
                entry = json.loads(line)
                self.store[entry["key"]] = entry
        self.hits = 0
        self.live_calls = 0
        self._lock = asyncio.Lock()
        self._last_live = 0.0

    async def complete_detailed(self, model, messages, schema, max_tokens=None):
        from whatfunnel_ai import CompletionResult

        key = hashlib.sha256(
            json.dumps([model, messages, schema.model_json_schema(), max_tokens], sort_keys=True).encode()
        ).hexdigest()
        entry = self.store.get(key)
        if entry is not None:
            self.hits += 1
            validated = schema.model_validate(entry["data"]).model_dump()
            return CompletionResult(validated, entry["usage"], entry["latency_ms"])
        async with self._lock:  # one live call at a time keeps the pacing honest
            wait = self.min_interval - (asyncio.get_running_loop().time() - self._last_live)
            if wait > 0 and self.live_calls:
                await asyncio.sleep(wait)
            try:
                result = await self.inner.complete_detailed(model, messages, schema, max_tokens=max_tokens)
            finally:
                self._last_live = asyncio.get_running_loop().time()
                self.live_calls += 1
        entry = {"key": key, "data": result.data, "usage": result.usage, "latency_ms": result.latency_ms}
        self.store[key] = entry
        if self.path:
            with open(self.path, "a") as handle:
                handle.write(json.dumps(entry) + "\n")
        return result


async def run_cases(
    cases: list[Case],
    menus: dict[str, dict],
    client: Any,
    model: str,
    concurrency: int = 4,
    min_interval: float = 0.0,
    kb: Optional[dict[str, list[dict]]] = None,
) -> list[CaseResult]:
    semaphore = asyncio.Semaphore(max(1, concurrency))
    results: list[Optional[CaseResult]] = [None] * len(cases)
    paced_by_client = hasattr(client, "live_calls")

    async def worker(index: int, case: Case) -> None:
        async with semaphore:
            prediction = await predict(case, client, model, menus, kb)
            if min_interval and not paced_by_client:
                await asyncio.sleep(min_interval)
        results[index] = CaseResult(case, prediction)

    await asyncio.gather(*(worker(i, c) for i, c in enumerate(cases)))
    return [r for r in results if r is not None]


# ---------------------------------------------------------------------------
# Metrics
# ---------------------------------------------------------------------------

def _ratio(numerator: int, denominator: int) -> Optional[float]:
    return None if denominator == 0 else numerator / denominator


def _percentile(values: list[float], pct: float) -> Optional[float]:
    if not values:
        return None
    ordered = sorted(values)
    index = min(len(ordered) - 1, max(0, math.ceil(pct / 100 * len(ordered)) - 1))
    return ordered[index]


def _norm(text: str) -> str:
    return " ".join(text.lower().replace(",", "").split())


def _kb_metrics(results: list[CaseResult]) -> dict:
    kb_cases = [r for r in results if r.case.kb_expect]
    answerable = [r for r in kb_cases if r.case.kb_expect.get("answerable")]
    unanswerable = [r for r in kb_cases if r.case.kb_expect.get("answerable") is False]

    def includes_all(r: CaseResult) -> bool:
        answer = _norm(r.prediction.kb_answer or "")
        return all(_norm(s) in answer for s in r.case.kb_expect.get("must_include", []))

    def forbidden_hit(r: CaseResult) -> bool:
        answer = _norm(r.prediction.kb_answer or "")
        return any(_norm(s) in answer for s in r.case.kb_expect.get("must_not_include", []))

    answered = [r for r in answerable if r.prediction.kb_answer is not None]
    correct = [r for r in answered if includes_all(r) and not forbidden_hit(r)]
    incomplete = [r for r in answered if not includes_all(r) and not forbidden_hit(r)]
    hallucinated_answerable = [r for r in answered if forbidden_hit(r)]
    answered_anyway = [r for r in unanswerable if r.prediction.kb_answer is not None]
    hallucinated = [r for r in answered_anyway if forbidden_hit(r)] + [
        r for r in hallucinated_answerable
    ]
    produced = [r for r in results if r.prediction.kb_answer is not None]
    rejected = [
        r for r in results
        if r.prediction.kb_reason and r.prediction.kb_reason not in ("grounded", "kb_needs_human", "no_sources", "kb_error")
    ]
    return {
        "cases": len(kb_cases),
        "answerable": len(answerable),
        "answerable_answered": len(answered),
        "answerable_correct": len(correct),
        "answerable_correct_rate": _ratio(len(correct), len(answerable)),
        "answerable_incomplete": len(incomplete),
        "answerable_handed_off": len(answerable) - len(answered),
        "unanswerable": len(unanswerable),
        "unanswerable_handed_off": len(unanswerable) - len(answered_anyway),
        "unanswerable_answered_anyway": len(answered_anyway),
        "hallucinations": len(hallucinated),
        "hallucination_rate": _ratio(len(hallucinated), len(produced)) if produced else None,
        "answers_produced": len(produced),
        "gate_rejections": len(rejected),
        "gate_rejection_reasons": dict(Counter(r.prediction.kb_reason.split(":")[0] for r in rejected)),
        "hallucinated_cases": [_describe(r) | {"answer": r.prediction.kb_answer} for r in hallucinated],
        "incomplete_cases": [_describe(r) | {"answer": r.prediction.kb_answer} for r in incomplete],
        "answered_anyway_cases": [_describe(r) | {"answer": r.prediction.kb_answer} for r in answered_anyway],
        "gate_rejected_cases": [_describe(r) for r in rejected],
    }


def compute_metrics(
    results: list[CaseResult],
    price_in_per_m: Optional[float] = None,
    price_out_per_m: Optional[float] = None,
    with_slices: bool = True,
) -> dict:
    total = len(results)

    def effective_route(r: CaseResult) -> str:
        """Predicted route, or the expected one if the prediction is an accepted alternative."""
        return r.case.expected_route if r.route_ok else r.prediction.route

    confusion: dict[str, Counter] = {e: Counter() for e in ROUTES}
    for r in results:
        confusion[r.case.expected_route][effective_route(r)] += 1

    per_route = {}
    for route in ROUTES:
        tp = sum(1 for r in results if r.case.expected_route == route and effective_route(r) == route and r.route_ok)
        predicted = sum(1 for r in results if effective_route(r) == route)
        expected = sum(1 for r in results if r.case.expected_route == route)
        precision, recall = _ratio(tp, predicted), _ratio(tp, expected)
        f1 = None if not precision or not recall else 2 * precision * recall / (precision + recall)
        per_route[route] = {"precision": precision, "recall": recall, "f1": f1, "support": expected, "predicted": predicted}

    # Canned FAQ replies (faq route after gates).
    canned = [r for r in results if r.prediction.route == "faq"]
    canned_correct = [r for r in canned if r.case.expected_route == "faq" and r.prediction.faq_id == r.case.expected_faq_id]
    faq_cases = [r for r in results if r.case.expected_route == "faq"]
    negatives = [r for r in results if r.case.expected_route != "faq"]
    false_canned = [r for r in canned if r.case.expected_route != "faq" and "faq" not in r.case.also_ok_routes]
    wrong_faq = [r for r in canned if r.case.expected_route == "faq" and r.prediction.faq_id != r.case.expected_faq_id]

    # Handoff / escalation recall.
    escalation_cases = [r for r in results if r.case.expected_route == "handoff" and r.case.expected_handoff_reason in ESCALATION_REASONS]
    escalation_hit = [r for r in escalation_cases if r.prediction.route == "handoff"]
    reason_hit = [r for r in escalation_cases if r.prediction.route == "handoff" and r.prediction.handoff_reason == r.case.expected_handoff_reason]
    by_reason = {}
    for reason in ESCALATION_REASONS:
        subset = [r for r in escalation_cases if r.case.expected_handoff_reason == reason]
        by_reason[reason] = {
            "support": len(subset),
            "handoff_recall": _ratio(sum(1 for r in subset if r.prediction.route == "handoff"), len(subset)),
            "reason_recall": _ratio(sum(1 for r in subset if r.prediction.route == "handoff" and r.prediction.handoff_reason == reason), len(subset)),
        }
    non_escalation = [r for r in results if not (r.case.expected_route == "handoff" and r.case.expected_handoff_reason in ESCALATION_REASONS)]
    false_escalations = [
        r for r in non_escalation
        if r.prediction.route == "handoff" and r.prediction.handoff_reason in ESCALATION_REASONS and "handoff" not in r.case.also_ok_routes
    ]
    # Injection: nothing may reach the customer except through a flag/handoff.
    injection_cases = [r for r in results if r.case.expected_handoff_reason == "prompt_injection"]
    injection_leaks = [r for r in injection_cases if r.prediction.route not in ("handoff",)]

    # Silent drops: a message dropped without a reply or a human flag (route ignore) although it was
    # not labelled an acceptable ignore.
    silent_drops = [
        r for r in results
        if r.prediction.route == "ignore" and r.case.expected_route != "ignore" and "ignore" not in r.case.also_ok_routes
    ]
    ignored_total = [r for r in results if r.prediction.route == "ignore"]

    latencies = [float(r.prediction.latency_ms) for r in results if r.prediction.latency_ms]
    kb_latencies = [float(r.prediction.kb_latency_ms) for r in results if r.prediction.kb_latency_ms]
    prompt_tokens = sum(r.prediction.prompt_tokens for r in results)
    completion_tokens = sum(r.prediction.completion_tokens for r in results)
    cost = None
    if price_in_per_m is not None and price_out_per_m is not None:
        cost = prompt_tokens / 1e6 * price_in_per_m + completion_tokens / 1e6 * price_out_per_m

    by_kind: dict[str, dict] = {}
    for kind in sorted({r.case.kind for r in results}):
        subset = [r for r in results if r.case.kind == kind]
        by_kind[kind] = {
            "cases": len(subset),
            "route_accuracy": _ratio(sum(1 for r in subset if r.route_ok), len(subset)),
            "strict_accuracy": _ratio(sum(1 for r in subset if r.strict_ok), len(subset)),
        }

    metrics = {
        "prompt_version": PROMPT_VERSION,
        "cases": total,
        "route_accuracy": _ratio(sum(1 for r in results if r.route_ok), total),
        "strict_accuracy": _ratio(sum(1 for r in results if r.strict_ok), total),
        "errors": sum(1 for r in results if r.prediction.error),
        "per_route": per_route,
        "confusion_matrix": {e: dict(confusion[e]) for e in ROUTES},
        "canned": {
            "sent": len(canned),
            "correct": len(canned_correct),
            "precision": _ratio(len(canned_correct), len(canned)),
            "recall": _ratio(len(canned_correct), len(faq_cases)),
            "false_canned": len(false_canned),
            "false_canned_rate": _ratio(len(false_canned), len(negatives)),
            "wrong_faq": len(wrong_faq),
        },
        "handoff": {
            "escalation_cases": len(escalation_cases),
            "recall": _ratio(len(escalation_hit), len(escalation_cases)),
            "reason_accuracy": _ratio(len(reason_hit), len(escalation_cases)),
            "by_reason": by_reason,
            "false_escalations": len(false_escalations),
            "false_escalation_rate": _ratio(len(false_escalations), len(non_escalation)),
        },
        "injection": {
            "cases": len(injection_cases),
            "to_human": len(injection_cases) - len(injection_leaks),
            "leaks": len(injection_leaks),
            "flagged_as_injection": sum(1 for r in injection_cases if r.prediction.handoff_reason == "prompt_injection" and r.prediction.route == "handoff"),
        },
        "silent_drops": {
            "count": len(silent_drops),
            "ignored_total": len(ignored_total),
            "ignore_rejected_flags": sum(1 for r in results if r.prediction.flagged == "ignore_rejected"),
            "injection_flags": sum(1 for r in results if r.prediction.flagged == "prompt_injection"),
        },
        "kb": _kb_metrics(results),
        "latency_ms": {"mean": (sum(latencies) / len(latencies)) if latencies else None, "p50": _percentile(latencies, 50), "p95": _percentile(latencies, 95), "max": max(latencies) if latencies else None, "calls": len(latencies)},
        "kb_latency_ms": {"mean": (sum(kb_latencies) / len(kb_latencies)) if kb_latencies else None, "p50": _percentile(kb_latencies, 50), "p95": _percentile(kb_latencies, 95), "calls": len(kb_latencies)},
        "tokens": {"prompt": prompt_tokens, "completion": completion_tokens},
        "estimated_cost_usd": cost,
        "estimated_cost_per_1000_messages_usd": None if cost is None or not total else cost / total * 1000,
        "by_kind": by_kind,
        "false_canned_cases": [_describe(r) for r in false_canned + wrong_faq],
        "false_escalation_cases": [_describe(r) for r in false_escalations],
        "missed_escalation_cases": [_describe(r) for r in escalation_cases if r.prediction.route != "handoff"],
        "silent_drop_cases": [_describe(r) for r in silent_drops],
        "wrong_cases": [_describe(r) for r in results if not r.route_ok],
    }
    if with_slices:
        metrics["slices"] = {
            name: compute_metrics([r for r in results if r.case.slice == name], price_in_per_m, price_out_per_m, with_slices=False)
            for name in sorted({r.case.slice for r in results})
        }
    return metrics


def _describe(r: CaseResult) -> dict:
    return {
        "id": r.case.id,
        "slice": r.case.slice,
        "text": " / ".join(r.case.bubbles),
        "kind": r.case.kind,
        "expected": r.case.expected_route + (f":{r.case.expected_faq_id}" if r.case.expected_faq_id else ""),
        "also_ok": list(r.case.also_ok_routes),
        "predicted": r.prediction.route + (f":{r.prediction.faq_id}" if r.prediction.faq_id else ""),
        "handoff_reason": r.prediction.handoff_reason,
        "detail": r.prediction.detail,
        "error": r.prediction.error,
    }


# ---------------------------------------------------------------------------
# Reporting
# ---------------------------------------------------------------------------

def _pct(value: Optional[float]) -> str:
    return "n/a" if value is None else f"{value * 100:5.1f}%"


def _headline(name: str, m: dict) -> str:
    c, h = m["canned"], m["handoff"]
    return (
        f"  {name:<9}{m['cases']:>5}{_pct(m['route_accuracy']):>9}{_pct(m['strict_accuracy']):>9}"
        f"{_pct(c['precision']):>9}{_pct(c['recall']):>9}{c['false_canned']:>7}{c['wrong_faq']:>7}"
        f"{_pct(h['recall']):>9}{m['injection']['leaks']:>6}{m['silent_drops']['count']:>7}"
    )


def format_report(metrics: dict, show_cases: int = 25) -> str:
    lines = [
        f"Router eval  prompt={metrics['prompt_version']}  cases={metrics['cases']}  "
        f"route accuracy={_pct(metrics['route_accuracy'])} (strict {_pct(metrics['strict_accuracy'])})  "
        f"provider errors={metrics['errors']}",
        "",
        "By slice",
        f"  {'slice':<9}{'n':>5}{'lenient':>9}{'strict':>9}{'canned P':>9}{'canned R':>9}{'false':>7}{'wrongF':>7}{'esc R':>9}{'inj':>6}{'silent':>7}",
    ]
    for name, m in metrics.get("slices", {}).items():
        lines.append(_headline(name, m))
    lines.append(_headline("ALL", metrics))
    lines += [
        "",
        "Per route (final outcome after gates)",
        f"  {'route':<10}{'precision':>10}{'recall':>9}{'f1':>8}{'support':>9}{'predicted':>10}",
    ]
    for route, m in metrics["per_route"].items():
        f1 = "n/a" if m["f1"] is None else f"{m['f1']:.2f}"
        lines.append(f"  {route:<10}{_pct(m['precision']):>10}{_pct(m['recall']):>9}{f1:>8}{m['support']:>9}{m['predicted']:>10}")
    c = metrics["canned"]
    lines += [
        "",
        "Canned FAQ replies",
        f"  sent={c['sent']} correct={c['correct']} precision={_pct(c['precision'])} recall={_pct(c['recall'])}",
        f"  false canned={c['false_canned']} (rate on non-FAQ cases {_pct(c['false_canned_rate'])}), wrong FAQ id={c['wrong_faq']}",
    ]
    h = metrics["handoff"]
    lines += [
        "",
        "Handoff / escalation",
        f"  escalation cases={h['escalation_cases']} recall={_pct(h['recall'])} reason accuracy={_pct(h['reason_accuracy'])}",
    ]
    for reason, m in h["by_reason"].items():
        lines.append(f"    {reason:<17} n={m['support']:<3} handoff recall={_pct(m['handoff_recall'])} reason recall={_pct(m['reason_recall'])}")
    lines.append(f"  false escalations={h['false_escalations']} ({_pct(h['false_escalation_rate'])} of non-escalation cases)")
    i, s = metrics["injection"], metrics["silent_drops"]
    lines += [
        "",
        f"Injection: {i['cases']} cases, {i['to_human']} to a human, {i['leaks']} leaked, {i['flagged_as_injection']} flagged as prompt_injection",
        f"Silent drops (ignored without a flag, not labelled ignore): {s['count']}  "
        f"(ignored in total {s['ignored_total']}; ignore rejected by the guard and flagged {s['ignore_rejected_flags']}; "
        f"injection flags {s['injection_flags']})",
    ]
    kb = metrics["kb"]
    if kb["cases"]:
        lines += [
            "",
            "Grounded answers (KB stage; FAQ answers + concepts as sources)",
            f"  answerable cases={kb['answerable']}: answered {kb['answerable_answered']}, correct {kb['answerable_correct']} "
            f"({_pct(kb['answerable_correct_rate'])}), incomplete {kb['answerable_incomplete']}, handed off {kb['answerable_handed_off']}",
            f"  unanswerable cases={kb['unanswerable']}: handed off {kb['unanswerable_handed_off']}, answered anyway {kb['unanswerable_answered_anyway']}",
            f"  answers produced={kb['answers_produced']}, hallucinations (forbidden content)={kb['hallucinations']}, "
            f"answers blocked by the grounding gate={kb['gate_rejections']} {kb['gate_rejection_reasons']}",
        ]
    lines += ["", "Confusion matrix (rows expected, columns predicted)"]
    lines.append("  " + " " * 10 + "".join(f"{r:>10}" for r in ROUTES))
    for expected in ROUTES:
        row = metrics["confusion_matrix"][expected]
        lines.append("  " + f"{expected:<10}" + "".join(f"{row.get(p, 0):>10}" for p in ROUTES))
    lat, tok, kl = metrics["latency_ms"], metrics["tokens"], metrics["kb_latency_ms"]
    mean = "n/a" if lat["mean"] is None else f"{lat['mean']:.0f}"
    kmean = "n/a" if kl["mean"] is None else f"{kl['mean']:.0f}"
    lines += [
        "",
        f"Router latency ms: mean={mean} p50={lat['p50']} p95={lat['p95']} max={lat['max']} (calls={lat['calls']});  "
        f"KB latency ms: mean={kmean} p50={kl['p50']} p95={kl['p95']} (calls={kl['calls']})",
        f"Tokens: prompt={tok['prompt']} completion={tok['completion']}",
    ]
    if metrics["estimated_cost_usd"] is not None:
        lines.append(f"Estimated cost: ${metrics['estimated_cost_usd']:.4f} total, ${metrics['estimated_cost_per_1000_messages_usd']:.3f} per 1000 messages")
    else:
        lines.append("Estimated cost: set EVAL_PRICE_IN / EVAL_PRICE_OUT (USD per 1M tokens) to estimate")
    lines += ["", "By kind"] + [
        f"  {k:<26} n={m['cases']:<4} lenient={_pct(m['route_accuracy'])} strict={_pct(m['strict_accuracy'])}"
        for k, m in metrics["by_kind"].items()
    ]
    sections = [
        ("False canned replies", "false_canned_cases"), ("Missed escalations", "missed_escalation_cases"),
        ("False escalations", "false_escalation_cases"), ("Silent drops", "silent_drop_cases"),
        ("Other wrong routes", "wrong_cases"),
    ]
    for title, key in sections:
        items = metrics[key]
        if items:
            lines += ["", f"{title} ({len(items)})"]
            for item in items[:show_cases]:
                lines.append(f"  [{item['id']}] {item['text'][:90]!r} expected={item['expected']} predicted={item['predicted']} reason={item['handoff_reason']} {item['detail']}")
    for title, key in (("KB hallucinations", "hallucinated_cases"), ("KB incomplete answers", "incomplete_cases"), ("KB answered although unanswerable", "answered_anyway_cases")):
        items = kb[key]
        if items:
            lines += ["", f"{title} ({len(items)})"]
            for item in items[:show_cases]:
                lines.append(f"  [{item['id']}] {item['text'][:70]!r} -> {str(item['answer'])[:140]!r}")
    return "\n".join(lines)


# ---------------------------------------------------------------------------
# CLI
# ---------------------------------------------------------------------------

def build_client_from_env(timeout: float = 30.0):
    from whatfunnel_ai import ProviderClient

    base_url, api_key = os.environ.get("EVAL_BASE_URL"), os.environ.get("EVAL_API_KEY")
    if not base_url or not api_key:
        raise SystemExit("Set EVAL_BASE_URL and EVAL_API_KEY (OpenAI-compatible endpoint), and optionally EVAL_MODEL.")
    return ProviderClient(
        api_key=api_key,
        base_url=base_url,
        timeout_seconds=timeout,
        max_attempts=int(os.environ.get("EVAL_MAX_ATTEMPTS", "4")),
        retry_backoff_seconds=float(os.environ.get("EVAL_RETRY_BACKOFF", "3")),
        max_retry_wait_seconds=float(os.environ.get("EVAL_MAX_RETRY_WAIT", "60")),
    )


def main(argv: Optional[list[str]] = None) -> int:
    parser = argparse.ArgumentParser(description="Evaluate the cascade router and KB stage on labelled JSONL sets")
    parser.add_argument("--cases", nargs="+", default=[str(HERE / f) for f in DEFAULT_CASE_FILES])
    parser.add_argument("--menus", nargs="+", default=[str(HERE / f) for f in DEFAULT_MENU_FILES])
    parser.add_argument("--kb", default=str(HERE / "kb.json"), help="knowledge fixtures for the KB stage")
    parser.add_argument("--no-kb", action="store_true", help="skip the grounded-answer stage")
    parser.add_argument("--limit", type=int, default=0, help="only run the first N cases")
    parser.add_argument("--only-kind", default="", help="comma-separated kinds to run")
    parser.add_argument("--only-slice", default="", help="comma-separated slices to run (seen, heldout, kb)")
    parser.add_argument("--only-ids", default="", help="comma-separated case ids to run")
    parser.add_argument("--cache", default=os.environ.get("EVAL_CACHE", ""), help="JSONL replay cache path")
    parser.add_argument("--json", dest="json_out", default="", help="write full metrics JSON here")
    parser.add_argument("--concurrency", type=int, default=int(os.environ.get("EVAL_CONCURRENCY", "1")))
    parser.add_argument("--min-interval", type=float, default=float(os.environ.get("EVAL_MIN_INTERVAL", "0")), help="seconds between live calls (rate limits)")
    args = parser.parse_args(argv)

    cases = load_all_cases(args.cases)
    if args.only_slice:
        cases = [c for c in cases if c.slice in set(args.only_slice.split(","))]
    if args.only_kind:
        cases = [c for c in cases if c.kind in set(args.only_kind.split(","))]
    if args.only_ids:
        cases = [c for c in cases if c.id in set(args.only_ids.split(","))]
    if args.limit:
        cases = cases[: args.limit]
    menus = load_menus(*args.menus)
    kb = None if args.no_kb else load_kb(args.kb)
    model = os.environ.get("EVAL_MODEL", DEFAULT_MODEL)
    client = CachingClient(build_client_from_env(float(os.environ.get("EVAL_TIMEOUT", "30"))), args.cache, args.min_interval)
    print(f"Running {len(cases)} cases with model {model}", file=sys.stderr)
    results = asyncio.run(run_cases(cases, menus, client, model, args.concurrency, kb=kb))
    print(f"live calls={client.live_calls} cache hits={client.hits}", file=sys.stderr)
    price_in = float(os.environ["EVAL_PRICE_IN"]) if os.environ.get("EVAL_PRICE_IN") else None
    price_out = float(os.environ["EVAL_PRICE_OUT"]) if os.environ.get("EVAL_PRICE_OUT") else None
    metrics = compute_metrics(results, price_in, price_out)
    print(format_report(metrics))
    if args.json_out:
        Path(args.json_out).write_text(json.dumps(metrics, indent=2))
        rows = [
            {"id": r.case.id, "slice": r.case.slice, "text": r.case.bubbles, "expected": r.case.expected_route, "prediction": r.prediction.__dict__}
            for r in results
        ]
        Path(args.json_out).with_suffix(".rows.json").write_text(json.dumps(rows, indent=1))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
