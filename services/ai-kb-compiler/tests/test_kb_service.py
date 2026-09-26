import os
import uuid
import json
import pytest
from unittest.mock import AsyncMock, MagicMock
from fastapi import HTTPException

from config import config
from crypto import encrypt, get_key_bytes
from db import ScopedDB, create_db_pool
import kb_service
from slug import slugify, get_unique_slug
from audit import write_audit_log
from schemas import (
    CompilePasteSchema,
    UpdateConceptRequest,
    UpdatePatternRequest,
)

DATABASE_URL = os.getenv(
    "DATABASE_URL",
    "postgres://whatfunnel:whatfunnel@postgres:5432/whatfunnel?sslmode=disable"
)


def mock_provider(complete_result=None, embedding=None):
    client = MagicMock()
    client.complete = AsyncMock(return_value=complete_result or {})
    client.embed = AsyncMock(return_value=embedding or [0.03] * 1536)
    return client


async def setup_test_data():
    pool = await create_db_pool(DATABASE_URL)
    account_id = uuid.uuid4()
    user_id = uuid.uuid4()

    key = get_key_bytes(config.APP_ENCRYPTION_KEY)
    encrypted_api_key = encrypt(key, b"sk-test")

    async with pool.acquire() as conn:
        await conn.execute(
            "INSERT INTO accounts (id, name, plan) VALUES ($1, 'Unit Test Account', 'self_hosted')",
            account_id,
        )
        await conn.execute(
            """
            INSERT INTO account_ai_providers
                (account_id, base_url, encrypted_api_key, analysis_model, reply_model, embedding_model)
            VALUES ($1, 'https://api.openai.com/v1', $2, 'analysis-test', 'reply-test', 'embedding-test')
            """,
            account_id, encrypted_api_key,
        )
        await conn.execute(
            "INSERT INTO users (id, account_id, email, role) VALUES ($1, $2, 'service-test@example.com', 'manager')",
            user_id, account_id
        )

    db = ScopedDB(pool, account_id)
    return pool, db, account_id, user_id


async def teardown_test_data(pool, account_id):
    async with pool.acquire() as conn:
        await conn.execute("DELETE FROM audit_logs WHERE account_id = $1", account_id)
        await conn.execute("DELETE FROM kb_ingestions WHERE account_id = $1", account_id)
        await conn.execute("DELETE FROM kb_concepts WHERE account_id = $1", account_id)
        await conn.execute("DELETE FROM patterns WHERE account_id = $1", account_id)
        await conn.execute("DELETE FROM automation_suggestions WHERE account_id = $1", account_id)
        await conn.execute("DELETE FROM users WHERE account_id = $1", account_id)
        await conn.execute("DELETE FROM accounts WHERE id = $1", account_id)
    await pool.close()


def test_slugify():
    assert slugify("Hello World!") == "hello-world"
    assert slugify("Pricing & FAQ -- 2026") == "pricing-faq-2026"
    assert slugify("   Special Characters #$%   ") == "special-characters"
    assert slugify("") == ""


@pytest.mark.asyncio
async def test_slug_uniqueness():
    pool, db, account_id, user_id = await setup_test_data()
    try:
        # Initial slug
        slug1 = await get_unique_slug(db, "test-concept")
        assert slug1 == "test-concept"

        # Insert one concept with that slug
        await db.execute(
            """
            INSERT INTO kb_concepts (account_id, slug, type, title, body_text, source)
            VALUES ($1, $2, 'faq', 'Test Concept', 'Body', 'owner_pasted')
            """,
            account_id, slug1
        )

        # Next slug should append -1
        slug2 = await get_unique_slug(db, "test-concept")
        assert slug2 == "test-concept-1"

        # Insert that one too
        await db.execute(
            """
            INSERT INTO kb_concepts (account_id, slug, type, title, body_text, source)
            VALUES ($1, $2, 'faq', 'Test Concept 1', 'Body', 'owner_pasted')
            """,
            account_id, slug2
        )

        # Next slug should append -2
        slug3 = await get_unique_slug(db, "test-concept")
        assert slug3 == "test-concept-2"
    finally:
        await teardown_test_data(pool, account_id)


@pytest.mark.asyncio
async def test_audit_log_actor_handling():
    pool, db, account_id, user_id = await setup_test_data()
    try:
        # Write audit log with valid user
        await write_audit_log(
            db=db,
            actor_user_id=user_id,
            action="test.action",
            target_type="test",
            metadata={"detail": "with_user"}
        )

        # Write audit log with invalid non-existent user
        fake_user = uuid.uuid4()
        await write_audit_log(
            db=db,
            actor_user_id=fake_user,
            action="test.action_unknown_user",
            target_type="test",
            metadata={"detail": "without_user"}
        )

        rows = await db.fetch("SELECT action, actor_user_id FROM audit_logs WHERE account_id = $1 ORDER BY created_at", account_id)
        assert len(rows) == 2
        assert rows[0]["actor_user_id"] == user_id
        assert rows[1]["actor_user_id"] is None
    finally:
        await teardown_test_data(pool, account_id)


@pytest.mark.asyncio
async def test_kb_service_validation_errors():
    pool, db, account_id, user_id = await setup_test_data()
    try:
        # Blank raw text in create_ingestion
        with pytest.raises(HTTPException) as exc_info:
            await kb_service.create_ingestion(db, "   \n\t  ")
        assert exc_info.value.status_code == 422

        # Non-existent ingestion
        with pytest.raises(HTTPException) as exc_info:
            await kb_service.get_ingestion(db, uuid.uuid4())
        assert exc_info.value.status_code == 404

        # Non-existent concept deletion
        with pytest.raises(HTTPException) as exc_info:
            await kb_service.delete_concept(db, uuid.uuid4())
        assert exc_info.value.status_code == 404

        # Non-existent pattern deletion
        with pytest.raises(HTTPException) as exc_info:
            await kb_service.delete_pattern(db, uuid.uuid4())
        assert exc_info.value.status_code == 404

        # Non-existent suggestion approve
        with pytest.raises(HTTPException) as exc_info:
            await kb_service.approve_suggestion(
                db=db,
                sugg_uuid=uuid.uuid4(),
                reviewed_by_uuid=user_id,
                client_factory=lambda _: mock_provider()
            )
        assert exc_info.value.status_code == 404

        # Non-existent suggestion reject
        with pytest.raises(HTTPException) as exc_info:
            await kb_service.reject_suggestion(
                db=db,
                sugg_uuid=uuid.uuid4(),
                reviewed_by_uuid=user_id
            )
        assert exc_info.value.status_code == 404
    finally:
        await teardown_test_data(pool, account_id)


@pytest.mark.asyncio
async def test_kb_service_suggestion_edited_answer():
    pool, db, account_id, user_id = await setup_test_data()
    try:
        pattern_id = uuid.uuid4()
        await db.execute(
            """
            INSERT INTO patterns (id, account_id, canonical_question, answer_text, trigger_phrases)
            VALUES ($1, $2, 'What are your hours?', 'Old hours', ARRAY['hours'])
            """,
            pattern_id, account_id
        )

        sugg_id = uuid.uuid4()
        proposed = {
            "pattern_id": str(pattern_id),
            "answer_text": "Updated new hours: 9am-6pm",
        }
        await db.execute(
            """
            INSERT INTO automation_suggestions (id, account_id, type, proposed_payload, confidence, status)
            VALUES ($1, $2, 'edited_answer', $3, 0.95, 'pending')
            """,
            sugg_id, account_id, json.dumps(proposed)
        )

        client = mock_provider(embedding=[0.05] * 1536)
        await kb_service.approve_suggestion(
            db=db,
            sugg_uuid=sugg_id,
            reviewed_by_uuid=user_id,
            client_factory=lambda _: client
        )

        # Verify pattern is updated
        updated_pattern = await db.fetchrow(
            "SELECT answer_text, embedding FROM patterns WHERE id = $1",
            pattern_id
        )
        assert updated_pattern["answer_text"] == "Updated new hours: 9am-6pm"
        assert updated_pattern["embedding"] is not None

        # Verify suggestion status
        sugg_row = await db.fetchrow(
            "SELECT status, reviewed_by FROM automation_suggestions WHERE id = $1",
            sugg_id
        )
        assert sugg_row["status"] == "approved"
        assert sugg_row["reviewed_by"] == user_id
    finally:
        await teardown_test_data(pool, account_id)
