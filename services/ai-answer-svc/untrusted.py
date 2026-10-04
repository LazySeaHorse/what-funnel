"""Normalisation of untrusted customer text before it is used in a prompt or in deterministic checks."""

from __future__ import annotations

import re
import unicodedata

MAX_BUBBLE_CHARS = 600

# Characters in these Unicode categories are never legitimate message content for our purposes:
# Cf = format (zero-width space/joiner, bidi embeddings and isolates, BOM, soft hyphen, tag characters),
# Cc = control (other than newline and tab), Zl/Zp = line/paragraph separators (mapped to a newline).
_DROPPED_CATEGORIES = {"Cf", "Cc"}

# Delimiter look-alikes: a run of two or more angle or square brackets, with optional whitespace
# between them, can only be an attempt to forge our <<< >>> / [[ ]] markers (a lone "<3" or "a > b"
# is untouched). The marker words are removed for the same reason.
_BRACKET_RUN = re.compile(r"(?:[<>\[\]{}]\s*){2,}")
_MARKER_WORDS = re.compile(
    r"(?:UNTRUSTED|CUSTOMER|CONVERSATION)[\s_\-]*(?:DATA[\s_\-]*)?(?:START|END)", re.I
)


def normalize_text(text: str) -> str:
    """NFKC-normalise and drop invisible formatting/control characters.

    NFKC folds full-width and other compatibility forms (for example full-width brackets and
    letters) to their ASCII equivalents, so look-alike delimiters are caught by the checks below.
    """
    text = unicodedata.normalize("NFKC", str(text))
    out = []
    for char in text:
        category = unicodedata.category(char)
        if category in ("Zl", "Zp"):
            out.append("\n")
        elif category in _DROPPED_CATEGORIES and char not in "\n\t":
            continue
        else:
            out.append(char)
    return "".join(out)


def sanitize_untrusted(text: str, max_chars: int = MAX_BUBBLE_CHARS) -> str:
    """Normalise, neutralise delimiter look-alikes and cap length. Content stays readable."""
    text = normalize_text(text)
    text = _BRACKET_RUN.sub(" ", text)
    text = _MARKER_WORDS.sub(" ", text)
    text = text.strip()
    if len(text) > max_chars:
        text = text[:max_chars] + "..."
    return text
