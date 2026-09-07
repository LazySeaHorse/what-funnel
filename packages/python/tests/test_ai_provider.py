from unittest.mock import AsyncMock, MagicMock, patch

import httpx
import pytest
from cryptography.hazmat.primitives.ciphers.aead import AESGCM
from pydantic import BaseModel

from whatfunnel_ai import ProviderClient, ProviderError, load_ai_configuration


class Reply(BaseModel):
    answer: str


def encrypted_api_key(key: bytes, plaintext: str) -> str:
    nonce = bytes(range(12))
    return (nonce + AESGCM(key).encrypt(nonce, plaintext.encode(), None)).hex()


@pytest.mark.asyncio
async def test_load_ai_configuration_returns_typed_models():
    db = MagicMock()
    db.account_id = "account-id"
    db.fetchrow = AsyncMock(
        return_value={
            "base_url": "https://provider.test/v1/",
            "encrypted_api_key": encrypted_api_key(b"k" * 32, "secret"),
            "analysis_model": "analysis",
            "reply_model": "reply",
            "embedding_model": "embedding",
        }
    )

    config = await load_ai_configuration(db, "k" * 32)

    assert config.api_key == "secret"
    assert config.base_url == "https://provider.test/v1"
    assert config.analysis_model == "analysis"
    assert config.reply_model == "reply"


@pytest.mark.asyncio
async def test_complete_sends_selected_model_and_validates_schema():
    response = MagicMock()
    response.raise_for_status = MagicMock()
    response.json.return_value = {
        "choices": [{"message": {"content": '{"answer":"hello"}'}}]
    }

    with patch("httpx.AsyncClient.post", AsyncMock(return_value=response)) as post:
        result = await ProviderClient("key", "https://provider.test/v1").complete(
            "reply-model", [{"role": "user", "content": "hello"}], Reply
        )

    assert result == {"answer": "hello"}
    assert post.await_args.kwargs["json"]["model"] == "reply-model"


@pytest.mark.asyncio
async def test_complete_does_not_retry_non_transient_failure():
    request = httpx.Request("POST", "https://provider.test/v1/chat/completions")
    response = httpx.Response(400, request=request)
    error = httpx.HTTPStatusError("bad request", request=request, response=response)

    with patch("httpx.AsyncClient.post", AsyncMock(side_effect=error)) as post:
        with pytest.raises(ProviderError, match="rejected model"):
            await ProviderClient("key", "https://provider.test/v1").complete(
                "reply-model", [{"role": "user", "content": "hello"}], Reply
            )

    assert post.await_count == 1


@pytest.mark.asyncio
async def test_embed_rejects_wrong_dimension():
    response = MagicMock()
    response.raise_for_status = MagicMock()
    response.json.return_value = {"data": [{"embedding": [0.1, 0.2]}]}

    with patch("httpx.AsyncClient.post", AsyncMock(return_value=response)):
        with pytest.raises(ProviderError, match="expected 1536"):
            await ProviderClient("key", "https://provider.test/v1").embed(
                "embedding-model", "hello"
            )
