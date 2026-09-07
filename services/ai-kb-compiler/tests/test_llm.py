import pytest
from fastapi import HTTPException
from unittest.mock import AsyncMock, MagicMock

from llm import get_ai_config, provider_client
from whatfunnel_ai import AIConfiguration


@pytest.mark.asyncio
async def test_missing_provider_config_is_explicit():
    db = MagicMock()
    db.account_id = "account-without-provider"
    db.fetchrow = AsyncMock(return_value=None)

    with pytest.raises(HTTPException) as exc_info:
        await get_ai_config(db)

    assert exc_info.value.status_code == 409
    assert "not configured" in exc_info.value.detail


def test_provider_client_uses_configured_transport(monkeypatch):
    from llm import app_config

    monkeypatch.setattr(app_config, "AI_REQUEST_TIMEOUT_SECONDS", 17.0)
    config = AIConfiguration(
        api_key="secret",
        base_url="https://provider.example/v1",
        analysis_model="analysis-model",
        reply_model="reply-model",
        embedding_model="embedding-model",
    )

    client = provider_client(config)

    assert client.base_url == config.base_url
    assert client.timeout_seconds == 17.0
