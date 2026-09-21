"""
Modular matching helper for AI Cascade Tier 1 matching engine.
Provides clause segmentation, conversational filler stripping,
punctuation normalization, content token extraction, word root matching,
token sort ratio, and coverage-guarded rapidfuzz pattern matching.
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
RAPIDFUZZ_DEFAULT_THRESHOLD = 85.0

# Common grammar, preposition, auxiliary, pronoun, and conversational stopwords
STOPWORDS: set[str] = {
    "a", "an", "the", "in", "on", "at", "for", "to", "of", "with", "by", "from",
    "is", "are", "was", "were", "be", "been", "being", "do", "does", "did",
    "have", "has", "had", "i", "you", "he", "she", "it", "we", "they", "my", "your",
    "our", "their", "what", "which", "who", "whom", "this", "that", "these", "those",
    "am", "can", "could", "would", "should", "there", "here", "guys", "please",
    "tell", "me", "us", "any", "some", "about", "and", "or", "so", "time",
    "without", "out", "get",
}


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


def extract_content_tokens(text: str) -> list[str]:
    """
    Extracts content tokens from text, ignoring stopwords and single-character tokens.
    """
    if not text or not isinstance(text, str):
        return []
    norm = normalize_text(text)
    return [w for w in norm.split() if w not in STOPWORDS and len(w) > 1]


def token_match(t1: str, t2: str) -> bool:
    """
    Matches words taking into account plurals, inflections, and stems.
    Examples:
        - "cakes" ~ "cake"
        - "ebikes" ~ "ebike"
        - "located" ~ "location"
        - fuzz.ratio >= 80 for words >= 4 chars
    """
    if not t1 or not t2:
        return False
    t1 = t1.lower().strip()
    t2 = t2.lower().strip()
    if t1 == t2:
        return True
    if t1 + "s" == t2 or t2 + "s" == t1 or t1 + "es" == t2 or t2 + "es" == t1:
        return True
    if len(t1) >= 4 and len(t2) >= 4:
        if fuzz.ratio(t1, t2) >= 80.0:
            return True
        if t1[:4] == t2[:4]:
            return True
    return False


def count_token_overlap(set1: set[str], set2: set[str]) -> int:
    """
    Counts semantic token overlap between two sets.
    Matches words accounting for plurals, inflections, and stems.
    Each word in set2 is matched at most once.
    """
    if not set1 or not set2:
        return 0
    cnt = 0
    matched_s2: set[str] = set()
    for w1 in set1:
        for w2 in set2:
            if w2 not in matched_s2 and token_match(w1, w2):
                cnt += 1
                matched_s2.add(w2)
                break
    return cnt


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
    Evaluates trigger phrases against cleaned segments using a combination of:
    1. Levenshtein ratio (fuzz.ratio >= 88.0)
    2. Token sort ratio with length safeguard (fuzz.token_sort_ratio >= 85.0 with len_ratio >= 0.40)
    3. Terse keyword matching (1-2 content words completely matched in trigger tokens)
    4. Conversational coverage matching (trigger concepts substantially covered in query clause,
       e.g. t_cov >= 0.50 and s_cov >= 0.35 or t_cov == 1.0 and s_cov >= 0.25, scored with fuzz.token_set_ratio)

    Returns (matched_pattern, score) if score >= threshold (default 85.0), or (None, 0.0) if no match.
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
            t_tokens = set(extract_content_tokens(c_trig))

            for seg in segments:
                c_seg = normalize_text(seg)
                raw_seg = seg.lower().strip()
                s_tokens = set(extract_content_tokens(c_seg))

                # 1. Exact or near-exact Levenshtein ratio (typos, minor variance)
                r_norm = float(fuzz.ratio(c_trig, c_seg))
                r_raw = float(fuzz.ratio(raw_trig, raw_seg))
                r_score = max(r_norm, r_raw)
                if r_score >= 88.0 and r_score > best_score:
                    best_score = r_score
                    best_pattern = pat
                    if best_score == 100.0:
                        return best_pattern, 100.0
                    continue

                # 2. Token Sort Ratio (word reordering) with length safeguard
                tsr_score = float(fuzz.token_sort_ratio(c_trig, c_seg))
                max_len = max(len(c_trig), len(c_seg))
                len_ratio = min(len(c_trig), len(c_seg)) / max_len if max_len > 0 else 0.0
                if tsr_score >= 85.0 and len_ratio >= 0.40 and tsr_score > best_score:
                    best_score = tsr_score
                    best_pattern = pat
                    continue

                # 3. Content Token Matching with Length & Coverage Safeguards
                if not t_tokens or not s_tokens:
                    continue

                overlap_cnt = count_token_overlap(t_tokens, s_tokens)
                t_cov = overlap_cnt / len(t_tokens)
                s_cov = overlap_cnt / len(s_tokens)

                # Terse input match: 1-2 content words completely matched in trigger tokens
                # (e.g. "hours" -> "clinic hours", "parking" -> "parking options")
                if len(s_tokens) <= 2 and overlap_cnt == len(s_tokens):
                    if all(len(w) > 2 for w in s_tokens) and 92.0 > best_score:
                        best_score = 92.0
                        best_pattern = pat
                        continue

                # Conversational coverage match:
                # Trigger concepts are substantially contained in query clause
                if (t_cov >= 0.50 and s_cov >= 0.35) or (t_cov == 1.0 and s_cov >= 0.25):
                    score = max(88.0, float(fuzz.token_set_ratio(c_trig, c_seg)))
                    if score > best_score:
                        best_score = score
                        best_pattern = pat

    if best_score >= threshold:
        return best_pattern, best_score
    return None, 0.0
