"""DB-backed: unapproved FAQs never reach the router menu."""

import os
import uuid

import asyncpg
import pytest
import pytest_asyncio

from main import APPROVED_FAQ_QUERY
from router import build_menu

DATABASE_URL = os.getenv(
    "DATABASE_URL", "postgres://whatfunnel:whatfunnel@localhost:5432/whatfunnel?sslmode=disable"
)


@pytest_asyncio.fixture
async def account():
    try:
        pool = await asyncpg.create_pool(DATABASE_URL, min_size=1, max_size=2, timeout=5)
    except Exception:
        if os.getenv("CI"):
            pytest.fail("Postgres is not reachable")
        pytest.skip("Postgres is not reachable")
    account_id = uuid.uuid4()
    await pool.execute("INSERT INTO accounts (id, name, plan) VALUES ($1, 'FAQ menu test', 'self_hosted')", account_id)
    try:
        yield pool, account_id
    finally:
        await pool.execute("DELETE FROM patterns WHERE account_id = $1", account_id)
        await pool.execute("DELETE FROM accounts WHERE id = $1", account_id)
        await pool.close()


@pytest.mark.asyncio
async def test_menu_query_excludes_unapproved_faqs(account):
    pool, account_id = account
    await pool.execute(
        "INSERT INTO patterns (account_id, canonical_question, answer_text, trigger_phrases, approved_at) "
        "VALUES ($1, 'Approved?', 'yes', ARRAY['a'], NOW())", account_id,
    )
    await pool.execute(
        "INSERT INTO patterns (account_id, canonical_question, answer_text, trigger_phrases) "
        "VALUES ($1, 'Pending?', 'must never be sent', ARRAY['b'])", account_id,
    )
    rows = await pool.fetch(APPROVED_FAQ_QUERY, account_id)
    menu = build_menu(list(rows))
    assert [f.question for f in menu] == ["Approved?"]
