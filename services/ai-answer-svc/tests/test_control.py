from control import HUMAN_REVIEW_REPLY, next_cooldown_level, resolve_reply_mode, transcript_within_byte_budget


def test_reply_override_is_explicit_and_workspace_default_is_inherited():
    assert resolve_reply_mode("draft_only", "inherit") == "draft_only"
    assert resolve_reply_mode("auto_send", "inherit") == "auto_send"
    assert resolve_reply_mode("draft_only", "enabled") == "auto_send"
    assert resolve_reply_mode("auto_send", "disabled") == "disabled"


def test_progressive_cooldown_levels_are_bounded():
    assert next_cooldown_level(0, 1) == 1
    assert next_cooldown_level(1, 2) == 1
    assert next_cooldown_level(1, 3) == 2
    assert next_cooldown_level(2, 4) == 3
    assert next_cooldown_level(3, 5) == 4
    assert next_cooldown_level(4, 99) == 4


def test_judge_transcript_excludes_canned_replies_and_honors_byte_budget():
    transcript = transcript_within_byte_budget([
        ("customer", "hello"),
        ("assistant", HUMAN_REVIEW_REPLY),
        ("customer", "x" * 2000),
    ])

    assert HUMAN_REVIEW_REPLY not in transcript
    assert len(transcript.encode("utf-8")) <= 1000
    assert transcript.endswith("x" * 990)
