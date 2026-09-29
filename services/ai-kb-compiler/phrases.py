from typing import Iterable, Optional


def normalize_trigger_phrases(
    phrases: Optional[Iterable[str]],
    canonical_question: Optional[str] = None,
) -> list[str]:
    """Lower-case, whitespace-normalise and de-duplicate trigger phrases (order preserved).

    When canonical_question is given it is appended (normalised) if not already present, so the
    canonical wording always matches its own pattern.
    """
    result: list[str] = []
    seen: set[str] = set()
    candidates = list(phrases or [])
    if canonical_question:
        candidates.append(canonical_question)
    for phrase in candidates:
        if not isinstance(phrase, str):
            continue
        normalized = " ".join(phrase.lower().split())
        if normalized and normalized not in seen:
            seen.add(normalized)
            result.append(normalized)
    return result
