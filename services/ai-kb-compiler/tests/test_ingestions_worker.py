import asyncio
import os
import uuid
from unittest.mock import AsyncMock, MagicMock, patch

import pytest
import pytest_asyncio

import ingestions
from db import create_db_pool

DATABASE_URL = os.getenv(
    "DATABASE_URL",
    "postgres://whatfunnel:whatfunnel@localhost:5432/whatfunnel?sslmode=disable",
)


# ---------------------------------------------------------------------------
# Worker loop resilience (no database needed)
# ---------------------------------------------------------------------------

@pytest.mark.asyncio
async def test_worker_survives_database_errors_and_backs_off():
    pool = MagicMock()
    calls = {"n": 0}

    async def fetchrow(*args, **kwargs):
        calls["n"] += 1
        if calls["n"] <= 2:
            raise ConnectionError("database restarting")
        return None

    pool.fetchrow = fetchrow
    pool.execute = AsyncMock()
    sleeps: list[float] = []

    async def fake_sleep(delay):
        sleeps.append(delay)
        if len(sleeps) >= 4:
            raise asyncio.CancelledError

    with patch("ingestions.asyncio.sleep", fake_sleep):
        with pytest.raises(asyncio.CancelledError):
            await ingestions.run_worker(pool, object)

    # Two failing iterations back off 1s then 2s, and the loop keeps polling afterwards.
    assert sleeps[:2] == [1.0, 2.0]
    assert calls["n"] >= 4


def test_log_worker_exit_reports_unexpected_failure(caplog):
    import logging
    from main import log_worker_exit

    async def boom():
        raise RuntimeError("worker crashed")

    async def run():
        task = asyncio.create_task(boom())
        await asyncio.gather(task, return_exceptions=True)
        return task

    task = asyncio.run(run())
    with caplog.at_level(logging.ERROR):
        log_worker_exit(task)
    assert "worker died unexpectedly" in caplog.text


# ---------------------------------------------------------------------------
# Exclusive publishing / orphan recovery (real database)
# ---------------------------------------------------------------------------

@pytest_asyncio.fixture
async def pool_and_account():
    try:
        pool = await create_db_pool(DATABASE_URL)
    except Exception:
        pytest.skip("Postgres is not reachable")
    account_id = uuid.uuid4()
    await pool.execute(
        "INSERT INTO accounts (id, name, plan) VALUES ($1, 'Ingestion Worker Test', 'self_hosted')", account_id
    )
    try:
        yield pool, account_id
    finally:
        await pool.execute("DELETE FROM audit_logs WHERE account_id = $1", account_id)
        await pool.execute("DELETE FROM kb_concepts WHERE account_id = $1", account_id)
        await pool.execute("DELETE FROM patterns WHERE account_id = $1", account_id)
        await pool.execute("DELETE FROM kb_ingestions WHERE account_id = $1", account_id)
        await pool.execute("DELETE FROM accounts WHERE id = $1", account_id)
        await pool.close()


async def _publishing_ingestion(pool, account_id) -> dict:
    ingestion_id = uuid.uuid4()
    await pool.execute(
        "INSERT INTO kb_ingestions (id, account_id, raw_text, status) VALUES ($1, $2, 'raw', 'publishing')",
        ingestion_id, account_id,
    )
    await pool.execute(
        """
        INSERT INTO kb_ingestion_items (ingestion_id, position, type, title, tags, body_text, status)
        VALUES ($1, 0, 'faq', 'Opening Hours', '{}', 'We open at 9', 'approved')
        """,
        ingestion_id,
    )
    await pool.execute(
        """
        INSERT INTO kb_ingestion_patterns (ingestion_id, position, canonical_question, answer_text, trigger_phrases, status)
        VALUES ($1, 0, 'When do you open?', 'At 9', ARRAY['open'], 'approved')
        """,
        ingestion_id,
    )
    return {"id": ingestion_id, "account_id": account_id, "requested_by": None}


@pytest.mark.asyncio
async def test_concurrent_publish_of_same_ingestion_publishes_once(pool_and_account):
    pool, account_id = pool_and_account
    job = await _publishing_ingestion(pool, account_id)

    client = MagicMock()
    client.embed = AsyncMock(return_value=[0.01] * 1536)
    cfg = MagicMock(embedding_model="embedding-test")
    with patch("ingestions.get_ai_config", AsyncMock(return_value=cfg)), \
         patch("ingestions.provider_client", return_value=client):
        await asyncio.gather(
            ingestions._publish(pool, dict(job)),
            ingestions._publish(pool, dict(job)),
            ingestions._publish(pool, dict(job)),
        )

    assert await pool.fetchval("SELECT COUNT(*) FROM kb_concepts WHERE account_id = $1", account_id) == 1
    assert await pool.fetchval("SELECT COUNT(*) FROM patterns WHERE account_id = $1", account_id) == 1
    assert await pool.fetchval("SELECT status FROM kb_ingestions WHERE id = $1", job["id"]) == "complete"


@pytest.mark.asyncio
async def test_requeue_orphaned_only_touches_stale_processing_rows(pool_and_account):
    pool, account_id = pool_and_account
    fresh, stale = uuid.uuid4(), uuid.uuid4()
    for ingestion_id in (fresh, stale):
        await pool.execute(
            "INSERT INTO kb_ingestions (id, account_id, raw_text, status) VALUES ($1, $2, 'raw', 'processing')",
            ingestion_id, account_id,
        )
    await pool.execute(
        "UPDATE kb_ingestions SET updated_at = NOW() - INTERVAL '1 day' WHERE id = $1", stale
    )

    await ingestions._requeue_orphaned(pool)

    assert await pool.fetchval("SELECT status FROM kb_ingestions WHERE id = $1", fresh) == "processing"
    assert await pool.fetchval("SELECT status FROM kb_ingestions WHERE id = $1", stale) == "queued"


@pytest.mark.asyncio
async def test_stale_extraction_does_not_clobber_reviewed_ingestion(pool_and_account):
    pool, account_id = pool_and_account
    ingestion_id = uuid.uuid4()
    await pool.execute(
        "INSERT INTO kb_ingestions (id, account_id, raw_text, status) VALUES ($1, $2, 'raw', 'review_required')",
        ingestion_id, account_id,
    )
    await pool.execute(
        """
        INSERT INTO kb_ingestion_items (ingestion_id, position, type, title, tags, body_text)
        VALUES ($1, 0, 'faq', 'Reviewed by owner', '{}', 'body')
        """,
        ingestion_id,
    )
    client = MagicMock()
    client.complete = AsyncMock(return_value={
        "concepts": [{"type": "faq", "title": "Late duplicate", "tags": [], "body_text": "x"}],
        "patterns": [],
    })
    cfg = MagicMock(analysis_model="a")
    job = {"id": ingestion_id, "account_id": account_id, "raw_text": "raw", "requested_by": None}
    with patch("ingestions.get_ai_config", AsyncMock(return_value=cfg)), \
         patch("ingestions.provider_client", return_value=client):
        await ingestions._extract(pool, job, object)

    titles = [r["title"] for r in await pool.fetch(
        "SELECT title FROM kb_ingestion_items WHERE ingestion_id = $1", ingestion_id
    )]
    assert titles == ["Reviewed by owner"]
