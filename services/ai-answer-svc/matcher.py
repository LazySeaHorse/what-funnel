"""
Modular matching helper for AI Cascade Tier 1 matching engine.
Provides clause segmentation, conversational filler stripping,
punctuation normalization, content token extraction, word root matching,
token sort ratio, coverage-guarded rapidfuzz pattern matching,
and a safety & escalation guard.
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

# ==============================================================================
# Escalation & Safety Guard Patterns
# ==============================================================================

# Compiled regex patterns for a deliberately small set of genuinely safety-,
# complaint-, dispute- and legal-oriented signals. Any hit bypasses every automatic
# answer stage (pattern, embedding, RAG) and flags the conversation for a human.
# Merely out-of-scope topics (hiring, niche products, dietary questions, ...) are
# intentionally NOT listed here: they fall through on low match confidence instead.
ESCALATION_PATTERNS: list[re.Pattern] = [
    # 1. Acute medical emergencies, severe distress, and trauma
    re.compile(r"\bbleed(ing)?\b", re.IGNORECASE),
    re.compile(r"\bthrobbing\b", re.IGNORECASE),
    re.compile(r"\b(severe|unbearable|acute|extreme|excruciating)\s+(swelling|pain|distress|ache)\b", re.IGNORECASE),
    re.compile(r"\bswelling\b", re.IGNORECASE),
    re.compile(r"\b(tooth|teeth|bone|arm|leg|jaw)\s+(broke|broken|fractured)\b", re.IGNORECASE),
    re.compile(r"\b(broke|broken|fractured)\s+(my|a|the)\s+(tooth|teeth|bone|arm|leg|jaw)\b", re.IGNORECASE),
    re.compile(r"\binjur(y|ed|ies)\b", re.IGNORECASE),
    re.compile(r"\banaphylaxis\b", re.IGNORECASE),
    re.compile(r"\bpoison(ing|ed)?\b", re.IGNORECASE),
    re.compile(r"\bcertified\s+allergen[- ]free\b", re.IGNORECASE),
    re.compile(r"\b(having|in|this\s+is|it(?:\x27|)s)\s+(an?\s+)?emergency\b", re.IGNORECASE),
    re.compile(r"\bemergency\s*!", re.IGNORECASE),
    re.compile(r"\bmedical\s+emergency\b", re.IGNORECASE),
    re.compile(r"\burgent\s+emergency\b", re.IGNORECASE),
    re.compile(r"\bemergency\b(?!\s+(?:dental\s+)?(?:appointments?|services?|care|policy|hours?|dentist))\b", re.IGNORECASE),
    re.compile(r"\burgent\b(?!\s+(?:care\s+hours?|appointments?))\b", re.IGNORECASE),

    # 2. Severe service failures, no-shows, and complaints
    re.compile(r"\bwaited\b.*\b\d+\s*hours?\b", re.IGNORECASE),
    re.compile(r"\bwaiting\s+(?:for\s+)?\b.*\b\d+\s*hours?\b", re.IGNORECASE),
    re.compile(r"\b(never\s+showed(\s+up)?|no[\s-]show(s)?|(nobody|no\s+one)\s+showed(\s+up)?)\b", re.IGNORECASE),
    re.compile(r"\b(nobody|no\s+one)\s+answered\b", re.IGNORECASE),
    re.compile(r"\bterrible\b", re.IGNORECASE),
    re.compile(r"\bhorrible\b", re.IGNORECASE),
    re.compile(r"\bunacceptable\b", re.IGNORECASE),
    re.compile(r"\bcatastrophic\b", re.IGNORECASE),
    re.compile(r"\bcomplain(t|s|ing)?\b", re.IGNORECASE),

    # 3. Financial / contractual disputes & refund demands
    re.compile(r"(?<!\bpolicy\s)(?<!\bterms\s)\brefund(s)?\b(?!\s+policy\b)(?!\s+terms\b)", re.IGNORECASE),
    re.compile(r"\b(demand|want|need|get|issue|request)\s+(a\s+)?(?:full\s+)?refund\b", re.IGNORECASE),
    re.compile(r"\brefund\s+(me|my|immediately|now)\b", re.IGNORECASE),
    re.compile(r"\bunauthorized\b", re.IGNORECASE),
    re.compile(r"\bdisput(e|es|ed|ing)\b", re.IGNORECASE),
    re.compile(r"\bchargeback\b", re.IGNORECASE),
    re.compile(r"\bfraud(ulent)?\b", re.IGNORECASE),
    re.compile(r"\bovercharg(ed|ing|es)?\b", re.IGNORECASE),
    re.compile(r"\b(cancel|cancelling)\s+(my|the|our)\s+(order|wedding|cake|appointment|booking|reservation|subscription|contract)\b", re.IGNORECASE),
    re.compile(r"\b(need|want|would\s+like)\s+to\s+cancel\b", re.IGNORECASE),
    re.compile(r"\bplease\s+cancel\b", re.IGNORECASE),
    re.compile(r"\bcancel(l?ation)?\b(?!\s+(?:policy|fee|terms))\b", re.IGNORECASE),

    # 4. Legal threats
    re.compile(r"\blawyer(s)?\b", re.IGNORECASE),
    re.compile(r"\bsue\b", re.IGNORECASE),
    re.compile(r"\bsuing\b", re.IGNORECASE),
    re.compile(r"\blegal\s+(action|counsel|proceedings?|representation|recourse)\b", re.IGNORECASE),
    re.compile(r"\battorney(s)?\b", re.IGNORECASE),
    re.compile(r"\btake\s+you\s+to\s+court\b", re.IGNORECASE),
    re.compile(r"\bsee\s+you\s+in\s+court\b", re.IGNORECASE),
]


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


QUESTION_START_REGEX = re.compile(
    r"^(what|whats|when|where|who|whom|whose|which|why|how|do|does|did|can|could|will|would|"
    r"should|is|are|was|were|have|has|may|might|shall|am)\b"
)
# Fillers that introduce a question and are stripped by clean_segment().
QUESTION_FILLER_REGEX = re.compile(
    r"^(can\s+you\s+tell\s+me|could\s+you\s+tell\s+me|do\s+you\s+know|i\s+was\s+wondering|"
    r"just\s+wondering|i\s+wanted\s+to\s+ask|i(?:\x27|)d\s+like\s+to\s+(ask|know)|"
    r"i\s+would\s+like\s+to\s+(ask|know)|i\s+have\s+a\s+question|quick\s+question)\b"
)
_SPLIT_WITH_DELIMS = re.compile(r"([\n.?!;]+)")


def _is_question(raw_part: str, delimiter: str, cleaned: str) -> bool:
    if "?" in delimiter:
        return True
    raw = raw_part.strip().lower()
    return bool(
        QUESTION_START_REGEX.match(cleaned)
        or QUESTION_START_REGEX.match(raw)
        or QUESTION_FILLER_REGEX.match(raw)
    )


def segment_inbound_detailed(bubbles: list[str]) -> list[tuple[str, bool]]:
    """
    Like segment_inbound() but also reports whether each cleaned clause is a question
    (ends with '?', starts with an interrogative/auxiliary word, or was introduced by a
    question filler such as "can you tell me").
    """
    if not bubbles:
        return []
    segments: list[tuple[str, bool]] = []
    for b in bubbles:
        if not b or not isinstance(b, str):
            continue
        pieces = _SPLIT_WITH_DELIMS.split(b)
        for i in range(0, len(pieces), 2):
            part = pieces[i]
            delimiter = pieces[i + 1] if i + 1 < len(pieces) else ""
            cleaned = clean_segment(part)
            if cleaned:
                segments.append((cleaned, _is_question(part, delimiter, cleaned)))
    return segments


def segment_inbound(bubbles: list[str]) -> list[str]:
    """
    Splits bubbles by punctuation boundaries [.?!;\n]+, cleans each segment,
    and returns non-empty candidate clauses.
    """
    return [seg for seg, _ in segment_inbound_detailed(bubbles)]


def is_escalation(text_or_bubbles: str | list[str]) -> bool:
    """
    Checks whether any escalation pattern is detected in the inbound message.
    Accepts either a single string or a list of bubble strings.

    Returns True if an acute emergency, severe service failure/complaint, refund or
    contractual dispute, or legal threat is detected; False otherwise.
    """
    if not text_or_bubbles:
        return False
    if isinstance(text_or_bubbles, str):
        full_text = text_or_bubbles
    else:
        full_text = " ".join(b for b in text_or_bubbles if b and isinstance(b, str))
    if not full_text.strip():
        return False

    for pat in ESCALATION_PATTERNS:
        if pat.search(full_text):
            return True
    return False


def _score_pair(c_trig: str, raw_trig: str, t_tokens: set[str], seg: str) -> float:
    """Best matching score (0.0 when no strategy qualifies) of one trigger phrase against one segment."""
    c_seg = normalize_text(seg)
    raw_seg = seg.lower().strip()
    best = 0.0

    # 1. Exact or near-exact Levenshtein ratio (typos, minor variance)
    r_score = max(float(fuzz.ratio(c_trig, c_seg)), float(fuzz.ratio(raw_trig, raw_seg)))
    if r_score >= 88.0:
        best = max(best, r_score)
        if best == 100.0:
            return best

    # 2. Token Sort Ratio (word reordering) with length safeguard
    tsr_score = float(fuzz.token_sort_ratio(c_trig, c_seg))
    max_len = max(len(c_trig), len(c_seg))
    len_ratio = min(len(c_trig), len(c_seg)) / max_len if max_len > 0 else 0.0
    if tsr_score >= 85.0 and len_ratio >= 0.40:
        best = max(best, tsr_score)

    # 3. Content Token Matching with Length & Coverage Safeguards
    s_tokens = set(extract_content_tokens(c_seg))
    if not t_tokens or not s_tokens:
        return best

    overlap_cnt = count_token_overlap(t_tokens, s_tokens)
    t_cov = overlap_cnt / len(t_tokens)
    s_cov = overlap_cnt / len(s_tokens)

    # Terse input match: 1-2 content words completely matched in trigger tokens
    # (e.g. "hours" -> "clinic hours", "parking" -> "parking options")
    if len(s_tokens) <= 2 and overlap_cnt == len(s_tokens) and all(len(w) > 2 for w in s_tokens):
        best = max(best, 92.0)

    # Conversational coverage match:
    # Trigger concepts are substantially contained in query clause
    if (t_cov >= 0.50 and s_cov >= 0.35) or (t_cov == 1.0 and s_cov >= 0.25):
        best = max(best, max(88.0, float(fuzz.token_set_ratio(c_trig, c_seg))))
    return best


def match_tier1_patterns(
    patterns: list[dict] | list[Any],
    bubbles: list[str],
    threshold: float = RAPIDFUZZ_DEFAULT_THRESHOLD,
) -> tuple[dict | Any | None, float]:
    """
    Evaluates trigger phrases against cleaned segments using a combination of:
    1. Safety / Escalation Guard: Immediate rejection if acute emergency, complaint, dispute
       or legal threat is detected (callers should also check is_escalation() themselves
       before any other automatic answer stage).
    2. Levenshtein ratio (fuzz.ratio >= 88.0)
    3. Token sort ratio with length safeguard (fuzz.token_sort_ratio >= 85.0 with len_ratio >= 0.40)
    4. Terse keyword matching (1-2 content words completely matched in trigger tokens)
    5. Conversational coverage matching (trigger concepts substantially covered in query clause,
       e.g. t_cov >= 0.50 and s_cov >= 0.35 or t_cov == 1.0 and s_cov >= 0.25, scored with fuzz.token_set_ratio)

    A pattern is only returned when it covers EVERY question segment of the inbound text
    (score >= threshold each). If another question in the message is not answered by that
    same pattern (e.g. "What are your hours? Also can you fix my broken widget?"), the
    message must not be auto-answered and falls through (None, 0.0).
    Non-question statements ("my name is Sam") do not need to be covered.

    Returns (matched_pattern, score) where score is the pattern's best segment score, or
    (None, 0.0) if no match, an unanswered question remains, or the text is escalated.
    """
    if not patterns or not bubbles:
        return None, 0.0

    if is_escalation(bubbles):
        return None, 0.0

    segments = segment_inbound_detailed(bubbles)
    if not segments:
        return None, 0.0

    candidates: list[tuple[float, int, Any]] = []
    for idx, pat in enumerate(patterns):
        triggers = (
            pat.get("trigger_phrases")
            if hasattr(pat, "get")
            else pat["trigger_phrases"]
        )
        if not triggers:
            continue
        seg_best = [0.0] * len(segments)
        for trig in triggers:
            if not trig or not isinstance(trig, str):
                continue
            c_trig = normalize_text(trig)
            raw_trig = trig.lower().strip()
            t_tokens = set(extract_content_tokens(c_trig))
            for i, (seg, _) in enumerate(segments):
                score = _score_pair(c_trig, raw_trig, t_tokens, seg)
                if score > seg_best[i]:
                    seg_best[i] = score
        best = max(seg_best)
        if best < threshold:
            continue
        # Every question clause must be answered by this same pattern.
        if any(is_q and seg_best[i] < threshold for i, (_, is_q) in enumerate(segments)):
            continue
        candidates.append((best, idx, pat))

    if not candidates:
        return None, 0.0
    # Highest score wins; earlier pattern wins ties.
    best_score, _, best_pattern = min(candidates, key=lambda c: (-c[0], c[1]))
    return best_pattern, best_score
