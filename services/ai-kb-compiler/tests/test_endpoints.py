import os
import uuid
import json
import pytest
from unittest.mock import patch, AsyncMock
from fastapi.testclient import TestClient

from main import app
from db import create_db_pool
from crypto import encrypt, get_key_bytes
from config import config

DATABASE_URL = os.getenv(
    "DATABASE_URL",
    "postgres://whatfunnel:whatfunnel@postgres:5432/whatfunnel?sslmode=disable"
)

# Mock concepts
mock_concepts_3_or_fewer = {
    "concepts": [
        {"type": "faq", "title": "About Us", "tags": ["info"], "body_markdown": "We are WhatFunnel."},
        {"type": "hours", "title": "Opening Hours", "tags": ["schedule"], "body_markdown": "9 AM to 5 PM."}
    ]
}

mock_concepts_more_than_3 = {
    "concepts": [
        {"type": "faq", "title": "FAQ 1", "tags": ["faq"], "body_markdown": "Answer 1"},
        {"type": "faq", "title": "FAQ 2", "tags": ["faq"], "body_markdown": "Answer 2"},
        {"type": "faq", "title": "FAQ 3", "tags": ["faq"], "body_markdown": "Answer 3"},
        {"type": "faq", "title": "FAQ 4", "tags": ["faq"], "body_markdown": "Answer 4"}
    ]
}

async def setup_test_data():
    pool = await create_db_pool(DATABASE_URL)
    app.state.db = pool

    account_id = uuid.uuid4()
    user_id = uuid.uuid4()

    # Create hex encrypted provider config
    provider_json = json.dumps({
        "api_key": "sk-test",
        "base_url": "https://api.openai.com/v1"
    })
    key = get_key_bytes(config.APP_ENCRYPTION_KEY)
    encrypted_config = encrypt(key, provider_json.encode("utf-8"))

    async with pool.acquire() as conn:
        await conn.execute(
            "INSERT INTO accounts (id, name, plan, ai_provider_config) VALUES ($1, 'Test Account', 'self_hosted', $2)",
            account_id, encrypted_config
        )
        await conn.execute(
            "INSERT INTO users (id, account_id, email, role) VALUES ($1, $2, 'test@example.com', 'admin')",
            user_id, account_id
        )

    return pool, account_id, user_id

async def teardown_test_data(pool, account_id):
    async with pool.acquire() as conn:
        await conn.execute("DELETE FROM audit_logs WHERE account_id = $1", account_id)
        await conn.execute("DELETE FROM kb_concepts WHERE account_id = $1", account_id)
        await conn.execute("DELETE FROM patterns WHERE account_id = $1", account_id)
        await conn.execute("DELETE FROM automation_suggestions WHERE account_id = $1", account_id)
        await conn.execute("DELETE FROM users WHERE account_id = $1", account_id)
        await conn.execute("DELETE FROM accounts WHERE id = $1", account_id)
    await pool.close()

@pytest.mark.asyncio
async def test_compile_paste_three_or_fewer():
    pool, account_id, user_id = await setup_test_data()

    try:
        # Mock complete to return 2 concepts
        # Mock embed to return 1536 float list
        with patch("main.complete", return_value=mock_concepts_3_or_fewer), \
             patch("main.embed", return_value=[0.05] * 1536):

            with TestClient(app) as client:
                headers = {
                    "X-Account-ID": str(account_id),
                    "X-User-ID": str(user_id)
                }
                response = client.post(
                    "/internal/kb/compile-paste",
                    json={"raw_text": "We are open 9am to 5pm. We are WhatFunnel."},
                    headers=headers
                )
                assert response.status_code == 200
                res_json = response.json()
                assert "added_concepts" in res_json
                assert len(res_json["added_concepts"]) == 2
                
                # Verify they are stored in DB
                async with pool.acquire() as conn:
                    rows = await conn.fetch("SELECT id, slug, source FROM kb_concepts WHERE account_id = $1", account_id)
                    assert len(rows) == 2
                    for r in rows:
                        assert r["source"] == "owner_pasted"
                    
                    # Check audit logs
                    audit_rows = await conn.fetch("SELECT action FROM audit_logs WHERE account_id = $1", account_id)
                    actions = [a["action"] for a in audit_rows]
                    assert "kb_concept.created" in actions
    finally:
        await teardown_test_data(pool, account_id)

@pytest.mark.asyncio
async def test_compile_paste_more_than_three():
    pool, account_id, user_id = await setup_test_data()

    try:
        with patch("main.complete", return_value=mock_concepts_more_than_3):
            with TestClient(app) as client:
                headers = {
                    "X-Account-ID": str(account_id),
                    "X-User-ID": str(user_id)
                }
                response = client.post(
                    "/internal/kb/compile-paste",
                    json={"raw_text": "Many things..."},
                    headers=headers
                )
                assert response.status_code == 200
                res_json = response.json()
                assert "suggestion_ids" in res_json
                assert len(res_json["suggestion_ids"]) == 4

                # Verify suggestions are stored
                async with pool.acquire() as conn:
                    rows = await conn.fetch(
                        "SELECT id, type, status, confidence FROM automation_suggestions WHERE account_id = $1",
                        account_id
                    )
                    assert len(rows) == 4
                    for r in rows:
                        assert r["type"] == "new_kb_concept"
                        assert r["status"] == "pending"
                        assert r["confidence"] == 1.0

                    # Check audit logs
                    audit_rows = await conn.fetch(
                        "SELECT action FROM audit_logs WHERE account_id = $1 AND action = 'automation_suggestion.created'",
                        account_id
                    )
                    assert len(audit_rows) == 4
    finally:
        await teardown_test_data(pool, account_id)

@pytest.mark.asyncio
async def test_slug_collision_resolution():
    pool, account_id, user_id = await setup_test_data()

    try:
        with patch("main.complete", return_value={
            "concepts": [
                {"type": "faq", "title": "Duplicate", "tags": [], "body_markdown": "First"},
                {"type": "faq", "title": "Duplicate", "tags": [], "body_markdown": "Second"}
            ]
        }), patch("main.embed", return_value=[0.01] * 1536):
            with TestClient(app) as client:
                headers = {
                    "X-Account-ID": str(account_id),
                    "X-User-ID": str(user_id)
                }
                response = client.post(
                    "/internal/kb/compile-paste",
                    json={"raw_text": "duplicate title"},
                    headers=headers
                )
                assert response.status_code == 200
                res_json = response.json()
                added = res_json["added_concepts"]
                assert len(added) == 2
                
                slugs = [a["slug"] for a in added]
                assert "duplicate" in slugs
                assert "duplicate-1" in slugs
    finally:
        await teardown_test_data(pool, account_id)

@pytest.mark.asyncio
async def test_concept_list_and_delete():
    pool, account_id, user_id = await setup_test_data()

    try:
        # Pre-populate a concept
        async with pool.acquire() as conn:
            concept_id = uuid.uuid4()
            await conn.execute(
                """
                INSERT INTO kb_concepts (id, account_id, slug, type, title, body_markdown, source)
                VALUES ($1, $2, 'test-slug', 'faq', 'Test Concept', 'Body text', 'owner_pasted')
                """,
                concept_id, account_id
            )

        with TestClient(app) as client:
            headers = {
                "X-Account-ID": str(account_id),
                "X-User-ID": str(user_id)
            }

            # 1. List
            resp_list = client.get("/internal/kb/concepts", headers=headers)
            assert resp_list.status_code == 200
            concepts = resp_list.json()["concepts"]
            assert len(concepts) == 1
            assert concepts[0]["title"] == "Test Concept"

            # 2. Delete
            resp_del = client.delete(f"/internal/kb/concepts/{concept_id}", headers=headers)
            assert resp_del.status_code == 200
            assert resp_del.json() == {"success": True}

            # Verify deletion from DB
            async with pool.acquire() as conn:
                row = await conn.fetchrow("SELECT id FROM kb_concepts WHERE id = $1", concept_id)
                assert row is None
                
                # Verify deleted audit log
                audit_row = await conn.fetchrow(
                    "SELECT action FROM audit_logs WHERE account_id = $1 AND action = 'kb_concept.deleted'",
                    account_id
                )
                assert audit_row is not None
    finally:
        await teardown_test_data(pool, account_id)

@pytest.mark.asyncio
async def test_suggestion_approve_reject():
    pool, account_id, user_id = await setup_test_data()

    try:
        # Insert a pending suggestion
        sugg_id = uuid.uuid4()
        proposed = {
            "title": "Approved Concept",
            "body_markdown": "This concept is approved",
            "type": "faq",
            "tags": ["approved"]
        }
        async with pool.acquire() as conn:
            await conn.execute(
                """
                INSERT INTO automation_suggestions (id, account_id, type, proposed_payload, confidence, status)
                VALUES ($1, $2, 'new_kb_concept', $3, 0.9, 'pending')
                """,
                sugg_id, account_id, json.dumps(proposed)
            )

        with patch("main.embed", return_value=[0.02] * 1536):
            with TestClient(app) as client:
                headers = {
                    "X-Account-ID": str(account_id),
                    "X-User-ID": str(user_id)
                }

                # 1. Approve
                resp_app = client.post(
                    f"/internal/kb/suggestions/{sugg_id}/approve",
                    json={"reviewed_by": str(user_id)},
                    headers=headers
                )
                assert resp_app.status_code == 200
                assert resp_app.json() == {"success": True}

                # Verify it's created as real concept
                async with pool.acquire() as conn:
                    row_concept = await conn.fetchrow(
                        "SELECT id, title, source FROM kb_concepts WHERE account_id = $1 AND title = 'Approved Concept'",
                        account_id
                    )
                    assert row_concept is not None
                    assert row_concept["source"] == "owner_pasted"

                    # Verify suggestion status updated
                    row_sugg = await conn.fetchrow(
                        "SELECT status, reviewed_by FROM automation_suggestions WHERE id = $1",
                        sugg_id
                    )
                    assert row_sugg["status"] == "approved"
                    assert row_sugg["reviewed_by"] == user_id

                # 2. Reject another suggestion
                sugg_id_2 = uuid.uuid4()
                async with pool.acquire() as conn:
                    await conn.execute(
                        """
                        INSERT INTO automation_suggestions (id, account_id, type, proposed_payload, confidence, status)
                        VALUES ($1, $2, 'new_kb_concept', $3, 0.8, 'pending')
                        """,
                        sugg_id_2, account_id, json.dumps(proposed)
                    )

                resp_rej = client.post(
                    f"/internal/kb/suggestions/{sugg_id_2}/reject",
                    json={"reviewed_by": str(user_id)},
                    headers=headers
                )
                assert resp_rej.status_code == 200
                assert resp_rej.json() == {"success": True}

                # Verify suggestion status updated to rejected
                async with pool.acquire() as conn:
                    row_sugg_2 = await conn.fetchrow(
                        "SELECT status FROM automation_suggestions WHERE id = $1",
                        sugg_id_2
                    )
                    assert row_sugg_2["status"] == "rejected"
    finally:
        await teardown_test_data(pool, account_id)


# ---------------------------------------------------------------------------
# Response-shape contract tests
# These tests assert the field names returned by each endpoint exactly match
# what the frontend now reads after the fix. They are the regression guard
# against field-name drift between backend and frontend.
# ---------------------------------------------------------------------------

@pytest.mark.asyncio
async def test_list_concepts_response_shape():
    """list_concepts must return concepts with type/body_markdown/source,
    NOT concept_type/content/source_type (the old phantom field names)."""
    pool, account_id, user_id = await setup_test_data()
    try:
        async with pool.acquire() as conn:
            concept_id = uuid.uuid4()
            await conn.execute(
                """
                INSERT INTO kb_concepts (id, account_id, slug, type, title, body_markdown, source)
                VALUES ($1, $2, 'shape-test-slug', 'faq', 'Shape Test', 'Body text here', 'owner_pasted')
                """,
                concept_id, account_id
            )

        with TestClient(app) as client:
            resp = client.get(
                "/internal/kb/concepts",
                headers={"X-Account-ID": str(account_id)}
            )
            assert resp.status_code == 200
            concepts = resp.json()["concepts"]
            assert len(concepts) >= 1
            c = next(x for x in concepts if x["id"] == str(concept_id))

            # Fields the fixed frontend reads
            assert c["type"] == "faq"
            assert c["body_markdown"] == "Body text here"
            assert c["source"] == "owner_pasted"
            assert "title" in c
            assert "slug" in c
            assert "id" in c

            # Phantom field names that no longer exist in the contract
            assert "concept_type" not in c
            assert "content" not in c
            assert "source_type" not in c
    finally:
        await teardown_test_data(pool, account_id)


@pytest.mark.asyncio
async def test_list_patterns_response_shape():
    """list_patterns must return patterns with canonical_question/answer_markdown/
    trigger_phrases, NOT pattern_name/representative_query/frequency_count/intent."""
    pool, account_id, user_id = await setup_test_data()
    try:
        async with pool.acquire() as conn:
            pattern_id = uuid.uuid4()
            await conn.execute(
                """
                INSERT INTO patterns (id, account_id, trigger_phrases, canonical_question, answer_markdown)
                VALUES ($1, $2, $3, 'What are your hours?', 'We are open 9am–5pm.')
                """,
                pattern_id, account_id, ["hours", "open", "schedule"]
            )

        with TestClient(app) as client:
            resp = client.get(
                "/internal/kb/patterns",
                headers={"X-Account-ID": str(account_id)}
            )
            assert resp.status_code == 200
            patterns = resp.json()["patterns"]
            assert len(patterns) >= 1
            p = next(x for x in patterns if x["id"] == str(pattern_id))

            # Fields the fixed frontend reads
            assert p["canonical_question"] == "What are your hours?"
            assert p["answer_markdown"] == "We are open 9am–5pm."
            assert isinstance(p["trigger_phrases"], list)
            assert "id" in p

            # Phantom field names that no longer exist in the contract
            assert "pattern_name" not in p
            assert "representative_query" not in p
            assert "frequency_count" not in p
            assert "intent" not in p
    finally:
        await teardown_test_data(pool, account_id)


@pytest.mark.asyncio
async def test_list_suggestions_response_shape():
    """list_suggestions must return suggestions whose proposed_payload is a parseable
    dict (not None), and whose top-level fields include type, confidence, and status.
    The frontend normalises proposed_payload into _payload to read title/body_markdown."""
    pool, account_id, user_id = await setup_test_data()
    try:
        sugg_id = uuid.uuid4()
        proposed = {
            "type": "faq",
            "title": "Shape Test Concept",
            "body_markdown": "Some body",
            "tags": []
        }
        async with pool.acquire() as conn:
            await conn.execute(
                """
                INSERT INTO automation_suggestions (id, account_id, type, proposed_payload, confidence, status)
                VALUES ($1, $2, 'new_kb_concept', $3, 0.85, 'pending')
                """,
                sugg_id, account_id, json.dumps(proposed)
            )

        with TestClient(app) as client:
            resp = client.get(
                "/internal/kb/suggestions?status_filter=pending",
                headers={"X-Account-ID": str(account_id)}
            )
            assert resp.status_code == 200
            suggestions = resp.json()["suggestions"]
            assert len(suggestions) >= 1
            s = next(x for x in suggestions if x["id"] == str(sugg_id))

            # Top-level fields the fixed frontend reads
            assert s["type"] == "new_kb_concept"
            assert s["confidence"] == 0.85
            assert s["status"] == "pending"

            # proposed_payload must be present and parseable to recover title/body
            raw_payload = s["proposed_payload"]
            if isinstance(raw_payload, str):
                payload = json.loads(raw_payload)
            else:
                payload = raw_payload
            assert payload["title"] == "Shape Test Concept"
            assert payload["body_markdown"] == "Some body"

            # Phantom top-level fields the old frontend expected (but never existed)
            assert "concept_type" not in s
            assert "content" not in s
            assert "source_type" not in s
            assert "title" not in s  # title lives inside proposed_payload, not top-level
    finally:
        await teardown_test_data(pool, account_id)
