import os
import secrets
import uuid
from typing import Optional
from fastapi import Header, Depends, HTTPException, Request, status
from db import ScopedDB


PLACEHOLDER_TOKEN = "change-me-in-production-at-least-32-chars"


def get_internal_token() -> str:
    """Dedicated inter-service token. There is no SESSION_SECRET fallback."""
    return os.getenv("INTERNAL_SERVICE_TOKEN", "").strip()


def insecure_internal_auth_allowed() -> bool:
    return os.getenv("ALLOW_INSECURE_INTERNAL_AUTH", "").lower() in ("true", "1", "yes")


async def verify_internal_auth(
    x_internal_token: Optional[str] = Header(None, alias="X-Internal-Token")
) -> None:
    expected_token = get_internal_token()
    if not expected_token or expected_token == PLACEHOLDER_TOKEN:
        # Fail closed. The dev opt-in is ignored in production.
        if insecure_internal_auth_allowed() and os.getenv("APP_ENV") != "production":
            return
        raise HTTPException(
            status_code=status.HTTP_401_UNAUTHORIZED,
            detail="Internal service token is not configured."
        )
    if not x_internal_token or not secrets.compare_digest(x_internal_token, expected_token):
        raise HTTPException(
            status_code=status.HTTP_401_UNAUTHORIZED,
            detail="Invalid or missing X-Internal-Token header."
        )


# Dependency to retrieve a tenant-scoped database client.
async def get_db(
    request: Request,
    _: None = Depends(verify_internal_auth),
    x_account_id: str = Header(..., alias="X-Account-ID")
) -> ScopedDB:
    try:
        account_uuid = uuid.UUID(x_account_id)
    except ValueError:
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail="Invalid X-Account-ID header format. Must be a valid UUID."
        )
    return ScopedDB(request.app.state.db, account_uuid)
