"""Deterministic groundedness checks for generated (KB) answers.

An answer may only state facts that appear in the sources it cites. This module verifies the facts
that can be checked mechanically, with proper tokenisation (no substring matching):

* quantities: numbers, prices, percentages, times and durations, matched as WHOLE numbers with a
  compatible unit; spelled-out numbers ("five days") are converted first;
* contact and locator facts: e-mail addresses and URLs / domains, matched as whole tokens;
* named terms: capitalised words inside a sentence (brands, people, places, days) must occur in the
  cited sources (a name only the customer mentioned does not count).

What it cannot check: facts without a number, locator or proper name ("we offer refunds", "free
parking", a policy stated in the opposite way). Those rely on the model's citations and needs_human
flag, which is why generated answers default to draft-only (see AI_RAG_AUTO_SEND).
"""

from __future__ import annotations

import re
from dataclasses import dataclass
from typing import Optional

from untrusted import normalize_text

# ---------------------------------------------------------------------------
# Number words
# ---------------------------------------------------------------------------

_UNITS = {
    "two": 2, "three": 3, "four": 4, "five": 5, "six": 6, "seven": 7, "eight": 8, "nine": 9, "ten": 10,
    "eleven": 11, "twelve": 12, "thirteen": 13, "fourteen": 14, "fifteen": 15, "sixteen": 16,
    "seventeen": 17, "eighteen": 18, "nineteen": 19,
}
_TENS = {"twenty": 20, "thirty": 30, "forty": 40, "fifty": 50, "sixty": 60, "seventy": 70, "eighty": 80, "ninety": 90}
# "one" is also a pronoun ("one of our staff"), so it only counts before a measure word.
_MEASURE_WORDS = {
    "day", "days", "week", "weeks", "month", "months", "year", "years", "hour", "hours", "minute", "minutes",
    "min", "mins", "hr", "hrs", "business", "working", "calendar",
}
_NUMBER_WORD_RE = re.compile(
    r"\b(?:(?:" + "|".join(sorted(list(_UNITS) + list(_TENS), key=len, reverse=True)) + r")"
    r"(?:[\s-]+(?:" + "|".join(_UNITS) + r"))?|one(?=\s+(?:" + "|".join(sorted(_MEASURE_WORDS)) + r")\b)|a\s+dozen|hundred)\b"
)


def _words_to_digits(text: str) -> str:
    def convert(match: re.Match) -> str:
        phrase = match.group(0)
        if phrase == "one":
            return "1"
        if phrase.startswith("a dozen"):
            return "12"
        if phrase == "hundred":
            return "100"
        total = 0
        for part in re.split(r"[\s-]+", phrase):
            total += _UNITS.get(part, 0) + _TENS.get(part, 0)
        return str(total)

    return _NUMBER_WORD_RE.sub(convert, text)


# ---------------------------------------------------------------------------
# Quantities
# ---------------------------------------------------------------------------

_CURRENCY = {"$": "$", "€": "$", "£": "$", "usd": "$", "eur": "$", "gbp": "$"}
_MEASURE_UNITS = {
    "day": "day", "days": "day", "week": "week", "weeks": "week", "month": "month", "months": "month",
    "year": "year", "years": "year", "hour": "hour", "hours": "hour", "hr": "hour", "hrs": "hour",
    "minute": "minute", "minutes": "minute", "min": "minute", "mins": "minute",
    "km": "km", "kilometer": "km", "kilometers": "km", "kilometre": "km", "kilometres": "km",
    "mile": "mile", "miles": "mile", "kg": "kg", "lb": "lb", "lbs": "lb", "guest": "guest", "guests": "guest",
    "people": "guest", "person": "guest", "seat": "seat", "seats": "seat",
}
_QUANTITY_RE = re.compile(
    r"(?P<cur>[$€£])?\s?(?<![\w.,])(?P<num>\d{1,3}(?:,\d{3})+(?:\.\d+)?|\d+(?:\.\d+)?)(?:(?P<colon>:\d{2}))?"
    r"(?:\s?(?P<suffix>%|percent\b|(?:a\.?m|p\.?m)\b\.?|(?:st|nd|rd|th)\b|[a-z]+\b))?"
)


@dataclass(frozen=True)
class Quantity:
    value: str
    unit: str = ""


def _canonical_number(raw: str, colon: Optional[str]) -> str:
    raw = raw.replace(",", "")
    if "." in raw:
        whole, frac = raw.split(".", 1)
        frac = frac.rstrip("0")
        raw = (whole.lstrip("0") or "0") + ("." + frac if frac else "")
    else:
        raw = raw.lstrip("0") or "0"
    if colon and colon != ":00":
        return raw + colon
    return raw


def extract_quantities(text: str) -> list[Quantity]:
    text = _words_to_digits(normalize_text(text).lower())
    quantities: list[Quantity] = []
    for match in _QUANTITY_RE.finditer(text):
        value = _canonical_number(match.group("num"), match.group("colon"))
        suffix = (match.group("suffix") or "").replace(".", "")
        if match.group("cur"):
            unit = "$"
        elif suffix in ("%", "percent"):
            unit = "%"
        elif suffix in ("am", "pm"):
            unit = suffix
        else:
            unit = _MEASURE_UNITS.get(suffix, "")
        quantities.append(Quantity(value, unit))
    return quantities


def _compatible(claimed: Quantity, source: Quantity) -> bool:
    """Is a number in the answer supported by a number in a cited source?

    The value must be the same whole number. A claimed price or percentage needs a source price or
    percentage (a bare "15" does not support "$15"). For times and durations a source number without
    a unit is accepted ("9 to 5 pm" supports "9am"), but a source that states a different unit is not.
    """
    if claimed.value != source.value:
        return False
    if claimed.unit in ("$", "%"):
        return source.unit == claimed.unit
    return not claimed.unit or not source.unit or claimed.unit == source.unit


def unsupported_quantities(answer: str, source_text: str) -> list[Quantity]:
    sources = extract_quantities(source_text)
    return [q for q in extract_quantities(answer) if not any(_compatible(q, s) for s in sources)]


# ---------------------------------------------------------------------------
# Locators: e-mail, URLs and domains
# ---------------------------------------------------------------------------

_EMAIL_RE = re.compile(r"[\w.+-]+@[\w-]+(?:\.[\w-]+)+")
_URL_RE = re.compile(
    r"(?:https?://|www\.)[^\s,;)\]>]+|"
    r"\b[a-z0-9][a-z0-9-]*(?:\.[a-z0-9-]+)*\.(?:[a-z]{2,24})(?:/[^\s,;)\]>]*)?",
    re.I,
)


def _canonical_locator(raw: str) -> str:
    raw = raw.lower().rstrip(".,;:!?)")
    raw = re.sub(r"^https?://", "", raw)
    raw = re.sub(r"^www\.", "", raw)
    return raw.rstrip("/")


def extract_locators(text: str) -> set[str]:
    text = normalize_text(text)
    found: set[str] = set()
    for match in _EMAIL_RE.finditer(text):
        found.add(_canonical_locator(match.group(0)))
    without_emails = _EMAIL_RE.sub(" ", text)
    for match in _URL_RE.finditer(without_emails):
        candidate = _canonical_locator(match.group(0))
        host = candidate.split("/", 1)[0]
        # "e.g", "i.e" and abbreviations such as "p.m" are not hosts
        if len(host.rsplit(".", 1)[-1]) >= 2 and not re.fullmatch(r"(?:[a-z]\.)+[a-z]", host):
            found.add(candidate)
    return found


def unsupported_locators(answer: str, source_text: str) -> list[str]:
    sources = extract_locators(source_text)
    return sorted(loc for loc in extract_locators(answer) if loc not in sources)


# ---------------------------------------------------------------------------
# Named terms
# ---------------------------------------------------------------------------

_WORD_RE = re.compile(r"[^\W\d_][\w'’]*", re.UNICODE)
_SENTENCE_START = re.compile(r"(?:^|[.!?]\s+|\n\s*[-*\d.)]*\s*)$")


def _stem(word: str) -> str:
    word = word.lower().replace("’", "'")
    for suffix in ("'s", "s'"):
        if word.endswith(suffix):
            word = word[: -len(suffix)]
    if len(word) > 3 and word.endswith("s") and not word.endswith("ss"):
        word = word[:-1]
    return word


def _vocabulary(text: str) -> set[str]:
    return {_stem(m.group(0)) for m in _WORD_RE.finditer(normalize_text(text))}


def named_terms(answer: str) -> list[str]:
    """Capitalised words that are not at the start of a sentence (brands, names, places, days)."""
    text = normalize_text(answer)
    terms = []
    for match in _WORD_RE.finditer(text):
        word = match.group(0)
        if not word[0].isupper() or word == "I" or word.startswith("I'"):
            continue
        if _SENTENCE_START.search(text[: match.start()]):
            continue
        terms.append(word)
    return terms


def unsupported_named_terms(answer: str, source_text: str) -> list[str]:
    """Named terms must come from the sources. A name only the CUSTOMER mentioned does not count:
    "yes, we accept Cigna" is exactly the answer that must not be invented."""
    allowed = _vocabulary(source_text)
    return sorted({t for t in named_terms(answer) if _stem(t) not in allowed})


# ---------------------------------------------------------------------------
# Combined check
# ---------------------------------------------------------------------------

def check_facts(answer: str, source_text: str) -> tuple[bool, str]:
    """(ok, reason). reason names the first unsupported fact class and value."""
    quantities = unsupported_quantities(answer, source_text)
    if quantities:
        first = quantities[0]
        return False, f"ungrounded_number:{first.value}{first.unit}"[:80]
    locators = unsupported_locators(answer, source_text)
    if locators:
        return False, f"ungrounded_locator:{locators[0]}"[:80]
    terms = unsupported_named_terms(answer, source_text)
    if terms:
        return False, f"ungrounded_term:{terms[0]}"[:80]
    return True, "grounded"
