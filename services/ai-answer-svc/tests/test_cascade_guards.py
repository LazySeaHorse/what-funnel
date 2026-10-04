"""Send-failure handling, run-lock release and DB-backed draft storage tests."""

import json
import os
import uuid
from unittest.mock import AsyncMock, MagicMock, patch

import asyncpg
import pytest
import pytest_asyncio

from db import ScopedDB
from cascade_fakes import FakeClient, executed, faq_row, make_db, router_reply, run_cascade
from main import insert_pending_draft


FAQ_HOURS = lambda: faq_row("What are your shipping times?", "Shipping takes 3 days.")  # noqa: E731


def _canned_client():
    return FakeClient(router=router_reply("faq", "F1", True, "none"))


@pytest.mark.asyncio
async def test_send_failure_flags_human_and_records_event():
    db = make_db("auto_send", ("what are your shipping times",), faqs=[FAQ_HOURS()])
    run = await run_cascade(db, _canned_client(), extra_patches=[
        patch("main.send_ai_message", AsyncMock(side_effect=RuntimeError("conversation-svc down")))
    ])

    review = executed(db, "state = 'review_required'")
    assert len(review) == 1
    assert review[0].args[3] == "auto_send_failed"
    events = run.events()
    assert len(events) == 1
    assert events[0].args[4] == "canned"
    assert events[0].args[6] == "flagged_human"
    assert events[0].args[7] is None
    assert run.control_states()[-1] == "review_required"


@pytest.mark.asyncio
async def test_event_insert_failure_after_successful_send_is_not_reported_as_send_failure(caplog):
    reply_id = uuid.uuid4()
    db = make_db("auto_send", ("what are your shipping times",), faqs=[FAQ_HOURS()])

    async def execute(query, *args):
        if "INSERT INTO ai_answer_events" in query:
            raise RuntimeError("db down")

    db.execute = AsyncMock(side_effect=execute)
    run = await run_cascade(db, _canned_client(), send_result={"id": str(reply_id)})

    assert not executed(db, "state = 'review_required'")
    assert "Failed to record ai_answer_events" in caplog.text
    assert "Failed to auto-send" not in caplog.text
    ready = [
        json.loads(c.args[1]["payload"]) for c in run.redis.xadd.call_args_list
        if c.args[0] == "ai.reply_ready"
    ]
    assert ready and ready[0]["action"] == "auto_sent"


@pytest.mark.asyncio
async def test_unexpected_cascade_error_releases_run_lock_and_propagates():
    db = make_db("auto_send", ("what are your shipping times",), faqs=[FAQ_HOURS()])

    with pytest.raises(RuntimeError, match="redis down"):
        await run_cascade(db, _canned_client(), extra_patches=[
            patch("main.publish_redis_stream", AsyncMock(side_effect=[None, RuntimeError("redis down")]))
        ])

    assert executed(db, "SET run_state = 'idle'")


def test_run_reclaim_window_exceeds_cascade_deadline():
    from main import config

    assert config.AI_RUN_RECLAIM_SECONDS > config.AI_CASCADE_DEADLINE_SECONDS
    assert config.AI_CASCADE_DEADLINE_SECONDS <= 60
    assert config.AI_REQUEST_TIMEOUT_SECONDS <= 30
    assert config.AI_PROVIDER_MAX_ATTEMPTS == 2


@pytest.mark.asyncio
async def test_cascade_deadline_fails_closed_without_customer_message(monkeypatch):
    import asyncio

    monkeypatch.setattr("main.config.AI_CASCADE_DEADLINE_SECONDS", 0.05)

    async def slow_decide(*args, **kwargs):
        await asyncio.sleep(5)

    db = make_db("draft_only", ("where is my parcel",), faqs=[FAQ_HOURS()])
    run = await run_cascade(db, _canned_client(), extra_patches=[patch("main.decide_reply", slow_decide)])
    run.send.assert_not_awaited()
    run.insert_draft.assert_not_awaited()
    assert executed(db, "state = 'review_required'")[0].args[3] == "unanswerable"
    assert run.events()[0].args[4] == "handoff"


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

    first = await insert_pending_draft(db, convo_id, msg_id, "first", "canned", None, epoch)
    second = await insert_pending_draft(db, convo_id, msg_id, "second", "rag", None, epoch)

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

    assert await insert_pending_draft(db, convo_id, msg_id, "stale", "canned", None, epoch + 5) is None

    assert await pool.fetchval("SELECT COUNT(*) FROM ai_reply_drafts WHERE conversation_id = $1", convo_id) == 0
    assert await pool.fetchval("SELECT COUNT(*) FROM ai_answer_events WHERE conversation_id = $1", convo_id) == 0


@pytest.mark.asyncio
async def test_insert_pending_draft_discarded_when_conversation_not_active(convo_env):
    db, pool, convo_id, msg_id = convo_env
    epoch = await _epoch(pool, convo_id)
    await pool.execute(
        "UPDATE conversation_ai_state SET state = 'review_required' WHERE conversation_id = $1", convo_id
    )
    assert await insert_pending_draft(db, convo_id, msg_id, "x", "canned", None, epoch) is None
