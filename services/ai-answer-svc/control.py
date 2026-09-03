from datetime import timedelta
from typing import Iterable, Literal


HUMAN_REVIEW_REPLY = (
    "This is an automated reply. We received your message and sent it to our "
    "support team. A team member will answer you soon."
)
UNANSWERED_WINDOW = timedelta(minutes=10)
COOLDOWN_DELAYS = {
    1: timedelta(seconds=30),
    2: timedelta(minutes=1),
    3: timedelta(minutes=2),
    4: timedelta(minutes=5),
}


def resolve_reply_mode(workspace_mode: str, reply_override: str) -> str:
    if reply_override == "disabled":
        return "disabled"
    if reply_override == "enabled":
        return "auto_send"
    return "auto_send" if workspace_mode == "auto_send" else "draft_only"


def next_cooldown_level(current_level: int, unanswered_count: int) -> int:
    if current_level <= 0:
        return 1
    if current_level == 1 and unanswered_count < 3:
        return 1
    return min(4, current_level + 1)


def transcript_within_byte_budget(
    messages: Iterable[tuple[str, str]], byte_budget: int = 1000
) -> str:
    """Return newest transcript data within a conservative model-token bound.

    A BPE token always represents at least one input byte. Bounding UTF-8 bytes
    therefore guarantees that the payload cannot exceed the same token count,
    while remaining provider-agnostic.
    """
    selected: list[str] = []
    remaining = byte_budget
    for role, text in reversed(list(messages)):
        if text == HUMAN_REVIEW_REPLY:
            continue
        line = f"{role}: {text.strip()}"
        encoded = line.encode("utf-8")
        if len(encoded) > remaining:
            encoded = encoded[-remaining:]
            while encoded and (encoded[0] & 0xC0) == 0x80:
                encoded = encoded[1:]
            line = encoded.decode("utf-8", errors="ignore")
        if line:
            selected.append(line)
            remaining -= len(line.encode("utf-8"))
        if remaining <= 0:
            break
    return "\n".join(reversed(selected))


JudgeVerdict = Literal["real_customer", "likely_spam"]
