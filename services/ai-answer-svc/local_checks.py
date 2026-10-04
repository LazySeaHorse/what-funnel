"""Deterministic local checks on untrusted customer text: safety backstop, greetings,
acknowledgements and context-less fragments. No model call, no keyword scoring of topics."""

from __future__ import annotations

import re
import unicodedata

from untrusted import normalize_text

# ---------------------------------------------------------------------------
# Safety backstop: legal threats and acute emergencies, nothing else.
# A hit means handoff whatever the model says. Patterns require THREAT or EMERGENCY context, not a
# bare topic word: "my lawyer will contact you" hits, "my lawyer friend recommends you" does not;
# "I smell gas" hits, "do you sell gas grills" does not. No cancel/urgent/refund/complaint words.
# ---------------------------------------------------------------------------

_APOS = r"['\u2019]?"
_LEGAL_THREAT = (
    r"legal action|legal proceedings|legal team|legal counsel|see you in court|small claims|class action",
    # "sue" is also a name, so the verb needs a subject or "to"/modal before it (or be "suing")
    r"(?:i|we|will|would|can|could|should|may|might|must|gonna|to|ll)\s+sue\s+(?:you|us|your|the|this|them|over)\b",
    r"suing\s+(?:you|us|your|the|this|them|over)\b",
    r"(?:file|filing|filed|bring|bringing|start|starting)\s+(?:a\s+)?(?:lawsuit|law suit|legal)",
    r"(?:my|our|the)\s+(?:lawyers?|attorneys?|solicitors?)\s+(?:will|would|is|are|has|have|had|can|should|to|says?|said|advised|"
    + _APOS + r"ll|wants?|needs?)\b",
    r"(?:call|calling|contact|contacting|hire|hired|hiring|get|getting|involve|involving|speak(?:ing)?\s+to|talk(?:ing)?\s+to)\s+"
    r"(?:a|my|an|our)\s+(?:lawyers?|attorneys?|solicitors?)\b(?!\s+(?:friend|referral|number|directory))",
    r"lawyer(?:ed)?\s+up",
)
_EMERGENCY = (
    # breathing, heart, stroke, consciousness, seizures, choking, bleeding
    r"can" + _APOS + r"t\s+breathe|cannot\s+breathe|not\s+breathing|stopped\s+breathing|trouble\s+breathing|difficulty\s+breathing",
    r"chest\s+(?:pain|tightness)|heart\s+attack|having\s+a\s+stroke|(?:is|are|was)\s+unconscious|unresponsive",
    r"(?:having|had)\s+a\s+seizure|(?:is|are|was|am)\s+choking|choking\s+on",
    r"overdos(?:e|ed|ing)|(?:went|going|gone)\s+into\s+anaphyla\w*|(?:having|in)\s+(?:an?\s+)?anaphyla\w*(?:\s+shock)?|anaphylactic\s+shock",
    r"won" + _APOS + r"t\s+stop\s+bleeding|bleeding\s+(?:heavily|badly|profusely)|losing\s+(?:a\s+lot\s+of\s+)?blood",
    r"call(?:ing|ed)?\s+(?:911|999|112|an\s+ambulance|the\s+ambulance)|need\s+an\s+ambulance",
    r"suicid\w*|kill\s+myself|end\s+my\s+life|want\s+to\s+die",
    # gas, carbon monoxide, fire
    r"gas\s+leak|leaking\s+gas|smell(?:s|ed|ing)?\s+(?:of\s+|like\s+)?gas|gas\s+smell|carbon\s+monoxide",
    r"(?:on|caught|catching)\s+fire|fire\s+(?:broke\s+out|started)|(?:is|are)\s+(?:smoking|burning)\s+(?:and|now|badly)|started\s+(?:to\s+)?smok(?:e|ing)",
)
_BACKSTOP_RE = re.compile(r"\b(?:" + "|".join(_LEGAL_THREAT + _EMERGENCY) + r")\b", re.I)


def backstop_hit(bubbles: list[str]) -> bool:
    return any(_BACKSTOP_RE.search(normalize_text(b)) for b in bubbles)


# ---------------------------------------------------------------------------
# Greeting detection (deterministic, anchored, whole message)
# ---------------------------------------------------------------------------
# Chosen over a router route because it needs no model call (zero latency and cost), is fully
# predictable and cannot be steered by customer text. The cost is a closed vocabulary: an unusual
# greeting ("yo yo") falls through to the router, which treats it as a message to hand off or ignore.

DEFAULT_GREETING_REPLY = "Hi! Thanks for reaching out. How can we help you today?"

_CORE_PHRASES = (
    "good morning", "good afternoon", "good evening", "good day", "good night",
    "hello", "hi", "hey", "hiya", "heya", "howdy", "greetings", "yo", "hola", "bonjour", "ciao",
    "namaste", "salam", "assalamu alaikum", "morning", "afternoon", "evening", "sup", "whats up",
    "what's up", "how are you", "hows it going", "how is it going",
    "anyone there", "anybody there", "anyone here", "anybody here", "is anyone there",
    "is anybody there", "is anyone here", "is anybody here", "is someone there", "is somebody there",
    "is there anyone", "is there anybody", "is there someone", "are you there", "are you here",
    "any one there", "hello anyone", "hi anyone", "hello is anyone there",
    "i have a question", "i have a quick question", "i have question", "quick question", "got a question",
    "can i ask a question", "can i ask something", "can i ask you something", "may i ask a question",
    "i need help", "need help", "can you help me", "can someone help me", "can anyone help me",
    "could you help me", "help me please", "help",
)
_FILLER_PHRASES = (
    "there", "team", "guys", "everyone", "everybody", "all", "folks", "sir", "madam", "maam", "mam",
    "support", "friends", "please", "pls", "plz", "again",
)
_PHRASES_BY_LENGTH = sorted(
    [(p, True) for p in _CORE_PHRASES] + [(p, False) for p in _FILLER_PHRASES],
    key=lambda item: -len(item[0].split()),
)
_MAX_GREETING_TOKENS = 8


def _normalize_greeting(text: str) -> str:
    text = normalize_text(text).lower().replace("’", "'")
    text = re.sub(r"(.)\1{2,}", r"\1", text)  # heyyyy -> hey
    text = re.sub(r"[^a-z0-9' ]+", " ", text)  # punctuation, emoji
    return " ".join(text.split())


def is_greeting_message(text: str) -> bool:
    """True if the whole message is only a greeting / conversation starter (no actual request)."""
    words = _normalize_greeting(text).split()
    if not words or len(words) > _MAX_GREETING_TOKENS:
        return False
    # "whats" / "hows" without apostrophes
    words = ["what's" if w == "whats" else "hows" if w == "hows" else w for w in words]
    position = 0
    seen_core = False
    while position < len(words):
        for phrase, is_core in _PHRASES_BY_LENGTH:
            parts = phrase.split()
            if words[position:position + len(parts)] == parts:
                position += len(parts)
                seen_core = seen_core or is_core
                break
        else:
            return False
    return seen_core


def is_greeting_batch(bubbles: list[str]) -> bool:
    return bool(bubbles) and all(is_greeting_message(b) for b in bubbles)




# ---------------------------------------------------------------------------
# Acknowledgement detection: the guard for route=ignore
# ---------------------------------------------------------------------------
# The router may say "ignore" (nothing to answer), which drops the message without a human ever
# seeing it. That is only acceptable for a bare acknowledgement or pleasantry, so it is honoured
# only when this deterministic check agrees: the WHOLE message is made of closed-vocabulary
# acknowledgement phrases (thanks, ok, got it, bye, emoji, ...) or a greeting, with no question
# mark and no digits. Anything else ("i have cigna", "yes please", "ok thanks but I still want
# to speak to someone", "I love your terrible puns lol") is not dropped but flagged for a human.

_ACK_PHRASES = (
    "thanks", "thank you", "thank u", "thx", "thnx", "ty", "tysm", "many thanks", "thanks a lot",
    "thanks so much", "thank you so much", "thank you very much", "thanks very much", "much appreciated",
    "i appreciate it", "appreciate it", "appreciated",
    "ok", "okay", "okey", "k", "kk", "alright", "all right", "got it", "gotcha", "noted", "understood",
    "i see", "sure", "fine", "will do", "sounds good", "sounds great", "all good", "that works",
    "that is fine", "thats fine", "makes sense", "no problem", "no worries", "np",
    "great", "perfect", "cool", "awesome", "nice", "good", "excellent", "lovely", "wonderful", "amazing",
    "bye", "goodbye", "bye bye", "see you", "see ya", "see you soon", "talk later", "talk soon", "take care",
    "have a good day", "have a nice day", "have a great day", "have a good one", "good night",
    "cheers", "lol", "haha", "hahaha",
)
_ACK_FILLERS = ("so", "much", "very", "a", "lot", "again", "too", "you", "then", "all", "the", "help", "for", "your", "everything")
_ACK_PHRASES_BY_LENGTH = sorted(
    [(p, True) for p in _ACK_PHRASES] + [(p, False) for p in _ACK_FILLERS],
    key=lambda item: -len(item[0].split()),
)
_MAX_ACK_TOKENS = 8


def _consume_vocabulary(words: list[str], phrases: list[tuple[str, bool]]):
    """Greedy longest-phrase match over the whole word list.

    Returns None if some word is outside the vocabulary, else whether at least one core phrase
    (not just a filler) was seen.
    """
    position, seen_core = 0, False
    while position < len(words):
        for phrase, is_core in phrases:
            parts = phrase.split()
            if words[position:position + len(parts)] == parts:
                position += len(parts)
                seen_core = seen_core or is_core
                break
        else:
            return None
    return seen_core


def _is_emoji_only(text: str) -> bool:
    saw_symbol = False
    for char in normalize_text(text):
        category = unicodedata.category(char)
        if category.startswith(("L", "N")):
            return False
        if category in ("So", "Sk"):
            saw_symbol = True
        elif not (category.startswith(("Z", "P", "M", "C")) or category == "Sm"):
            return False
    return saw_symbol


def is_acknowledgement(text: str) -> bool:
    """True if the whole message is only an acknowledgement, pleasantry, farewell or greeting."""
    normalized = normalize_text(text)
    if "?" in normalized or re.search(r"\d", normalized):
        return False
    if _is_emoji_only(normalized):
        return True
    if is_greeting_message(normalized):
        return True
    words = _normalize_greeting(normalized).split()
    if not words or len(words) > _MAX_ACK_TOKENS:
        return False
    return bool(_consume_vocabulary(words, _ACK_PHRASES_BY_LENGTH))


# ---------------------------------------------------------------------------
# Context-less fragments
# ---------------------------------------------------------------------------
# A message that opens with a coordinating or continuing word ("and on saturdays?", "also gluten
# free?", "what about sunday") is elliptical: its meaning depends on an earlier turn. When the
# conversation has no earlier turn it cannot be answered by guessing a topic from the FAQ menu, so
# it goes to a human. This is a structural check (the opening function word), not a topic list.

_CONTINUATION_OPENERS = (
    "and", "also", "or", "but", "so", "then", "plus", "what about", "how about", "and what about",
    "and how about", "same for", "ok and", "okay and", "and also",
)
_MAX_FRAGMENT_TOKENS = 8


def is_elliptical_fragment(text: str) -> bool:
    words = _normalize_greeting(text).split()
    if not words or len(words) > _MAX_FRAGMENT_TOKENS:
        return False
    for opener in sorted(_CONTINUATION_OPENERS, key=lambda o: -len(o.split())):
        parts = opener.split()
        if words[: len(parts)] == parts and len(words) > len(parts):
            return True
    return False
