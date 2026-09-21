import json
import pytest
import uuid
import time
from unittest.mock import AsyncMock, MagicMock, patch

from debounce import (
    calculate_debounce_delay,
    record_inbound_message,
    cancel_debounce,
    pop_due_conversations,
    requeue_in_flight,
    DEBOUNCE_QUEUE_KEY,
    DEBOUNCE_META_PREFIX,
    DEBOUNCE_SEEN_PREFIX,
)
from config import config


def test_calculate_debounce_delay():
    assert calculate_debounce_delay(1) == 10.0
    assert calculate_debounce_delay(2) == 5.0
    assert calculate_debounce_delay(3) == 10.0
    assert calculate_debounce_delay(4) == 10.0


@pytest.mark.asyncio
async def test_record_inbound_message_first_bubble():
    redis_mock = AsyncMock()
    # First message: sadd returns 1 (new message)
    redis_mock.sadd.return_value = 1
    redis_mock.hincrby.return_value = 1

    account_id = uuid.uuid4()
    convo_id = uuid.uuid4()
    msg_id = uuid.uuid4()

    scheduled, delay = await record_inbound_message(
        redis_mock, account_id, convo_id, msg_id, is_text=True
    )

    assert scheduled is True
    assert delay == 10.0

    redis_mock.sadd.assert_awaited_once_with(f"{DEBOUNCE_SEEN_PREFIX}{convo_id}", str(msg_id))
    redis_mock.hincrby.assert_awaited_once_with(f"{DEBOUNCE_META_PREFIX}{convo_id}", "bubble_count", 1)
    redis_mock.hset.assert_awaited_once_with(
        f"{DEBOUNCE_META_PREFIX}{convo_id}",
        mapping={"account_id": str(account_id), "latest_message_id": str(msg_id)},
    )
    redis_mock.zadd.assert_awaited_once()
    zadd_call = redis_mock.zadd.call_args
    assert zadd_call.args[0] == DEBOUNCE_QUEUE_KEY
    assert str(convo_id) in zadd_call.args[1]


@pytest.mark.asyncio
async def test_record_inbound_message_subsequent_bubble():
    redis_mock = AsyncMock()
    redis_mock.sadd.return_value = 1
    # 2nd bubble
    redis_mock.hincrby.return_value = 2

    account_id = uuid.uuid4()
    convo_id = uuid.uuid4()
    msg_id = uuid.uuid4()

    scheduled, delay = await record_inbound_message(
        redis_mock, account_id, convo_id, msg_id, is_text=True
    )

    assert scheduled is True
    assert delay == 5.0


@pytest.mark.asyncio
async def test_record_inbound_message_burst_bubble():
    redis_mock = AsyncMock()
    redis_mock.sadd.return_value = 1
    # 3rd bubble jumps back up to 10s
    redis_mock.hincrby.return_value = 3

    account_id = uuid.uuid4()
    convo_id = uuid.uuid4()
    msg_id = uuid.uuid4()

    scheduled, delay = await record_inbound_message(
        redis_mock, account_id, convo_id, msg_id, is_text=True
    )

    assert scheduled is True
    assert delay == 10.0


@pytest.mark.asyncio
async def test_record_inbound_message_duplicate_webhook_ignored():
    redis_mock = AsyncMock()
    # Duplicate delivery: sadd returns 0
    redis_mock.sadd.return_value = 0

    account_id = uuid.uuid4()
    convo_id = uuid.uuid4()
    msg_id = uuid.uuid4()

    scheduled, delay = await record_inbound_message(
        redis_mock, account_id, convo_id, msg_id, is_text=True
    )

    assert scheduled is False
    assert delay == 0.0
    redis_mock.hincrby.assert_not_awaited()
    redis_mock.zadd.assert_not_awaited()


@pytest.mark.asyncio
async def test_record_inbound_message_non_text():
    redis_mock = AsyncMock()
    redis_mock.sadd.return_value = 1
    redis_mock.hincrby.return_value = 1

    account_id = uuid.uuid4()
    convo_id = uuid.uuid4()
    msg_id = uuid.uuid4()

    scheduled, delay = await record_inbound_message(
        redis_mock, account_id, convo_id, msg_id, is_text=False
    )

    assert scheduled is True
    redis_mock.hset.assert_awaited_once_with(
        f"{DEBOUNCE_META_PREFIX}{convo_id}",
        mapping={
            "account_id": str(account_id),
            "latest_message_id": str(msg_id),
            "has_non_text": "1",
        },
    )


@pytest.mark.asyncio
async def test_cancel_debounce():
    redis_mock = AsyncMock()
    convo_id = uuid.uuid4()

    await cancel_debounce(redis_mock, convo_id)

    redis_mock.zrem.assert_awaited_once_with(DEBOUNCE_QUEUE_KEY, str(convo_id))
    redis_mock.delete.assert_awaited_once_with(f"{DEBOUNCE_META_PREFIX}{convo_id}")


@pytest.mark.asyncio
async def test_pop_due_conversations():
    redis_mock = AsyncMock()
    convo_id = uuid.uuid4()
    account_id = uuid.uuid4()
    msg_id = uuid.uuid4()

    redis_mock.eval.return_value = [str(convo_id).encode("utf-8")]
    redis_mock.hgetall.return_value = {
        b"account_id": str(account_id).encode("utf-8"),
        b"latest_message_id": str(msg_id).encode("utf-8"),
        b"bubble_count": b"2",
        b"has_non_text": b"1",
    }

    due = await pop_due_conversations(redis_mock)

    assert len(due) == 1
    assert due[0]["conversation_id"] == convo_id
    assert due[0]["account_id"] == account_id
    assert due[0]["latest_message_id"] == msg_id
    assert due[0]["has_non_text"] is True
    assert due[0]["bubble_count"] == 2


@pytest.mark.asyncio
async def test_requeue_in_flight():
    redis_mock = AsyncMock()
    convo_id = uuid.uuid4()
    account_id = uuid.uuid4()
    msg_id = uuid.uuid4()

    await requeue_in_flight(redis_mock, convo_id, account_id, msg_id, has_non_text=False, delay=2.5)

    redis_mock.hset.assert_awaited_once()
    redis_mock.zadd.assert_awaited_once()
    zadd_call = redis_mock.zadd.call_args
    assert str(convo_id) in zadd_call.args[1]


# A mock Record class to simulate asyncpg row returns
class MockRecord(dict):
    def __getattr__(self, name):
        try:
            return self[name]
        except KeyError:
            raise AttributeError(name)


@pytest.mark.asyncio
async def test_process_conversation_updated_outbound_cancels_debounce():
    from main import process_conversation_updated

    db_pool = MagicMock()
    redis_client = AsyncMock()
    account_id = uuid.uuid4()
    convo_id = uuid.uuid4()
    msg_id = uuid.uuid4()

    mock_msg = MockRecord({
        "direction": "outbound",
        "content_type": "text",
        "content": json.dumps({"text": "Human replying"}),
    })

    with patch("main.ScopedDB") as MockScopedDB:
        db_instance = MockScopedDB.return_value
        db_instance.fetchrow = AsyncMock(return_value=mock_msg)
        db_instance.account_id = account_id

        await process_conversation_updated({
            "account_id": str(account_id),
            "conversation_id": str(convo_id),
            "message_id": str(msg_id),
        }, db_pool, redis_client)

        # Cancel debounce must be called
        redis_client.zrem.assert_awaited_once_with(DEBOUNCE_QUEUE_KEY, str(convo_id))
        redis_client.delete.assert_awaited_once_with(f"{DEBOUNCE_META_PREFIX}{convo_id}")


@pytest.mark.asyncio
async def test_execute_conversation_cascade_non_text_routes_to_human_review():
    from main import execute_conversation_cascade
    from control import NON_TEXT_HUMAN_REVIEW_REPLY

    db_pool = MagicMock()
    redis_client = AsyncMock()
    account_id = uuid.uuid4()
    convo_id = uuid.uuid4()
    msg_id = uuid.uuid4()

    mock_convo = MockRecord({
        "assigned_user_ids": [], "state": "active", "state_reason": None,
        "reply_override": "inherit", "run_state": "idle", "generation_epoch": 0,
        "cooldown_level": 0, "unanswered_count": 0,
        "unanswered_window_started_at": None,
    })
    mock_account = MockRecord({
        "settings": json.dumps({
            "ai_enabled": True,
            "ai_reply_mode_default": "auto_send",
        })
    })

    async def mock_fetchrow(query, *args):
        if "SELECT c.assigned_user_ids" in query:
            return mock_convo
        if "SELECT settings FROM accounts" in query:
            return mock_account
        if "SET run_state = 'replying'" in query:
            return MockRecord({"generation_epoch": 1})
        if "SET state = 'cooldown'" in query:
            return MockRecord({"generation_epoch": 2})
        return None

    with patch("main.ScopedDB") as MockScopedDB, \
         patch("main.send_ai_message", AsyncMock(return_value={"id": str(uuid.uuid4())})) as mock_send:
        db_instance = MockScopedDB.return_value
        db_instance.fetchrow = mock_fetchrow
        db_instance.execute = AsyncMock()
        db_instance.account_id = account_id

        await execute_conversation_cascade(
            convo_id, account_id, msg_id, db_pool, redis_client, has_non_text=True
        )

        # Verify pre-written non-text acknowledgement was sent
        mock_send.assert_awaited_once()
        assert mock_send.call_args[0][2] == NON_TEXT_HUMAN_REVIEW_REPLY

        # Verify ai_answer_events was recorded with non_text stage and flagged_human
        event_calls = [c for c in db_instance.execute.call_args_list if "INSERT INTO ai_answer_events" in c.args[0]]
        assert len(event_calls) == 1
        params = event_calls[0].args[1:]
        assert params[3] == "non_text"  # stage_matched
        assert params[5] == "flagged_human"  # action


@pytest.mark.asyncio
async def test_execute_conversation_cascade_combines_bubbles_and_matches_pattern():
    from main import execute_conversation_cascade

    db_pool = MagicMock()
    redis_client = AsyncMock()
    account_id = uuid.uuid4()
    convo_id = uuid.uuid4()
    msg_id = uuid.uuid4()

    mock_convo = MockRecord({
        "assigned_user_ids": [], "state": "active", "state_reason": None,
        "reply_override": "inherit", "run_state": "idle", "generation_epoch": 0,
        "cooldown_level": 0, "unanswered_count": 0,
        "unanswered_window_started_at": None,
    })
    mock_account = MockRecord({
        "settings": json.dumps({
            "ai_enabled": True,
            "ai_reply_mode_default": "draft_only",
        })
    })
    # User sends 2 bubbles: "Hello there" followed by "Do you offer house calls?"
    bubble_1 = MockRecord({"id": uuid.uuid4(), "content": json.dumps({"text": "Hello there"}), "created_at": "2026-09-21T10:00:00Z"})
    bubble_2 = MockRecord({"id": msg_id, "content": json.dumps({"text": "Do you offer house calls?"}), "created_at": "2026-09-21T10:00:05Z"})

    mock_pattern = MockRecord({
        "trigger_phrases": ["Do you offer house calls?"],
        "answer_text": "Yes, we offer house calls."
    })

    async def mock_fetchrow(query, *args):
        if "SELECT c.assigned_user_ids" in query:
            return mock_convo
        if "SELECT settings FROM accounts" in query:
            return mock_account
        if "SET run_state = 'replying'" in query:
            return MockRecord({"generation_epoch": 1})
        return None

    async def mock_fetch(query, *args):
        if "WHERE conversation_id = $1 AND account_id = $2" in query and "ORDER BY created_at ASC" in query:
            return [bubble_1, bubble_2]
        if "patterns" in query:
            return [mock_pattern]
        return []

    with patch("main.ScopedDB") as MockScopedDB:
        db_instance = MockScopedDB.return_value
        db_instance.fetchrow = mock_fetchrow
        db_instance.fetch = mock_fetch
        draft_id = uuid.uuid4()
        db_instance.fetchval = AsyncMock(return_value=draft_id)
        db_instance.execute = AsyncMock()
        db_instance.account_id = account_id

        await execute_conversation_cascade(
            convo_id, account_id, msg_id, db_pool, redis_client, has_non_text=False
        )

        # Verify pattern matched even with multi-bubble greeting!
        db_instance.fetchval.assert_awaited_once()
        args = db_instance.fetchval.call_args[0]
        params = args[1:]
        assert params[3] == "Yes, we offer house calls."
        assert params[4] == "pattern"
        assert params[5] == 1.0


@pytest.mark.asyncio
async def test_execute_conversation_cascade_in_flight_requeues():
    from main import execute_conversation_cascade

    db_pool = MagicMock()
    redis_client = AsyncMock()
    account_id = uuid.uuid4()
    convo_id = uuid.uuid4()
    msg_id = uuid.uuid4()

    mock_convo = MockRecord({
        "assigned_user_ids": [], "state": "active", "state_reason": None,
        "reply_override": "inherit", "run_state": "replying", "generation_epoch": 1,
        "cooldown_level": 0, "unanswered_count": 0,
        "unanswered_window_started_at": None,
    })
    mock_account = MockRecord({
        "settings": json.dumps({"ai_enabled": True, "ai_reply_mode_default": "auto_send"})
    })

    async def mock_fetchrow(query, *args):
        if "SELECT c.assigned_user_ids" in query:
            return mock_convo
        if "SELECT settings FROM accounts" in query:
            return mock_account
        if "SET run_state = 'replying'" in query:
            # Active generation lock fails!
            return None
        return None

    with patch("main.ScopedDB") as MockScopedDB:
        db_instance = MockScopedDB.return_value
        db_instance.fetchrow = mock_fetchrow
        db_instance.account_id = account_id

        await execute_conversation_cascade(
            convo_id, account_id, msg_id, db_pool, redis_client, has_non_text=False
        )

        # Must re-queue into Redis ZSET for retry rather than dropping!
        redis_client.zadd.assert_awaited_once()
        assert str(convo_id) in redis_client.zadd.call_args.args[1]

