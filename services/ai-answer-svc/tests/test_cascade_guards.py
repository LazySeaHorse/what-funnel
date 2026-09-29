"""Escalation guard, send-failure handling and DB-backed draft storage tests."""

import json
import os
import uuid
from unittest.mock import AsyncMock, MagicMock, patch

import asyncpg
import pytest
import pytest_asyncio

from db import ScopedDB
from main import execute_conversation_cascade, insert_pending_draft


class MockRecord(dict):
    def __getattr__(self, name):
        try:
            return self[name]
        except KeyError:
            raise AttributeError(name)


@pytest.fixture(autouse=True)
def disable_debounce(monkeypatch):
    monkeypatch.setattr("main.config.AI_DEBOUNCE_ENABLED", False)


def _cascade_db(mode: str, text: str, patterns=None):
    conversation = MockRecord({
        "assigned_user_ids": [], "state": "active", "state_reason": None,
        "reply_override": "inherit", "run_state": "idle", "generation_epoch": 0,
        "cooldown_level": 0, "unanswered_count": 0, "unanswered_window_started_at": None,
    })
    account = MockRecord({"settings": json.dumps({"ai_enabled": True, "ai_reply_mode_default": mode})})
    message = MockRecord({"content": json.dumps({"text": text})})

    async def fetchrow(query, *args):
        if "SELECT c.assigned_user_ids" in query:
            return conversation
        if "SELECT settings FROM accounts" in query:
            return account
        if "SET run_state = 'replying'" in query:
            return MockRecord({"generation_epoch": 1})
        if "SELECT content FROM messages" in query:
            return message
        return None

    async def fetch(query, *args):
        if "FROM patterns" in query:
            return patterns or []
        if "FROM messages" in query:
            return [MockRecord({"id": uuid.uuid4(), "content": message["content"], "created_at": None})]
        return []

    db = MagicMock()
    db.fetchrow = fetchrow
    db.fetch = fetch
    db.execute = AsyncMock()
    db.fetchval = AsyncMock(return_value=None)
    return db


def _executed(db, needle: str):
    return [c for c in db.execute.await_args_list if needle in c.args[0]]


@pytest.mark.asyncio
@pytest.mark.parametrize("mode", ["auto_send", "draft_only"])
async def test_escalation_blocks_pattern_embedding_and_rag_stages(mode):
    """A refund demand matching a FAQ pattern must never be auto-answered or embedded."""
    account_id, convo_id, msg_id = uuid.uuid4(), uuid.uuid4(), uuid.uuid4()
    patterns = [MockRecord({"trigger_phrases": ["shipping times"], "answer_text": "Shipping takes 3 days."})]
    redis_client = AsyncMock()

    with patch("main.ScopedDB") as MockScopedDB, \
         patch("main.get_ai_config", AsyncMock()) as get_cfg, \
         patch("main.insert_pending_draft", AsyncMock()) as insert_draft, \
         patch("main.send_ai_message", AsyncMock()) as send:
        db = _cascade_db(mode, "I demand a full refund, the shipping times are unacceptable", patterns)
        db.account_id = account_id
        MockScopedDB.return_value = db

        await execute_conversation_cascade(convo_id, account_id, msg_id, MagicMock(), redis_client)

    get_cfg.assert_not_awaited()  # no embedding / RAG stage ran
    send.assert_not_awaited()
    insert_draft.assert_not_awaited()
    review = _executed(db, "state = 'review_required'")
    assert len(review) == 1
    assert review[0].args[3] == "escalation"
    events = _executed(db, "INSERT INTO ai_answer_events")
    assert len(events) == 1
    assert events[0].args[4] == "none"
    assert events[0].args[6] == "flagged_human"


@pytest.mark.asyncio
async def test_send_failure_flags_human_and_records_event():
    account_id, convo_id, msg_id = uuid.uuid4(), uuid.uuid4(), uuid.uuid4()
    patterns = [MockRecord({"trigger_phrases": ["shipping times"], "answer_text": "Shipping takes 3 days."})]
    redis_client = AsyncMock()

    with patch("main.ScopedDB") as MockScopedDB, \
         patch("main.send_ai_message", AsyncMock(side_effect=RuntimeError("conversation-svc down"))):
        db = _cascade_db("auto_send", "what are your shipping times", patterns)
        db.account_id = account_id
        MockScopedDB.return_value = db

        await execute_conversation_cascade(convo_id, account_id, msg_id, MagicMock(), redis_client)

    review = _executed(db, "state = 'review_required'")
    assert len(review) == 1
    assert review[0].args[3] == "auto_send_failed"
    events = _executed(db, "INSERT INTO ai_answer_events")
    assert len(events) == 1
    assert events[0].args[4] == "pattern"
    assert events[0].args[6] == "flagged_human"
    assert events[0].args[7] is None
    control = [
        json.loads(c.args[1]["payload"]) for c in redis_client.xadd.call_args_list
        if c.args[0] == "ai.control.updated"
    ]
    assert control[-1]["state"] == "review_required"


@pytest.mark.asyncio
async def test_event_insert_failure_after_successful_send_is_not_reported_as_send_failure(caplog):
    account_id, convo_id, msg_id = uuid.uuid4(), uuid.uuid4(), uuid.uuid4()
    reply_id = uuid.uuid4()
    patterns = [MockRecord({"trigger_phrases": ["shipping times"], "answer_text": "Shipping takes 3 days."})]
    redis_client = AsyncMock()

    async def execute(query, *args):
        if "INSERT INTO ai_answer_events" in query:
            raise RuntimeError("db down")

    with patch("main.ScopedDB") as MockScopedDB, \
         patch("main.send_ai_message", AsyncMock(return_value={"id": str(reply_id)})):
        db = _cascade_db("auto_send", "what are your shipping times", patterns)
        db.execute = AsyncMock(side_effect=execute)
        db.account_id = account_id
        MockScopedDB.return_value = db

        await execute_conversation_cascade(convo_id, account_id, msg_id, MagicMock(), redis_client)

    assert not _executed(db, "state = 'review_required'")
    assert "Failed to record ai_answer_events" in caplog.text
    assert "Failed to auto-send" not in caplog.text
    ready = [
        json.loads(c.args[1]["payload"]) for c in redis_client.xadd.call_args_list
        if c.args[0] == "ai.reply_ready"
    ]
    assert ready and ready[0]["action"] == "auto_sent"


@pytest.mark.asyncio
async def test_unexpected_cascade_error_releases_run_lock_and_propagates():
    account_id, convo_id, msg_id = uuid.uuid4(), uuid.uuid4(), uuid.uuid4()
    patterns = [MockRecord({"trigger_phrases": ["shipping times"], "answer_text": "Shipping takes 3 days."})]

    with patch("main.ScopedDB") as MockScopedDB, \
         patch("main.send_ai_message", AsyncMock(return_value={"id": str(uuid.uuid4())})), \
         patch("main.publish_redis_stream", AsyncMock(side_effect=[None, RuntimeError("redis down")])):
        db = _cascade_db("auto_send", "what are your shipping times", patterns)
        db.account_id = account_id
        MockScopedDB.return_value = db

        with pytest.raises(RuntimeError, match="redis down"):
            await execute_conversation_cascade(convo_id, account_id, msg_id, MagicMock(), AsyncMock())

    assert _executed(db, "SET run_state = 'idle'")


def test_run_reclaim_window_exceeds_request_timeout():
    from main import config

    assert config.AI_RUN_RECLAIM_SECONDS > config.AI_REQUEST_TIMEOUT_SECONDS


# ---------------------------------------------------------------------------
# DB-backed: draft storage against the real partial unique index
# ---------------------------------------------------------------------------

DATABASE_URL = os.getenv(
    "DATABASE_URL", "postgres://whatfunnel:whatfunnel@localhost:5432/whatfunnel?sslmode=disable"
)


@pytest_asyncio.fixture
async def convo_env():
    try:
        pool = await asyncpg.create_pool(DATABASE_URL, min_size=1, max_size=3, timeout=5)
    except Exception:
        if os.getenv("CI"):
            pytest.fail("Postgres is not reachable")
        pytest.skip("Postgres is not reachable")
    account_id, channel_id, contact_id = uuid.uuid4(), uuid.uuid4(), uuid.uuid4()
    convo_id, msg_id = uuid.uuid4(), uuid.uuid4()
    try:
        await pool.execute(
            "INSERT INTO accounts (id, name, plan) VALUES ($1, 'Draft Test Account', 'self_hosted')", account_id
        )
        await pool.execute(
            "INSERT INTO channels (id, account_id, type, status) VALUES ($1, $2, 'webchat', 'connected')",
            channel_id, account_id,
        )
        await pool.execute(
            "INSERT INTO contacts (id, account_id, channel_id, external_identity) VALUES ($1, $2, $3, 'ext')",
            contact_id, account_id, channel_id,
        )
        await pool.execute(
            "INSERT INTO conversations (id, account_id, contact_id, channel_id, status) VALUES ($1, $2, $3, $4, 'open')",
            convo_id, account_id, contact_id, channel_id,
        )
        await pool.execute(
            """
            INSERT INTO messages (id, account_id, conversation_id, direction, sender_type, content_type, content)
            VALUES ($1, $2, $3, 'inbound', 'contact', 'text', '{"text": "hi"}')
            """,
            msg_id, account_id, convo_id,
        )
        yield ScopedDB(pool, account_id), pool, convo_id, msg_id
    finally:
        await pool.execute("DELETE FROM accounts WHERE id = $1", account_id)
        await pool.close()


async def _epoch(pool, convo_id) -> int:
    return await pool.fetchval(
        "SELECT generation_epoch FROM conversation_ai_state WHERE conversation_id = $1", convo_id
    )


@pytest.mark.asyncio
async def test_insert_pending_draft_supersedes_existing_pending_draft(convo_env):
    db, pool, convo_id, msg_id = convo_env
    epoch = await _epoch(pool, convo_id)

    first = await insert_pending_draft(db, convo_id, msg_id, "first", "pattern", 1.0, epoch)
    second = await insert_pending_draft(db, convo_id, msg_id, "second", "embedding", 0.9, epoch)

    assert first is not None and second is not None and first != second
    rows = await pool.fetch(
        "SELECT id, status, draft_text FROM ai_reply_drafts WHERE conversation_id = $1 ORDER BY created_at",
        convo_id,
    )
    assert [(r["id"], r["status"]) for r in rows] == [(first, "superseded"), (second, "pending")]
    events = await pool.fetchval(
        "SELECT COUNT(*) FROM ai_answer_events WHERE conversation_id = $1 AND action = 'drafted'", convo_id
    )
    assert events == 2


@pytest.mark.asyncio
async def test_insert_pending_draft_discards_stale_generation(convo_env):
    db, pool, convo_id, msg_id = convo_env
    epoch = await _epoch(pool, convo_id)

    assert await insert_pending_draft(db, convo_id, msg_id, "stale", "pattern", 1.0, epoch + 5) is None

    assert await pool.fetchval("SELECT COUNT(*) FROM ai_reply_drafts WHERE conversation_id = $1", convo_id) == 0
    assert await pool.fetchval("SELECT COUNT(*) FROM ai_answer_events WHERE conversation_id = $1", convo_id) == 0


@pytest.mark.asyncio
async def test_insert_pending_draft_discarded_when_conversation_not_active(convo_env):
    db, pool, convo_id, msg_id = convo_env
    epoch = await _epoch(pool, convo_id)
    await pool.execute(
        "UPDATE conversation_ai_state SET state = 'review_required' WHERE conversation_id = $1", convo_id
    )
    assert await insert_pending_draft(db, convo_id, msg_id, "x", "pattern", 1.0, epoch) is None
