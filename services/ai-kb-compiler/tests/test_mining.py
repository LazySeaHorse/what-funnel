import os
import uuid
import json
import pytest
from unittest.mock import patch, AsyncMock
from datetime import datetime, timedelta, timezone

from db import ScopedDB, create_db_pool
from config import config
from crypto import encrypt, get_key_bytes
from mining import run_mining

DATABASE_URL = os.getenv(
    "DATABASE_URL",
    "postgres://whatfunnel:whatfunnel@postgres:5432/whatfunnel?sslmode=disable"
)

async def setup_test_data():
    pool = await create_db_pool(DATABASE_URL)
    account_id = uuid.uuid4()
    
    # Encrypted config
    provider_json = json.dumps({
        "api_key": "sk-test",
        "base_url": "https://api.openai.com/v1"
    })
    key = get_key_bytes(config.APP_ENCRYPTION_KEY)
    encrypted_config = encrypt(key, provider_json.encode("utf-8"))

    async with pool.acquire() as conn:
        await conn.execute(
            "INSERT INTO accounts (id, name, plan, ai_provider_config, created_at) VALUES ($1, 'Mining Account', 'self_hosted', $2, $3)",
            account_id, encrypted_config, datetime.now(timezone.utc) - timedelta(days=1)
        )
    return pool, account_id

async def teardown_test_data(pool, account_id):
    async with pool.acquire() as conn:
        await conn.execute("DELETE FROM audit_logs WHERE account_id = $1", account_id)
        await conn.execute("DELETE FROM kb_mining_runs WHERE account_id = $1", account_id)
        await conn.execute("DELETE FROM automation_suggestions WHERE account_id = $1", account_id)
        await conn.execute("DELETE FROM messages WHERE account_id = $1", account_id)
        await conn.execute("DELETE FROM conversations WHERE account_id = $1", account_id)
        await conn.execute("DELETE FROM contacts WHERE account_id = $1", account_id)
        await conn.execute("DELETE FROM channels WHERE account_id = $1", account_id)
        await conn.execute("DELETE FROM accounts WHERE id = $1", account_id)
    await pool.close()

@pytest.mark.asyncio
async def test_mining_scanned_count_cutoff():
    """Verify that if there are <5 messages, it logs a mining run with 0 clusters and returns early."""
    pool, account_id = await setup_test_data()
    db = ScopedDB(pool, account_id)

    try:
        # Prepopulate a channel, contact, conversation, and only 3 messages
        async with pool.acquire() as conn:
            channel_id = uuid.uuid4()
            contact_id = uuid.uuid4()
            convo_id = uuid.uuid4()
            await conn.execute(
                "INSERT INTO channels (id, account_id, type, bridge_identity, status) VALUES ($1, $2, 'matrix_whatsapp', 'whatsapp', 'connected')",
                channel_id, account_id
            )
            await conn.execute(
                "INSERT INTO contacts (id, account_id, channel_id, external_identity, display_name) VALUES ($1, $2, $3, 'c1', 'C1')",
                contact_id, account_id, channel_id
            )
            await conn.execute(
                "INSERT INTO conversations (id, account_id, contact_id, channel_id, status) VALUES ($1, $2, $3, $4, 'open')",
                convo_id, account_id, contact_id, channel_id
            )
            # Insert 3 messages
            for i in range(3):
                msg_id = await conn.fetchval(
                    """
                    INSERT INTO messages (account_id, conversation_id, direction, sender_type, content_type, content)
                    VALUES ($1, $2, 'inbound', 'contact', 'text', $3::jsonb)
                    RETURNING id
                    """,
                    account_id, convo_id, json.dumps({"text": f"Msg {i}"})
                )
                await conn.execute(
                    """
                    INSERT INTO ai_answer_events (account_id, conversation_id, message_id, stage_matched, confidence, action)
                    VALUES ($1, $2, $3, 'none', NULL, 'flagged_human')
                    """,
                    account_id, convo_id, msg_id
                )

        result = await run_mining(db)
        assert result["messages_scanned"] == 3
        assert result["clusters_found"] == 0
        assert result["suggestions_created"] == 0

        # Verify DB entry
        async with pool.acquire() as conn:
            run = await conn.fetchrow("SELECT * FROM kb_mining_runs WHERE account_id = $1", account_id)
            assert run is not None
            assert run["messages_scanned"] == 3
            assert run["clusters_found"] == 0
            assert run["suggestions_created"] == 0
    finally:
        await teardown_test_data(pool, account_id)

@pytest.mark.asyncio
async def test_mining_clustering_and_cutoff():
    """
    Verify greedy clustering:
    - Group A (3 messages): similar -> should form 1 suggestion.
    - Group B (3 messages): similar -> should form 1 suggestion.
    - Message C (1 message): unique -> should form a cluster, but gets discarded (<3 cutoff).
    """
    pool, account_id = await setup_test_data()
    db = ScopedDB(pool, account_id)

    try:
        # Prepopulate structure
        async with pool.acquire() as conn:
            channel_id = uuid.uuid4()
            contact_id = uuid.uuid4()
            convo_id = uuid.uuid4()
            await conn.execute(
                "INSERT INTO channels (id, account_id, type, bridge_identity, status) VALUES ($1, $2, 'matrix_whatsapp', 'whatsapp', 'connected')",
                channel_id, account_id
            )
            await conn.execute(
                "INSERT INTO contacts (id, account_id, channel_id, external_identity, display_name) VALUES ($1, $2, $3, 'c1', 'C1')",
                contact_id, account_id, channel_id
            )
            await conn.execute(
                "INSERT INTO conversations (id, account_id, contact_id, channel_id, status) VALUES ($1, $2, $3, $4, 'open')",
                convo_id, account_id, contact_id, channel_id
            )

            # Insert 7 inbound text messages
            texts = [
                # Group A: opening hours questions
                "what time do you open?",
                "when are you open?",
                "opening hours please",
                # Group B: pricing questions
                "how much does it cost?",
                "what is the price?",
                "pricing options?",
                # Unique message C
                "do you have parking?"
            ]
            for txt in texts:
                msg_id = await conn.fetchval(
                    """
                    INSERT INTO messages (account_id, conversation_id, direction, sender_type, content_type, content)
                    VALUES ($1, $2, 'inbound', 'contact', 'text', $3::jsonb)
                    RETURNING id
                    """,
                    account_id, convo_id, json.dumps({"text": txt})
                )
                await conn.execute(
                    """
                    INSERT INTO ai_answer_events (account_id, conversation_id, message_id, stage_matched, confidence, action)
                    VALUES ($1, $2, $3, 'none', NULL, 'flagged_human')
                    """,
                    account_id, convo_id, msg_id
                )

        # Mock embeddings
        # Group A embeddings will be close to [1.0, 0.0, ...]
        # Group B embeddings will be close to [0.0, 1.0, ...]
        # Msg C embedding will be close to [0.0, 0.0, 1.0, ...]
        vA = [1.0] + [0.0]*1535
        vB = [0.0, 1.0] + [0.0]*1534
        vC = [0.0, 0.0, 1.0] + [0.0]*1533

        embeddings_map = {
            "what time do you open?": vA,
            "when are you open?": vA,
            "opening hours please": vA,
            "how much does it cost?": vB,
            "what is the price?": vB,
            "pricing options?": vB,
            "do you have parking?": vC,
        }

        async def mock_embed(key, base_url, model, text):
            return embeddings_map[text]

        mock_complete_resp = {
            "canonical_question": "Drafted question",
            "answer_markdown": "Drafted answer"
        }

        with patch("mining.embed", side_effect=mock_embed), \
             patch("mining.complete", return_value=mock_complete_resp):

            result = await run_mining(db)
            
            assert result["messages_scanned"] == 7
            # 3 clusters: Group A, Group B, and Message C
            assert result["clusters_found"] == 3
            # Group A and Group B created suggestions, Message C discarded because size < 3
            assert result["suggestions_created"] == 2

            # Verify suggestions in DB
            async with pool.acquire() as conn:
                suggs = await conn.fetch(
                    "SELECT type, proposed_payload, confidence FROM automation_suggestions WHERE account_id = $1",
                    account_id
                )
                assert len(suggs) == 2
                for s in suggs:
                    assert s["type"] == "new_pattern"
                    payload = json.loads(s["proposed_payload"])
                    assert "canonical_question" in payload
                    assert "answer_markdown" in payload
                    assert len(payload["trigger_phrases"]) == 3
                    
                    # Confidence for size 3 cluster should be calculated as min(1.0, 3/10) * avg_similarity = 0.3 * 1.0 = 0.3
                    assert abs(float(s["confidence"]) - 0.3) < 0.01

                # Verify run table updated
                run = await conn.fetchrow(
                    "SELECT * FROM kb_mining_runs WHERE account_id = $1 ORDER BY run_at DESC LIMIT 1",
                    account_id
                )
                assert run["messages_scanned"] == 7
                assert run["clusters_found"] == 3
                assert run["suggestions_created"] == 2
    finally:
        await teardown_test_data(pool, account_id)
