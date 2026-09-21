"""
Modular matching helper for AI Cascade Tier 1 matching engine.
Provides clause segmentation, conversational filler stripping,
punctuation normalization, and rapidfuzz pattern matching.
"""

from __future__ import annotations

import re
from typing import Any
from rapidfuzz import fuzz

# Conversational salutations, fillers, and acknowledgments
FILLER_PREFIXES = [
    # Salutations (e.g., "hi", "hello there", "hey guys", "good morning")
    r"^(hi|hello|hey)(\s+(there|guys|everyone|all|y\x27all|yall))?\b",
    r"^(good\s+(morning|afternoon|evening|day))\b",
    r"^greetings\b",
    # Conversational questions and fillers (e.g., "quick question", "can you tell me", "i was wondering")
    r"^(quick\s+question(\s+(about|regarding)(\s+your)?\s+\w+)?|i\s+have\s+a\s+question(\s+(about|regarding)(\s+your)?\s+\w+)?)\b",
    r"^(can\s+you\s+tell\s+me|could\s+you\s+tell\s+me|do\s+you\s+know)\b",
    r"^(i\s+was\s+wondering|just\s+wondering)(\s+(if|about|whether))?\b",
    r"^(i\s+wanted\s+to\s+ask|i(?:\x27|)d\s+like\s+to\s+(ask|know)|i\s+would\s+like\s+to\s+(ask|know))\b",
    r"^(i\s+need\s+my\s+\w+\s+\w+)\b",
    # Follow-up acknowledgments (e.g., "thanks", "got it", "awesome")
    r"^(thanks|thank\s+you)(\s+(so\s+much|very\s+much|a\s+lot))?\b",
    r"^(many\s+thanks)\b",
    r"^(got\s+it|understood|that\s+helps|sounds\s+good)\b",
    r"^(awesome|great|cool|okay|ok|perfect|alright)(\s+(thanks|thank\s+you))?\b",
    # Transition words
    r"^(excuse\s+me|by\s+the\s+way|btw|so|well)\b",
]

PUNCTUATION_SPLIT_REGEX = re.compile(r"[\n.?!;]+")
STRIP_PUNCTUATION = " ,!.-?:;\"\x27\n\t"
RAPIDFUZZ_DEFAULT_THRESHOLD = 90.0


def clean_segment(s: str) -> str:
    """
    Strips leading conversational salutations (hi, hello, hey, good morning, etc.),
    conversational filler ("quick question", "can you tell me", "i was wondering"),
    and follow-up acknowledgments ("thanks", "got it", "awesome").
    """
    if not s or not isinstance(s, str):
        return ""
    s = s.strip().lower()
    changed = True
    while changed:
        changed = False
        for pat in FILLER_PREFIXES:
            m = re.match(pat, s)
            if m:
                s = s[m.end():].lstrip(STRIP_PUNCTUATION)
                changed = True
    return s.strip(STRIP_PUNCTUATION)


def normalize_text(text: str) -> str:
    """
    Normalizes punctuation, hyphens, and whitespace to avoid tokenization mismatches.
    Replaces punctuation and hyphens with spaces, then collapses consecutive whitespace.
    """
    if not text or not isinstance(text, str):
        return ""
    cleaned = re.sub(r"[^a-z0-9\s]", " ", text.lower())
    return re.sub(r"\s+", " ", cleaned).strip()


def segment_inbound(bubbles: list[str]) -> list[str]:
    """
    Splits bubbles by punctuation boundaries [.?!;\n]+, cleans each segment,
    and returns non-empty candidate clauses.
    """
    if not bubbles:
        return []
    segments: list[str] = []
    for b in bubbles:
        if not b or not isinstance(b, str):
            continue
        parts = PUNCTUATION_SPLIT_REGEX.split(b)
        for p in parts:
            cleaned = clean_segment(p)
            if cleaned:
                segments.append(cleaned)
    return segments


def match_tier1_patterns(
    patterns: list[dict] | list[Any],
    bubbles: list[str],
    threshold: float = RAPIDFUZZ_DEFAULT_THRESHOLD,
) -> tuple[dict | Any | None, float]:
    """
    Evaluates trigger phrases against cleaned segments using rapidfuzz fuzz.ratio.
    Returns (matched_pattern, score) if score >= threshold, or (None, 0.0) if no match.
    """
    if not patterns or not bubbles:
        return None, 0.0

    segments = segment_inbound(bubbles)
    if not segments:
        return None, 0.0

    best_pattern = None
    best_score = 0.0

    for pat in patterns:
        triggers = (
            pat.get("trigger_phrases")
            if hasattr(pat, "get")
            else pat["trigger_phrases"]
        )
        if not triggers:
            continue
        for trig in triggers:
            if not trig or not isinstance(trig, str):
                continue
            c_trig = normalize_text(trig)
            raw_trig = trig.lower().strip()

            for seg in segments:
                c_seg = normalize_text(seg)
                raw_seg = seg.lower().strip()

                score_norm = float(fuzz.ratio(c_trig, c_seg))
                score_raw = float(fuzz.ratio(raw_trig, raw_seg))
                score = max(score_norm, score_raw)

                if score >= threshold and score > best_score:
                    best_score = score
                    best_pattern = pat
                    if best_score == 100.0:
                        return best_pattern, 100.0

    if best_pattern is not None:
        return best_pattern, best_score
    return None, 0.0
