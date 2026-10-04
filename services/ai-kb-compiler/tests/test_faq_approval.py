"""FAQs are used by the AI cascade only after a human approved them."""

import asyncio
import json
import os
import uuid
from unittest.mock import AsyncMock, MagicMock, patch

import pytest
import pytest_asyncio

import ingestions
import kb_service
from db import ScopedDB, create_db_pool
from mining import example_phrases
from phrases import normalize_not_for
from schemas import OKFPatternDraft

DATABASE_URL = os.getenv(
    "DATABASE_URL",
    "postgres://whatfunnel:whatfunnel@localhost:5432/whatfunnel?sslmode=disable",
)


def test_normalize_not_for_collapses_whitespace_and_caps_length():
    assert normalize_not_for("  not   for:  pricing ") == "not for: pricing"
    assert normalize_not_for(None) == ""
    assert len(normalize_not_for("x" * 1000)) == 300


def test_pattern_draft_schema_carries_not_for():
    draft = OKFPatternDraft(canonical_question="Q?", answer_text="A", trigger_phrases=["q"], not_for="Not for **refunds**")
    assert "not_for" in OKFPatternDraft.model_json_schema()["properties"]
    assert "**" not in draft.not_for


def test_mined_trigger_phrases_are_short_distinct_and_capped():
    texts = [
        "how much is shipping",
        "How much is shipping",
        "hi i ordered last week and my order number is 12345 and i would like to know how much shipping costs please",
        "shipping cost?",
        "what does delivery cost",
        "price of delivery",
        "delivery fee",
        "cost to ship",
        "how much to ship to canada",
    ]
    picked = example_phrases(texts)
    assert len(picked) == 6
    assert all(len(p.split()) <= 15 for p in picked)
    assert len({p.lower() for p in picked}) == 6
    assert picked[0] == "cost to ship"  # shortest first, ties alphabetical


@pytest_asyncio.fixture
async def pool_and_account():
    try:
        pool = await create_db_pool(DATABASE_URL)
    except Exception:
        if os.getenv("CI"):
            pytest.fail("Postgres is not reachable")
        pytest.skip("Postgres is not reachable")
    account_id = uuid.uuid4()
    await pool.execute("INSERT INTO accounts (id, name, plan) VALUES ($1, 'FAQ Approval Test', 'self_hosted')", account_id)
    try:
        yield pool, account_id
    finally:
        for table in ("audit_logs", "automation_suggestions", "kb_concepts", "patterns", "kb_ingestions"):
            await pool.execute(f"DELETE FROM {table} WHERE account_id = $1", account_id)
        await pool.execute("DELETE FROM accounts WHERE id = $1", account_id)
        await pool.close()


@pytest.mark.asyncio
async def test_compile_paste_never_inserts_faqs_directly(pool_and_account):
    pool, account_id = pool_and_account
    db = ScopedDB(pool, account_id)
    client = MagicMock()
    client.complete = AsyncMock(return_value={
        "concepts": [],
        "patterns": [{"canonical_question": "When open?", "answer_text": "9 to 5", "trigger_phrases": ["hours"], "not_for": "holidays"}],
    })
    client.embed = AsyncMock(return_value=[0.1] * 1536)
    cfg = MagicMock(analysis_model="a", embedding_model="e")
    with patch("kb_service.get_ai_config", AsyncMock(return_value=cfg)), \
         patch("kb_service.publish_suggestion_created", AsyncMock()):
        response = await kb_service.compile_paste(db, "docs", client_factory=lambda _: client)

    assert len(response.suggestion_ids) == 1
    assert await pool.fetchval("SELECT COUNT(*) FROM patterns WHERE account_id = $1", account_id) == 0
    row = await pool.fetchrow("SELECT status, proposed_payload FROM automation_suggestions WHERE account_id = $1", account_id)
    assert row["status"] == "pending"
    assert json.loads(row["proposed_payload"])["not_for"] == "holidays"
    client.embed.assert_not_awaited()  # FAQs are not embedded


@pytest.mark.asyncio
async def test_approving_a_suggestion_creates_an_approved_faq(pool_and_account):
    pool, account_id = pool_and_account
    db = ScopedDB(pool, account_id)
    sugg_id = uuid.uuid4()
    await pool.execute(
        """
        INSERT INTO automation_suggestions (id, account_id, type, proposed_payload, confidence, status)
        VALUES ($1, $2, 'new_pattern', $3, 1.0, 'pending')
        """,
        sugg_id, account_id,
        json.dumps({"canonical_question": "When open?", "answer_text": "9 to 5", "trigger_phrases": ["hours"], "not_for": " holidays "}),
    )
    assert await pool.fetchval("SELECT COUNT(*) FROM patterns WHERE account_id = $1", account_id) == 0

    await kb_service.approve_suggestion(db, sugg_id, None, client_factory=lambda _: MagicMock(embed=AsyncMock()))

    row = await pool.fetchrow("SELECT approved_at, not_for FROM patterns WHERE account_id = $1", account_id)
    assert row["approved_at"] is not None and row["not_for"] == "holidays"


@pytest.mark.asyncio
async def test_ingestion_publish_only_creates_faqs_for_approved_rows(pool_and_account):
    pool, account_id = pool_and_account
    ingestion_id = uuid.uuid4()
    await pool.execute("INSERT INTO kb_ingestions (id, account_id, raw_text, status) VALUES ($1, $2, 'raw', 'publishing')", ingestion_id, account_id)
    for position, (question, status) in enumerate([("Approved?", "approved"), ("Draft?", "draft"), ("Rejected?", "rejected")]):
        await pool.execute(
            """
            INSERT INTO kb_ingestion_patterns (ingestion_id, position, canonical_question, answer_text, trigger_phrases, not_for, status)
            VALUES ($1, $2, $3, 'answer', ARRAY['x'], 'not for y', $4)
            """,
            ingestion_id, position, question, status,
        )
    cfg = MagicMock(embedding_model="e")
    with patch("ingestions.get_ai_config", AsyncMock(return_value=cfg)), \
         patch("ingestions.provider_client", return_value=MagicMock(embed=AsyncMock(return_value=[0.1] * 1536))):
        await ingestions._publish(pool, {"id": ingestion_id, "account_id": account_id, "requested_by": None})

    rows = await pool.fetch("SELECT canonical_question, approved_at, not_for FROM patterns WHERE account_id = $1", account_id)
    assert [r["canonical_question"] for r in rows] == ["Approved?"]
    assert rows[0]["approved_at"] is not None and rows[0]["not_for"] == "not for y"


@pytest.mark.asyncio
async def test_no_code_path_inserts_patterns_without_approval_marker():
    """Static guard: every INSERT INTO patterns in the service sets approved_at."""
    import pathlib
    import re

    root = pathlib.Path(__file__).resolve().parent.parent
    for path in root.glob("*.py"):
        text = path.read_text()
        for match in re.finditer(r"INSERT INTO patterns\s*\((.*?)\)", text, re.S):
            assert "approved_at" in match.group(1), f"{path.name}: pattern insert without approved_at"
