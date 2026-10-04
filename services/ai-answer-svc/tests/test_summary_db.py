"""DB + Redis backed: summary generation against real Postgres and Redis (provider faked)."""

import json
import os
import uuid
from types import SimpleNamespace

import asyncpg
import pytest
import pytest_asyncio
from redis.asyncio import Redis

import summary
from summary import generate_summary

DATABASE_URL = os.getenv(
    "DATABASE_URL", "postgres://whatfunnel:whatfunnel@localhost:5432/whatfunnel?sslmode=disable"
)
REDIS_URL = os.getenv("TEST_REDIS_URL") or f"redis://{os.getenv('REDIS_URL', 'localhost:6379')}"


@pytest_asyncio.fixture
async def world(monkeypatch):
    try:
        pool = await asyncpg.create_pool(DATABASE_URL, min_size=1, max_size=3, timeout=5)
        redis = Redis.from_url(REDIS_URL)
        await redis.ping()
    except Exception:
        if os.getenv("CI"):
            pytest.fail("Postgres/Redis is not reachable")
        pytest.skip("Postgres/Redis is not reachable")
    account, other_account = uuid.uuid4(), uuid.uuid4()
    for a in (account, other_account):
        await pool.execute("INSERT INTO accounts (id, name, plan, settings) VALUES ($1, 'summary test', 'self_hosted', $2)", a, json.dumps(
            {"summary_schema": {"budget": "Budget mentioned", "timeline": "When"}} if a == account else {}
        ))
    channel = await pool.fetchval("INSERT INTO channels (account_id, type, status) VALUES ($1, 'whatsapp', 'connected') RETURNING id", account)
    contact = await pool.fetchval("INSERT INTO contacts (account_id, channel_id, external_identity, display_name) VALUES ($1, $2, 'c1', 'C') RETURNING id", account, channel)
    convo = await pool.fetchval(
        "INSERT INTO conversations (account_id, contact_id, channel_id) VALUES ($1, $2, $3) RETURNING id", account, contact, channel
    )

    async def add(text, direction="inbound", sender="contact", ctype="text"):
        await pool.execute(
            "INSERT INTO messages (account_id, conversation_id, direction, sender_type, content_type, content) "
            "VALUES ($1, $2, $3, $4, $5, $6)", account, convo, direction, sender, ctype, json.dumps({"text": text}),
        )

    calls = []

    class Client:
        async def complete(self, model, messages, schema):
            calls.append(messages[0]["content"])
            return {"budget": "**5k** budget", "timeline": "next week"}

    async def ai_config(db):
        return SimpleNamespace(analysis_model="m")

    monkeypatch.setattr(summary, "provider_client", lambda cfg: Client())
    monkeypatch.setattr(summary, "get_ai_config", ai_config)
    await redis.delete(f"summary:cooldown:{convo}", f"summary:lock:{convo}", "conversation.summary_updated", "conversation.summary_failed")
    try:
        yield SimpleNamespace(pool=pool, redis=redis, account=account, other=other_account, convo=convo, add=add, calls=calls)
    finally:
        await redis.delete(f"summary:cooldown:{convo}", f"summary:lock:{convo}", "conversation.summary_updated", "conversation.summary_failed")
        await pool.execute("DELETE FROM accounts WHERE id = ANY($1)", [account, other_account])
        await redis.aclose()
        await pool.close()


@pytest.mark.asyncio
async def test_generate_persist_cache_and_regenerate(world):
    w = world
    await w.add("My budget is 5k, need it next week")
    await w.add("Got it", "outbound", "human")
    await w.add("", ctype="image")

    out = await generate_summary(w.pool, w.redis, w.account, w.convo, "requested")
    assert out.status == "generated"
    row = await w.pool.fetchrow("SELECT * FROM conversation_summaries WHERE conversation_id = $1", w.convo)
    assert json.loads(row["summary_fields"]) == {"budget": "5k budget", "timeline": "next week"}
    assert row["message_count_at_generation"] == 3 and row["account_id"] == w.account
    events = await w.redis.xrange("conversation.summary_updated")
    assert len(events) == 1
    payload = json.loads(events[0][1][b"payload"])
    assert payload["summary_fields"]["budget"] == "5k budget" and "generated_at" in payload
    assert "Customer: My budget is 5k" in w.calls[0] and "Agent: Got it" in w.calls[0]

    # Same state, after the cooldown: cached, no LLM call
    await w.redis.delete(f"summary:cooldown:{w.convo}")
    assert (await generate_summary(w.pool, w.redis, w.account, w.convo, "requested")).status == "cached"
    assert len(w.calls) == 1

    # New message: regenerates on request even inside the 60 s window
    await w.add("one more thing")
    await w.redis.delete(f"summary:cooldown:{w.convo}")
    assert (await generate_summary(w.pool, w.redis, w.account, w.convo, "requested")).status == "generated"
    assert await w.pool.fetchval("SELECT message_count_at_generation FROM conversation_summaries WHERE conversation_id = $1", w.convo) == 4
    assert await w.pool.fetchval("SELECT COUNT(*) FROM conversation_summaries WHERE conversation_id = $1", w.convo) == 1

    # Close-triggered run honours the 60 s debounce
    await w.add("and another")
    assert (await generate_summary(w.pool, w.redis, w.account, w.convo, "closed")).status == "skipped"


@pytest.mark.asyncio
async def test_foreign_account_cannot_summarize_conversation(world):
    w = world
    await w.add("secret text")
    out = await generate_summary(w.pool, w.redis, w.other, w.convo, "requested")
    assert out.status == "failed" and out.error_code == "no_messages"
    assert w.calls == []
    assert await w.pool.fetchval("SELECT COUNT(*) FROM conversation_summaries WHERE conversation_id = $1", w.convo) == 0
