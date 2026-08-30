import pytest
import uuid
import json
from datetime import datetime, timedelta, timezone
from unittest.mock import AsyncMock, MagicMock, patch

from main import (
    apply_chat_auto_reply_override,
    process_conversation_updated,
    process_conversation_closed,
)
from db import ScopedDB

# A mock Record class to simulate asyncpg row returns
class MockRecord(dict):
    def __getattr__(self, name):
        try:
            return self[name]
        except KeyError:
            raise AttributeError(name)

def test_chat_auto_reply_override_only_restricts_the_effective_default():
    assert apply_chat_auto_reply_override("auto_send", None) == "auto_send"
    assert apply_chat_auto_reply_override("auto_send", True) == "auto_send"
    assert apply_chat_auto_reply_override("auto_send", False) == "draft_only"
    assert apply_chat_auto_reply_override("draft_only", None) == "draft_only"
    assert apply_chat_auto_reply_override("draft_only", True) == "draft_only"
    assert apply_chat_auto_reply_override("draft_only", False) == "draft_only"

@pytest.mark.asyncio
async def test_rapidfuzz_matching():
    # Test that triggers matching fuzzy strings work
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

    # Setup database mocks
    mock_msg = MockRecord({
        "direction": "inbound",
        "content_type": "text",
        "content": json.dumps({"text": "do you do house calls?"})
    })
    mock_convo = MockRecord({
        # Draft-only suggestions continue after a human has taken over.
        "ai_mode_active": False,
        "assigned_user_ids": []
    })
    mock_account = MockRecord({
        "settings": json.dumps({
            "ai_enabled": True,
            "ai_reply_mode_default": "draft_only",
            "allow_member_reply_mode_override": True
        })
    })
    # Sane trig: "house calls" -> fuzz ratio against "do you do house calls?" is high if partial/word, 
    # but ratio of "do you do house calls?" and "Do you do house calls?" is 100%.
    mock_pattern = MockRecord({
        "trigger_phrases": ["Do you do house calls?"],
        "answer_markdown": "Yes, we offer house calls."
    })

    # Define DB fetch return values
    async def mock_fetchrow(query, *args):
        if "messages" in query:
            return mock_msg
        if "conversations" in query:
            return mock_convo
        if "accounts" in query:
            return mock_account
        return None

    async def mock_fetch(query, *args):
        if "patterns" in query:
            return [mock_pattern]
        return []

    with patch("main.ScopedDB") as MockScopedDB:
        db_instance = MockScopedDB.return_value
        db_instance.fetchrow = mock_fetchrow
        db_instance.fetch = mock_fetch
        db_instance.execute = AsyncMock()
        draft_id = uuid.uuid4()
        db_instance.fetchval = AsyncMock(return_value=draft_id)
        db_instance.account_id = account_id

        await process_conversation_updated(data, db_pool, redis_client)

        # The draft and answer event are stored atomically.
        db_instance.fetchval.assert_awaited_once()
        args = db_instance.fetchval.call_args[0]
        params = args[1:]
        assert "INSERT INTO ai_reply_drafts" in args[0]
        assert "INSERT INTO ai_answer_events" in args[0]
        assert params[0] == account_id
        assert params[1] == convo_id
        assert params[2] == message_id
        assert params[3] == "Yes, we offer house calls."
        assert params[4] == "pattern"
        assert params[5] == 1.0

        # 2. Redis published the draft to the WebSocket queue
        redis_client.xadd.assert_called_once()
        redis_args = redis_client.xadd.call_args[0]
        assert redis_args[0] == "ai.reply_ready"
        payload = json.loads(redis_args[1]["payload"].decode("utf-8"))
        assert payload["action"] == "drafted"
        assert payload["draft_id"] == str(draft_id)
        assert payload["draft_text"] == "Yes, we offer house calls."

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
        "ai_mode_active": False,
        "assigned_user_ids": []
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

        # AI Answer Event should be logged as flagged_human, stage_matched='none'
        db_instance.execute.assert_awaited_once()
        args = db_instance.execute.call_args.args
        sql = args[0]
        assert "INSERT INTO ai_answer_events" in sql
        assert "'none'" in sql
        assert "'flagged_human'" in sql

        redis_client.xadd.assert_not_awaited()

@pytest.mark.asyncio
async def test_reply_mode_overrides():
    # Test user-specific reply mode overrides
    db_pool = MagicMock()
    redis_client = AsyncMock()

    account_id = uuid.uuid4()
    convo_id = uuid.uuid4()
    message_id = uuid.uuid4()
    user_id = uuid.uuid4()

    data = {
        "account_id": str(account_id),
        "conversation_id": str(convo_id),
        "message_id": str(message_id)
    }

    mock_msg = MockRecord({
        "direction": "inbound",
        "content_type": "text",
        "content": json.dumps({"text": "do you do house calls?"})
    })
    mock_convo = MockRecord({
        "ai_mode_active": True,
        "assigned_user_ids": [user_id] # User assigned
    })
    mock_account = MockRecord({
        "settings": json.dumps({
            "ai_enabled": True,
            "ai_reply_mode_default": "draft_only", # Default is draft_only
            "allow_member_reply_mode_override": True # Allow member override
        })
    })
    mock_user = MockRecord({
        "reply_mode_override": "auto_send" # User wants auto_send!
    })
    mock_pattern = MockRecord({
        "trigger_phrases": ["do you do house calls?"],
        "answer_markdown": "Yes."
    })

    async def mock_fetchrow(query, *args):
        if "messages" in query:
            return mock_msg
        if "conversations" in query:
            return mock_convo
        if "accounts" in query:
            return mock_account
        if "users" in query:
            return mock_user
        return None

    async def mock_fetch(query, *args):
        if "patterns" in query:
            return [mock_pattern]
        return []

    with patch("main.ScopedDB") as MockScopedDB:
        db_instance = MockScopedDB.return_value
        db_instance.fetchrow = mock_fetchrow
        db_instance.fetch = mock_fetch
        db_instance.execute = AsyncMock()
        db_instance.fetchval = AsyncMock(return_value=0) # mock human_msgs_count
        db_instance.account_id = account_id

        # Mock conversation-svc send API call
        with patch("httpx.AsyncClient.post") as mock_post:
            mock_post.return_value = MagicMock(
                status_code=200,
                json=lambda: {"id": str(uuid.uuid4())}
            )

            await process_conversation_updated(data, db_pool, redis_client)

            # Action should resolve to auto_sent due to member override!
            db_instance.execute.assert_awaited_once()
            args = db_instance.execute.call_args.args
            params = args[1:]
            assert params[5] == "auto_sent"
            assert params[6] is not None # reply_message_id populated

            # Redis should be notified of auto_sent
            redis_client.xadd.assert_awaited_once()
            redis_args = redis_client.xadd.call_args.args
            payload = json.loads(redis_args[1]["payload"].decode("utf-8"))
            assert payload["action"] == "auto_sent"

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
        mock_ai_cfg = ("key", "url", "model", "embed")
        mock_summary = {"customer_wants": "Help with code"}

        with patch("main.get_ai_config", AsyncMock(return_value=mock_ai_cfg)), \
             patch("main.complete", AsyncMock(return_value=mock_summary)):

            await process_conversation_closed(data, db_pool, redis_client)

            # DB execute should be called to:
            # 1. Update conversations.ai_mode_active = true
            # 2. Insert/Upsert into conversation_summaries
            assert db_instance.execute.call_count == 2
            
            # Verify the ai_mode_active update
            first_call_args = db_instance.execute.call_args_list[0][0]
            assert "UPDATE conversations" in first_call_args[0]
            
            # Verify the summary upsert
            second_call_args = db_instance.execute.call_args_list[1][0]
            assert "INSERT INTO conversation_summaries" in second_call_args[0]
            assert second_call_args[3] == json.dumps(mock_summary)
            assert second_call_args[4] == 10 # current count
