"""Offline evaluation harness for the cascade router.

Runs the real routing code (local pre-checks, router prompt/schema, deterministic gates from
router.py) over a labelled JSONL set against any OpenAI-compatible chat-completions endpoint and
reports precision/recall per route, canned-reply precision, false-canned rate, handoff recall, a
confusion matrix and cost/latency.

    EVAL_BASE_URL=https://.../v1 EVAL_API_KEY=... EVAL_MODEL=gemini-3.1-flash-lite \\
        python -m eval.harness --cases eval/cases.jsonl --menus eval/menus.json

Run from services/ai-answer-svc with PYTHONPATH=.:../../packages/python (see README).

Case format (one JSON object per line):
    id                      unique case id
    menu                    key into menus.json (the account's FAQ menu)
    history                 optional prior turns: [{"role": "customer"|"agent", "text": "..."}]
    bubbles                 1-3 customer messages of one conversation (or "text" for a single one)
    first_message           optional bool; default: true when there is no history
    expected_route          greeting | faq | kb | handoff | ignore
    expected_faq_id         FAQ key in the menu (only for expected_route == faq), else null
    expected_handoff_reason none | spam | needs_human | prompt_injection | other
    also_ok_routes          optional extra routes that are acceptable (judgement calls)
    kind                    free-form slice label (paraphrase, typo, near_miss, follow_up, ...)

A prediction counts as correct when its route equals expected_route or is in also_ok_routes (and,
for faq, the FAQ matches). The route of a prediction is the FINAL outcome after the deterministic
gates: "faq" means a canned reply would have been sent verbatim.
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


def load_cases(path: str | Path) -> list[Case]:
    cases: list[Case] = []
    seen: set[str] = set()
    for line_number, line in enumerate(Path(path).read_text().splitlines(), start=1):
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
            )
        )
    return cases


def load_menus(path: str | Path) -> dict[str, dict]:
    """menus.json: {menu_id: {"business": str, "faqs": [{key, question, answer, examples, not_for}]}}"""
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


async def predict(case: Case, client: Any, model: str, menus: dict[str, dict]) -> Prediction:
    """Run the production routing path for one case (no database)."""
    menu, key_by_code = menu_entries(menus, case.menu)
    pre = local_precheck(case.bubbles, case.first_message, has_history=bool(case.history))
    if pre is not None:
        if pre.kind == "greeting":
            return Prediction(route="greeting", detail=pre.detail)
        return Prediction(route="handoff", handoff_reason="needs_human" if pre.handoff_kind == "escalation" else "other", detail=pre.detail)

    history = transcript_within_byte_budget(
        [(t["role"], t["text"]) for t in case.history], 1500
    )
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
    elif outcome.kind in ("kb", "ignore"):
        prediction.route = outcome.kind
    else:  # handoff (including fail-closed errors)
        prediction.route = "handoff"
        if prediction.handoff_reason == "none":
            prediction.handoff_reason = "other"
    return prediction


class CachingClient:
    """Wraps a provider client with a JSONL replay cache keyed by the request, to save quota."""

    def __init__(self, inner: Any, path: str):
        self.inner, self.path = inner, path
        self.store: dict[str, dict] = {}
        if os.path.exists(path):
            for line in open(path):
                entry = json.loads(line)
                self.store[entry["key"]] = entry
        self.hits = 0

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
        result = await self.inner.complete_detailed(model, messages, schema, max_tokens=max_tokens)
        entry = {"key": key, "data": result.data, "usage": result.usage, "latency_ms": result.latency_ms}
        self.store[key] = entry
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
) -> list[CaseResult]:
    semaphore = asyncio.Semaphore(max(1, concurrency))
    results: list[Optional[CaseResult]] = [None] * len(cases)

    async def worker(index: int, case: Case) -> None:
        async with semaphore:
            prediction = await predict(case, client, model, menus)
            if min_interval:
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


def compute_metrics(
    results: list[CaseResult],
    price_in_per_m: Optional[float] = None,
    price_out_per_m: Optional[float] = None,
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
    # A canned reply on a case that accepts "faq" as an alternative is not counted as false.
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

    latencies = [float(r.prediction.latency_ms) for r in results if r.prediction.latency_ms]
    prompt_tokens = sum(r.prediction.prompt_tokens for r in results)
    completion_tokens = sum(r.prediction.completion_tokens for r in results)
    cost = None
    if price_in_per_m is not None and price_out_per_m is not None:
        cost = prompt_tokens / 1e6 * price_in_per_m + completion_tokens / 1e6 * price_out_per_m

    by_kind: dict[str, dict] = {}
    for kind in sorted({r.case.kind for r in results}):
        subset = [r for r in results if r.case.kind == kind]
        by_kind[kind] = {"cases": len(subset), "route_accuracy": _ratio(sum(1 for r in subset if r.route_ok), len(subset))}

    return {
        "prompt_version": PROMPT_VERSION,
        "cases": total,
        "route_accuracy": _ratio(sum(1 for r in results if r.route_ok), total),
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
        "latency_ms": {"mean": (sum(latencies) / len(latencies)) if latencies else None, "p50": _percentile(latencies, 50), "p95": _percentile(latencies, 95), "calls": len(latencies)},
        "tokens": {"prompt": prompt_tokens, "completion": completion_tokens},
        "estimated_cost_usd": cost,
        "estimated_cost_per_1000_messages_usd": None if cost is None or not total else cost / total * 1000,
        "by_kind": by_kind,
        "false_canned_cases": [_describe(r) for r in false_canned + wrong_faq],
        "false_escalation_cases": [_describe(r) for r in false_escalations],
        "missed_escalation_cases": [_describe(r) for r in escalation_cases if r.prediction.route != "handoff"],
        "wrong_cases": [_describe(r) for r in results if not r.route_ok],
    }


def _describe(r: CaseResult) -> dict:
    return {
        "id": r.case.id,
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


def format_report(metrics: dict, show_cases: int = 25) -> str:
    lines = [
        f"Router eval  prompt={metrics['prompt_version']}  cases={metrics['cases']}  "
        f"route accuracy={_pct(metrics['route_accuracy'])}  provider errors={metrics['errors']}",
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
    lines += ["", "Confusion matrix (rows expected, columns predicted)"]
    header = "  " + " " * 10 + "".join(f"{r:>10}" for r in ROUTES)
    lines.append(header)
    for expected in ROUTES:
        row = metrics["confusion_matrix"][expected]
        lines.append("  " + f"{expected:<10}" + "".join(f"{row.get(p, 0):>10}" for p in ROUTES))
    lat, tok = metrics["latency_ms"], metrics["tokens"]
    mean = "n/a" if lat["mean"] is None else f"{lat['mean']:.0f}"
    lines += [
        "",
        f"Latency ms: mean={mean} p50={lat['p50']} p95={lat['p95']} (calls={lat['calls']})",
        f"Tokens: prompt={tok['prompt']} completion={tok['completion']}",
    ]
    if metrics["estimated_cost_usd"] is not None:
        lines.append(f"Estimated cost: ${metrics['estimated_cost_usd']:.4f} total, ${metrics['estimated_cost_per_1000_messages_usd']:.3f} per 1000 messages")
    else:
        lines.append("Estimated cost: set EVAL_PRICE_IN / EVAL_PRICE_OUT (USD per 1M tokens) to estimate")
    lines += ["", "By kind"] + [f"  {k:<18} n={m['cases']:<4} accuracy={_pct(m['route_accuracy'])}" for k, m in metrics["by_kind"].items()]
    for title, key in (("False canned replies", "false_canned_cases"), ("Missed escalations", "missed_escalation_cases"), ("False escalations", "false_escalation_cases"), ("Other wrong routes", "wrong_cases")):
        items = metrics[key]
        if items:
            lines += ["", f"{title} ({len(items)})"]
            for item in items[:show_cases]:
                lines.append(f"  [{item['id']}] {item['text'][:90]!r} expected={item['expected']} predicted={item['predicted']} reason={item['handoff_reason']}")
    return "\n".join(lines)


# ---------------------------------------------------------------------------
# CLI
# ---------------------------------------------------------------------------

def build_client_from_env(timeout: float = 30.0):
    from whatfunnel_ai import ProviderClient

    base_url, api_key = os.environ.get("EVAL_BASE_URL"), os.environ.get("EVAL_API_KEY")
    if not base_url or not api_key:
        raise SystemExit("Set EVAL_BASE_URL and EVAL_API_KEY (OpenAI-compatible endpoint), and optionally EVAL_MODEL.")
    return ProviderClient(api_key=api_key, base_url=base_url, timeout_seconds=timeout, max_attempts=int(os.environ.get("EVAL_MAX_ATTEMPTS", "2")))


def main(argv: Optional[list[str]] = None) -> int:
    here = Path(__file__).parent
    parser = argparse.ArgumentParser(description="Evaluate the cascade router on a labelled JSONL set")
    parser.add_argument("--cases", default=str(here / "cases.jsonl"))
    parser.add_argument("--menus", default=str(here / "menus.json"))
    parser.add_argument("--limit", type=int, default=0, help="only run the first N cases")
    parser.add_argument("--only-kind", default="", help="comma-separated kinds to run")
    parser.add_argument("--cache", default=os.environ.get("EVAL_CACHE", ""), help="JSONL replay cache path")
    parser.add_argument("--json", dest="json_out", default="", help="write full metrics JSON here")
    parser.add_argument("--concurrency", type=int, default=int(os.environ.get("EVAL_CONCURRENCY", "4")))
    parser.add_argument("--min-interval", type=float, default=float(os.environ.get("EVAL_MIN_INTERVAL", "0")), help="seconds to sleep after each call (rate limits)")
    args = parser.parse_args(argv)

    cases = load_cases(args.cases)
    if args.only_kind:
        wanted = set(args.only_kind.split(","))
        cases = [c for c in cases if c.kind in wanted]
    if args.limit:
        cases = cases[: args.limit]
    menus = load_menus(args.menus)
    model = os.environ.get("EVAL_MODEL", DEFAULT_MODEL)
    client = build_client_from_env(float(os.environ.get("EVAL_TIMEOUT", "30")))
    if args.cache:
        client = CachingClient(client, args.cache)
    print(f"Running {len(cases)} cases with model {model}", file=sys.stderr)
    results = asyncio.run(run_cases(cases, menus, client, model, args.concurrency, args.min_interval))
    price_in = float(os.environ["EVAL_PRICE_IN"]) if os.environ.get("EVAL_PRICE_IN") else None
    price_out = float(os.environ["EVAL_PRICE_OUT"]) if os.environ.get("EVAL_PRICE_OUT") else None
    metrics = compute_metrics(results, price_in, price_out)
    print(format_report(metrics))
    if args.json_out:
        Path(args.json_out).write_text(json.dumps(metrics, indent=2))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
