import os
import pytest
import uuid
import json
from datetime import datetime, timedelta, timezone
from unittest.mock import AsyncMock, MagicMock, patch

from main import (
    process_conversation_updated,
    process_conversation_closed,
    review_due_cooldown,
    send_ai_message,
)
from control import HUMAN_REVIEW_REPLY
from db import ScopedDB
from whatfunnel_ai import AIConfiguration


def ai_config():
    return AIConfiguration(
        api_key="key",
        base_url="https://provider.example/v1",
        analysis_model="analysis-model",
        reply_model="reply-model",
        embedding_model="embedding-model",
    )

@pytest.fixture(autouse=True)
def disable_debounce(monkeypatch):
    monkeypatch.setattr("main.config.AI_DEBOUNCE_ENABLED", False)

# A mock Record class to simulate asyncpg row returns
class MockRecord(dict):
    def __getattr__(self, name):
        try:
            return self[name]
        except KeyError:
            raise AttributeError(name)

@pytest.mark.asyncio
async def test_human_takeover_pauses_ai():
    # Test that human takeover blocks auto-send when mixed answering is disabled.
    db_pool = MagicMock()
    redis_client = AsyncMock()

    account_id = uuid.uuid4()
    convo_id = uuid.uuid4()
    message_id = uuid.uuid4()

    data = {
        "account_id": str(account_id),
        "conversation_id": str(convo_id),
        "message_id": str(message_id)
    }

    mock_msg = MockRecord({
        "direction": "inbound",
        "content_type": "text",
        "content": json.dumps({"text": "Hello"})
    })
    # AI Mode Active is False (human has taken over)
    mock_convo = MockRecord({
        "assigned_user_ids": [], "state": "paused_human", "state_reason": "human_message_sent",
        "reply_override": "inherit", "run_state": "idle", "generation_epoch": 1,
        "cooldown_level": 0, "unanswered_count": 0, "unanswered_window_started_at": None,
    })
    # mixed conversations answering is False (default)
    mock_account = MockRecord({
        "settings": json.dumps({
            "ai_enabled": True,
            "ai_reply_mode_default": "auto_send",
            "ai_may_auto_answer_mixed_conversations": False
        })
    })

    async def mock_fetchrow(query, *args):
        if "messages" in query:
            return mock_msg
        if "conversations" in query:
            return mock_convo
        if "accounts" in query:
            return mock_account
        return None

    with patch("main.ScopedDB") as MockScopedDB:
        db_instance = MockScopedDB.return_value
        db_instance.fetchrow = mock_fetchrow
        db_instance.execute = AsyncMock()
        db_instance.account_id = account_id

        await process_conversation_updated(data, db_pool, redis_client)

        # Admission stops before all inference and answer-event work.
        db_instance.execute.assert_not_awaited()
        redis_client.xadd.assert_not_awaited()


@pytest.mark.asyncio
async def test_unanswerable_auto_reply_enters_cooldown_and_sends_acknowledgement():
    db_pool = MagicMock()
    redis_client = AsyncMock()
    account_id = uuid.uuid4()
    convo_id = uuid.uuid4()
    message_id = uuid.uuid4()

    message = MockRecord({
        "direction": "inbound", "content_type": "text",
        "content": json.dumps({"text": "Can you answer this unknown question?"}),
    })
    conversation = MockRecord({
        "assigned_user_ids": [], "state": "active", "state_reason": None,
        "reply_override": "inherit", "run_state": "idle", "generation_epoch": 0,
        "cooldown_level": 0, "unanswered_count": 0,
        "unanswered_window_started_at": None,
    })
    account = MockRecord({"settings": json.dumps({
        "ai_enabled": True, "ai_reply_mode_default": "auto_send",
    })})

    async def mock_fetchrow(query, *args):
        if "SELECT direction, content_type" in query:
            return message
        if "SELECT c.assigned_user_ids" in query:
            return conversation
        if "SELECT settings FROM accounts" in query:
            return account
        if "SET run_state = 'replying'" in query:
            return MockRecord({"generation_epoch": 1})
        if "SET state = 'cooldown'" in query:
            return MockRecord({"generation_epoch": 2})
        return None

    with patch("main.ScopedDB") as MockScopedDB, \
         patch("main.get_ai_config", AsyncMock(side_effect=ValueError("not configured"))), \
         patch("main.send_ai_message", AsyncMock(return_value={"id": str(uuid.uuid4())})) as send:
        db = MockScopedDB.return_value
        db.account_id = account_id
        db.fetchrow = mock_fetchrow
        db.fetch = AsyncMock(return_value=[])
        db.execute = AsyncMock()

        await process_conversation_updated({
            "account_id": str(account_id),
            "conversation_id": str(convo_id),
            "message_id": str(message_id),
        }, db_pool, redis_client)

        send.assert_awaited_once()
        assert send.await_args.args[2] == HUMAN_REVIEW_REPLY
        assert send.await_args.args[3] == 2
        assert send.await_args.args[4] == "human_review_ack"
        control_events = [
            json.loads(call.args[1]["payload"])
            for call in redis_client.xadd.call_args_list
            if call.args[0] == "ai.control.updated"
        ]
        assert control_events[-1]["state"] == "cooldown"


@pytest.mark.asyncio
async def test_due_cooldown_judge_blocks_likely_spam_without_knowledge_or_tools():
    account_id = uuid.uuid4()
    convo_id = uuid.uuid4()
    db_pool = AsyncMock()
    db_pool.fetchrow.return_value = MockRecord({
        "account_id": account_id, "conversation_id": convo_id,
        "cooldown_level": 2, "generation_epoch": 7,
    })
    redis_client = AsyncMock()

    client = MagicMock()
    client.complete = AsyncMock(return_value={"verdict": "likely_spam"})
    with patch("main.ScopedDB") as MockScopedDB, \
         patch("main.get_ai_config", AsyncMock(return_value=ai_config())), \
         patch("main.provider_client", return_value=client):
        db = MockScopedDB.return_value
        db.fetch = AsyncMock(return_value=[
            MockRecord({"sender_type": "contact", "content": json.dumps({"text": "same unknown question"})}),
        ])
        db.execute = AsyncMock()

        assert await review_due_cooldown(db_pool, redis_client) is True

        assert client.complete.await_args.args[0] == "analysis-model"
        prompt = client.complete.await_args.args[1]
        assert len(prompt) == 1
        assert "UNTRUSTED TRANSCRIPT" in prompt[0]["content"]
        assert "same unknown question" in prompt[0]["content"]
        transition = db.execute.await_args
        assert transition.args[3] == "blocked_spam"
        assert transition.args[4] == "judge_likely_spam"
        assert "FROM previous, updated" in transition.args[0]

@pytest.mark.asyncio
async def test_summary_debounce():
    # Testdebounce logic on conversation closed
    db_pool = MagicMock()
    redis_client = AsyncMock()

    account_id = uuid.uuid4()
    convo_id = uuid.uuid4()

    data = {
        "account_id": str(account_id),
        "conversation_id": str(convo_id)
    }

    # Case 1: Elapsed < 60s -> should skip
    with patch("main.ScopedDB") as MockScopedDB:
        db_instance = MockScopedDB.return_value
        db_instance.fetchval = AsyncMock(return_value=10) # 10 messages now
        
        # Last generated summary was 30 seconds ago, with 5 messages
        db_instance.fetchrow = AsyncMock(return_value=MockRecord({
            "generated_at": datetime.now(timezone.utc) - timedelta(seconds=30),
            "message_count_at_generation": 5
        }))
        db_instance.execute = AsyncMock()

        await process_conversation_closed(data, db_pool, redis_client)
        
        # DB execute should not be called to update summary since < 60s elapsed
        db_instance.execute.assert_not_called()

    # Case 2: Elapsed >= 60s but count has not increased -> should skip
    with patch("main.ScopedDB") as MockScopedDB:
        db_instance = MockScopedDB.return_value
        db_instance.fetchval = AsyncMock(return_value=5) # still 5 messages
        db_instance.fetchrow = AsyncMock(return_value=MockRecord({
            "generated_at": datetime.now(timezone.utc) - timedelta(seconds=70),
            "message_count_at_generation": 5
        }))
        db_instance.execute = AsyncMock()

        await process_conversation_closed(data, db_pool, redis_client)
        db_instance.execute.assert_not_called()

    # Case 3: Elapsed >= 60s and message count increased -> should regenerate!
    with patch("main.ScopedDB") as MockScopedDB:
        db_instance = MockScopedDB.return_value
        db_instance.fetchval = AsyncMock(side_effect=lambda query, *args: 10 if "messages" in query else None)
        
        # Return mock records for fetchrow queries
        async def custom_fetchrow(query, *args):
            if "summaries" in query:
                return MockRecord({
                    "generated_at": datetime.now(timezone.utc) - timedelta(seconds=70),
                    "message_count_at_generation": 5
                })
            if "accounts" in query:
                return MockRecord({
                    "settings": json.dumps({
                        "summary_schema": [
                            {"key": "customer_wants", "label": "Wants", "description": "w"}
                        ]
                    })
                })
            return None

        db_instance.fetchrow = custom_fetchrow
        db_instance.fetch = AsyncMock(return_value=[
            MockRecord({"direction": "inbound", "sender_type": "contact", "content": json.dumps({"text": "Hello"})})
        ])
        db_instance.execute = AsyncMock()

        # Mock LLM calls
        mock_summary = {"customer_wants": "Help with code"}
        client = MagicMock()
        client.complete = AsyncMock(return_value=mock_summary)

        with patch("main.get_ai_config", AsyncMock(return_value=ai_config())), \
             patch("main.provider_client", return_value=client):

            await process_conversation_closed(data, db_pool, redis_client)

            assert client.complete.await_args.args[0] == "analysis-model"
            assert db_instance.execute.call_count == 1
            summary_call_args = db_instance.execute.call_args_list[0][0]
            assert "INSERT INTO conversation_summaries" in summary_call_args[0]
            assert summary_call_args[3] == json.dumps(mock_summary)
            assert summary_call_args[4] == 10 # current count


@pytest.mark.asyncio
async def test_send_ai_message_canonical_manager_role():
    account_id = uuid.uuid4()
    convo_id = uuid.uuid4()

    with patch("httpx.AsyncClient.post") as mock_post:
        mock_response = MagicMock()
        mock_response.status_code = 200
        mock_response.json.return_value = {"id": str(uuid.uuid4())}
        mock_response.raise_for_status = MagicMock()
        mock_post.return_value = mock_response

        res = await send_ai_message(
            account_id=account_id,
            conversation_id=convo_id,
            text="Hello from AI",
            generation_epoch=3,
            purpose="reply",
            idempotency_key="ai-reply:123",
        )

        assert mock_post.called
        call_args, call_kwargs = mock_post.call_args
        assert call_args[0] == f"http://conversation-svc:8083/internal/conversations/{convo_id}/send"
        
        headers = call_kwargs["headers"]
        assert headers["X-User-Role"] == "manager"
        assert headers["X-Account-ID"] == str(account_id)
        assert "X-Internal-Token" in headers

        body = call_kwargs["json"]
        assert body["sender_type"] == "ai"
        assert body["text"] == "Hello from AI"
        assert body["generation_epoch"] == 3
        assert body["message_purpose"] == "reply"
        assert body["idempotency_key"] == "ai-reply:123"


@pytest.mark.asyncio
async def test_send_ai_message_prefers_internal_service_token():
    account_id = uuid.uuid4()
    convo_id = uuid.uuid4()

    with patch.dict(os.environ, {
        "INTERNAL_SERVICE_TOKEN": "primary-internal-token-32-chars",
        "SESSION_SECRET": "fallback-session-secret-32-chars",
    }):
        with patch("httpx.AsyncClient.post") as mock_post:
            mock_response = MagicMock()
            mock_response.status_code = 200
            mock_response.json.return_value = {"id": str(uuid.uuid4())}
            mock_post.return_value = mock_response

            await send_ai_message(
                account_id=account_id,
                conversation_id=convo_id,
                text="Hello from AI",
                generation_epoch=1,
                purpose="reply",
                idempotency_key="ai-reply:1",
            )

            headers = mock_post.call_args[1]["headers"]
            assert headers["X-Internal-Token"] == "primary-internal-token-32-chars"


@pytest.mark.asyncio
async def test_send_ai_message_ignores_session_secret_and_fails_closed():
    env = {k: v for k, v in os.environ.items()
           if k not in ("INTERNAL_SERVICE_TOKEN", "ALLOW_INSECURE_INTERNAL_AUTH")}
    env["SESSION_SECRET"] = "fallback-session-secret-32-chars"

    with patch.dict(os.environ, env, clear=True):
        with patch("main.config.INTERNAL_SERVICE_TOKEN", ""), \
             patch("config.config.ALLOW_INSECURE_INTERNAL_AUTH", False), \
             patch("httpx.AsyncClient.post") as mock_post:
            with pytest.raises(RuntimeError, match="INTERNAL_SERVICE_TOKEN"):
                await send_ai_message(
                    account_id=uuid.uuid4(),
                    conversation_id=uuid.uuid4(),
                    text="Hello from AI",
                    generation_epoch=1,
                    purpose="reply",
                    idempotency_key="ai-reply:2",
                )
            assert not mock_post.called


@pytest.mark.asyncio
async def test_send_ai_message_insecure_opt_in_allows_missing_token():
    env = {k: v for k, v in os.environ.items() if k != "INTERNAL_SERVICE_TOKEN"}
    env["ALLOW_INSECURE_INTERNAL_AUTH"] = "true"

    with patch.dict(os.environ, env, clear=True):
        with patch("main.config.INTERNAL_SERVICE_TOKEN", ""), \
             patch("httpx.AsyncClient.post") as mock_post:
            mock_response = MagicMock()
            mock_response.json.return_value = {"id": str(uuid.uuid4())}
            mock_post.return_value = mock_response
            await send_ai_message(
                account_id=uuid.uuid4(),
                conversation_id=uuid.uuid4(),
                text="Hello from AI",
                generation_epoch=1,
                purpose="reply",
                idempotency_key="ai-reply:3",
            )
            assert mock_post.call_args[1]["headers"]["X-Internal-Token"] == ""


