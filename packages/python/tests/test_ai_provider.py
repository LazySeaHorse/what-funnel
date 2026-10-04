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
    payload = post.await_args.kwargs["json"]
    assert payload["model"] == "reply-model"
    assert payload["response_format"] == {
        "type": "json_schema",
        "json_schema": {
            "name": "Reply",
            "strict": True,
            "schema": Reply.model_json_schema(),
        },
    }


@pytest.mark.asyncio
async def test_complete_rejects_schema_mismatch():
    response = MagicMock()
    response.raise_for_status = MagicMock()
    response.json.return_value = {
        "choices": [{"message": {"content": '[{"answer":"hello"}]'}}]
    }

    with patch("httpx.AsyncClient.post", AsyncMock(return_value=response)):
        with pytest.raises(ProviderError, match="failed schema validation"):
            await ProviderClient("key", "https://provider.test/v1").complete(
                "reply-model", [{"role": "user", "content": "hello"}], Reply
            )


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


@pytest.mark.asyncio
async def test_key_bytes_parsing_edge_cases():
    from whatfunnel_ai.config import _key_bytes, AIConfigurationError

    # 1. Valid 64-char hex key
    hex_key = "ab" * 32
    assert _key_bytes(hex_key) == bytes.fromhex(hex_key)

    # 2. Valid 32-char raw key with non-hex characters
    raw_key = "change-me-32-byte-hex-key-padded"
    assert _key_bytes(raw_key) == raw_key.encode("utf-8")

    # 3. Valid 32-char raw key containing only hex digits
    raw_hex_digits = "0123456789abcdef0123456789abcdef"
    assert _key_bytes(raw_hex_digits) == raw_hex_digits.encode("utf-8")

    # 4. Invalid 64-char hex key (contains 'zz')
    invalid_hex = "ab" * 31 + "zz"
    with pytest.raises(AIConfigurationError, match="invalid 64-character hex encoding"):
        _key_bytes(invalid_hex)

    # 5. Invalid lengths
    for bad_len in [0, 16, 31, 33, 63, 65]:
        with pytest.raises(AIConfigurationError, match="64 hex characters or 32 raw bytes"):
            _key_bytes("a" * bad_len)



# --- retry policy ---------------------------------------------------------------

def _status_error(status, headers=None):
    request = httpx.Request("POST", "https://provider.test/v1/chat/completions")
    response = httpx.Response(status, request=request, headers=headers or {})
    return httpx.HTTPStatusError("err", request=request, response=response)


def _ok():
    response = MagicMock()
    response.raise_for_status = MagicMock()
    response.json.return_value = {"choices": [{"message": {"content": '{"answer":"hi"}'}}]}
    return response


async def _complete(client, side_effect):
    with patch("httpx.AsyncClient.post", AsyncMock(side_effect=side_effect)) as post, \
         patch("whatfunnel_ai.client.asyncio.sleep", AsyncMock()) as sleep:
        try:
            result = await client.complete("m", [{"role": "user", "content": "x"}], Reply)
        except ProviderError as error:
            result = error
    return result, post.await_count, [c.args[0] for c in sleep.await_args_list]


def test_parse_retry_after_seconds_date_and_garbage():
    from datetime import datetime, timezone
    from whatfunnel_ai.client import parse_retry_after

    assert parse_retry_after("7") == 7.0
    assert parse_retry_after(" 2.5 ") == 2.5
    assert parse_retry_after("-3") == 0.0
    assert parse_retry_after(None) is None and parse_retry_after("soon") is None
    now = datetime(2026, 1, 1, 12, 0, 0, tzinfo=timezone.utc)
    assert parse_retry_after("Thu, 01 Jan 2026 12:00:09 GMT", now=now) == 9.0
    assert parse_retry_after("Thu, 01 Jan 2026 11:00:00 GMT", now=now) == 0.0


@pytest.mark.asyncio
async def test_429_honours_retry_after_then_succeeds():
    client = ProviderClient("k", "https://provider.test/v1", max_attempts=2)
    result, calls, sleeps = await _complete(client, [_status_error(429, {"Retry-After": "3"}), _ok()])
    assert result == {"answer": "hi"} and calls == 2
    assert len(sleeps) == 1 and 3.0 <= sleeps[0] <= 3.4


@pytest.mark.asyncio
async def test_retry_after_longer_than_cap_fails_without_waiting():
    client = ProviderClient("k", "https://provider.test/v1", max_attempts=3, max_retry_wait_seconds=10)
    result, calls, sleeps = await _complete(client, [_status_error(429, {"Retry-After": "120"}), _ok()])
    assert isinstance(result, ProviderError) and calls == 1 and sleeps == []


@pytest.mark.asyncio
async def test_5xx_uses_exponential_backoff_with_jitter_within_bounds():
    client = ProviderClient("k", "https://provider.test/v1", max_attempts=4, retry_backoff_seconds=1.0, max_retry_wait_seconds=3.0)
    result, calls, sleeps = await _complete(client, [_status_error(503)] * 4)
    assert isinstance(result, ProviderError) and calls == 4 and len(sleeps) == 3
    # equal jitter: delay in [ceiling/2, ceiling] with ceiling = min(cap, base * 2**retry)
    for sleep, ceiling in zip(sleeps, (1.0, 2.0, 3.0)):
        assert ceiling / 2 <= sleep <= ceiling


@pytest.mark.asyncio
@pytest.mark.parametrize("failure", [httpx.ReadTimeout("slow"), httpx.ConnectError("down"), _status_error(400), _status_error(401), _status_error(404)])
async def test_only_429_and_5xx_are_retried(failure):
    client = ProviderClient("k", "https://provider.test/v1", max_attempts=3)
    result, calls, sleeps = await _complete(client, [failure, _ok()])
    assert isinstance(result, ProviderError) and calls == 1 and sleeps == []
