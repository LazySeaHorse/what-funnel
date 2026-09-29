import pytest
from fastapi import HTTPException

from auth import verify_internal_auth


@pytest.mark.asyncio
async def test_missing_configured_token_fails_closed(monkeypatch):
    monkeypatch.delenv("INTERNAL_SERVICE_TOKEN", raising=False)
    with pytest.raises(HTTPException) as exc:
        await verify_internal_auth(None)
    assert exc.value.status_code == 401


@pytest.mark.asyncio
async def test_session_secret_is_not_a_fallback(monkeypatch):
    monkeypatch.delenv("INTERNAL_SERVICE_TOKEN", raising=False)
    monkeypatch.setenv("SESSION_SECRET", "session-secret-32-characters-long!!")
    with pytest.raises(HTTPException):
        await verify_internal_auth("session-secret-32-characters-long!!")


@pytest.mark.asyncio
async def test_explicit_insecure_opt_in_allows_dev(monkeypatch):
    monkeypatch.delenv("INTERNAL_SERVICE_TOKEN", raising=False)
    monkeypatch.setenv("ALLOW_INSECURE_INTERNAL_AUTH", "true")
    monkeypatch.delenv("APP_ENV", raising=False)
    await verify_internal_auth(None)


@pytest.mark.asyncio
async def test_insecure_opt_in_ignored_in_production(monkeypatch):
    monkeypatch.delenv("INTERNAL_SERVICE_TOKEN", raising=False)
    monkeypatch.setenv("ALLOW_INSECURE_INTERNAL_AUTH", "true")
    monkeypatch.setenv("APP_ENV", "production")
    with pytest.raises(HTTPException):
        await verify_internal_auth(None)


@pytest.mark.asyncio
async def test_token_must_match(monkeypatch):
    monkeypatch.setenv("INTERNAL_SERVICE_TOKEN", "right-token-right-token-right-token")
    await verify_internal_auth("right-token-right-token-right-token")
    with pytest.raises(HTTPException):
        await verify_internal_auth("wrong")
    with pytest.raises(HTTPException):
        await verify_internal_auth(None)
