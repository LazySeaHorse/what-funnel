import json
import os
import pytest
import pytest_asyncio
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


@pytest_asyncio.fixture
async def redis_client(monkeypatch):
    """Real Redis with private key names so shared instances are never touched."""
    from redis.asyncio import Redis

    suffix = uuid.uuid4().hex[:8]
    queue = f"test:ai_debounce:{suffix}:queue"
    meta = f"test:ai_debounce:{suffix}:meta:"
    seen = f"test:ai_debounce:{suffix}:seen:"
    monkeypatch.setattr("debounce.DEBOUNCE_QUEUE_KEY", queue)
    monkeypatch.setattr("debounce.DEBOUNCE_META_PREFIX", meta)
    monkeypatch.setattr("debounce.DEBOUNCE_SEEN_PREFIX", seen)
    url = os.getenv("REDIS_URL", "redis://localhost:6379")
    if "://" not in url:
        url = f"redis://{url}"
    client = Redis.from_url(url)
    try:
        await client.ping()
    except Exception:
        await client.aclose()
        if os.getenv("CI"):
            pytest.fail("Redis is not reachable")
        pytest.skip("Redis is not reachable")
    try:
        yield client
    finally:
        keys = await client.keys(f"test:ai_debounce:{suffix}:*")
        if keys:
            await client.delete(*keys)
        await client.aclose()


@pytest.mark.asyncio
async def test_record_inbound_message_first_bubble(redis_client):
    import debounce

    convo_id, msg_id, account_id = uuid.uuid4(), uuid.uuid4(), uuid.uuid4()
    before = time.time()
    scheduled, delay = await record_inbound_message(redis_client, account_id, convo_id, msg_id, is_text=True)

    assert scheduled is True
    assert delay == 10.0
    meta = await redis_client.hgetall(f"{debounce.DEBOUNCE_META_PREFIX}{convo_id}")
    assert meta[b"account_id"] == str(account_id).encode()
    assert meta[b"latest_message_id"] == str(msg_id).encode()
    assert meta[b"bubble_count"] == b"1"
    assert b"has_non_text" not in meta
    deadline = await redis_client.zscore(debounce.DEBOUNCE_QUEUE_KEY, str(convo_id))
    assert before + 10.0 <= deadline <= time.time() + 10.0


@pytest.mark.asyncio
async def test_record_inbound_message_bubble_delays(redis_client):
    account_id, convo_id = uuid.uuid4(), uuid.uuid4()
    delays = []
    for _ in range(4):
        scheduled, delay = await record_inbound_message(redis_client, account_id, convo_id, uuid.uuid4())
        assert scheduled is True
        delays.append(delay)
    assert delays == [10.0, 5.0, 10.0, 10.0]


@pytest.mark.asyncio
async def test_record_inbound_message_duplicate_webhook_ignored(redis_client):
    import debounce

    account_id, convo_id, msg_id = uuid.uuid4(), uuid.uuid4(), uuid.uuid4()
    assert await record_inbound_message(redis_client, account_id, convo_id, msg_id) == (True, 10.0)
    assert await record_inbound_message(redis_client, account_id, convo_id, msg_id) == (False, 0.0)
    meta = await redis_client.hgetall(f"{debounce.DEBOUNCE_META_PREFIX}{convo_id}")
    assert meta[b"bubble_count"] == b"1"


@pytest.mark.asyncio
async def test_record_inbound_message_non_text(redis_client):
    import debounce

    convo_id = uuid.uuid4()
    await record_inbound_message(redis_client, uuid.uuid4(), convo_id, uuid.uuid4(), is_text=False)
    meta = await redis_client.hgetall(f"{debounce.DEBOUNCE_META_PREFIX}{convo_id}")
    assert meta[b"has_non_text"] == b"1"


@pytest.mark.asyncio
async def test_cancel_debounce(redis_client):
    import debounce

    convo_id = uuid.uuid4()
    await record_inbound_message(redis_client, uuid.uuid4(), convo_id, uuid.uuid4())
    await cancel_debounce(redis_client, convo_id)
    assert await redis_client.zscore(debounce.DEBOUNCE_QUEUE_KEY, str(convo_id)) is None
    assert not await redis_client.exists(f"{debounce.DEBOUNCE_META_PREFIX}{convo_id}")


@pytest.mark.asyncio
async def test_pop_due_conversations_returns_meta_atomically(redis_client, monkeypatch):
    import debounce

    account_id, convo_id, msg_id = uuid.uuid4(), uuid.uuid4(), uuid.uuid4()
    await record_inbound_message(redis_client, account_id, convo_id, uuid.uuid4())
    await record_inbound_message(redis_client, account_id, convo_id, msg_id, is_text=False)

    # Not due yet.
    assert await pop_due_conversations(redis_client) == []

    real_time = time.time
    monkeypatch.setattr("debounce.time.time", lambda: real_time() + 60)
    due = await pop_due_conversations(redis_client)
    assert len(due) == 1
    assert due[0]["conversation_id"] == convo_id
    assert due[0]["account_id"] == account_id
    assert due[0]["latest_message_id"] == msg_id
    assert due[0]["has_non_text"] is True
    assert due[0]["bubble_count"] == 2
    assert due[0]["attempts"] == 0

    # Popped exactly once; metadata is gone with it.
    assert await pop_due_conversations(redis_client) == []
    assert not await redis_client.exists(f"{debounce.DEBOUNCE_META_PREFIX}{convo_id}")


@pytest.mark.asyncio
async def test_message_recorded_after_pop_starts_a_fresh_cycle(redis_client, monkeypatch):
    account_id, convo_id = uuid.uuid4(), uuid.uuid4()
    await record_inbound_message(redis_client, account_id, convo_id, uuid.uuid4())
    real_time = time.time
    monkeypatch.setattr("debounce.time.time", lambda: real_time() + 60)
    assert len(await pop_due_conversations(redis_client)) == 1
    monkeypatch.undo()

    newer = uuid.uuid4()
    assert await record_inbound_message(redis_client, account_id, convo_id, newer) == (True, 10.0)
    monkeypatch.setattr("debounce.time.time", lambda: real_time() + 60)
    due = await pop_due_conversations(redis_client)
    assert [d["latest_message_id"] for d in due] == [newer]
    assert due[0]["bubble_count"] == 1


@pytest.mark.asyncio
async def test_requeue_in_flight_keeps_newer_message_and_later_deadline(redis_client):
    import debounce

    account_id, convo_id = uuid.uuid4(), uuid.uuid4()
    newer, older = uuid.uuid4(), uuid.uuid4()
    await record_inbound_message(redis_client, account_id, convo_id, newer)
    deadline_before = await redis_client.zscore(debounce.DEBOUNCE_QUEUE_KEY, str(convo_id))

    await requeue_in_flight(redis_client, convo_id, account_id, older, delay=2.5, attempts=1)

    meta = await redis_client.hgetall(f"{debounce.DEBOUNCE_META_PREFIX}{convo_id}")
    assert meta[b"latest_message_id"] == str(newer).encode()
    assert meta[b"attempts"] == b"1"
    assert await redis_client.zscore(debounce.DEBOUNCE_QUEUE_KEY, str(convo_id)) == deadline_before


@pytest.mark.asyncio
async def test_requeue_in_flight_restores_popped_conversation(redis_client, monkeypatch):
    import debounce

    account_id, convo_id, msg_id = uuid.uuid4(), uuid.uuid4(), uuid.uuid4()
    await requeue_in_flight(redis_client, convo_id, account_id, msg_id, has_non_text=True, delay=2.5)
    assert await redis_client.zscore(debounce.DEBOUNCE_QUEUE_KEY, str(convo_id)) is not None
    real_time = time.time
    monkeypatch.setattr("debounce.time.time", lambda: real_time() + 60)
    due = await pop_due_conversations(redis_client)
    assert due[0]["latest_message_id"] == msg_id
    assert due[0]["has_non_text"] is True


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
        assert params[3] == "none"  # stage_matched
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

    draft_id = uuid.uuid4()
    with patch("main.ScopedDB") as MockScopedDB, \
         patch("main.insert_pending_draft", AsyncMock(return_value=draft_id)) as insert_draft:
        db_instance = MockScopedDB.return_value
        db_instance.fetchrow = mock_fetchrow
        db_instance.fetch = mock_fetch
        db_instance.execute = AsyncMock()
        db_instance.account_id = account_id

        await execute_conversation_cascade(
            convo_id, account_id, msg_id, db_pool, redis_client, has_non_text=False
        )

        # Verify pattern matched even with multi-bubble greeting!
        insert_draft.assert_awaited_once()
        args = insert_draft.call_args[0]
        assert args[3] == "Yes, we offer house calls."
        assert args[4] == "pattern"
        assert args[5] == 1.0


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

