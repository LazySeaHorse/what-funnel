import os
import secrets
import uuid
from typing import Optional
from fastapi import Header, Depends, HTTPException, Request, status
from db import ScopedDB


def get_internal_token() -> str:
    return os.getenv("INTERNAL_SERVICE_TOKEN") or os.getenv("SESSION_SECRET") or ""


async def verify_internal_auth(
    x_internal_token: Optional[str] = Header(None, alias="X-Internal-Token")
) -> None:
    expected_token = get_internal_token()
    is_prod = os.getenv("APP_ENV") == "production"
    if is_prod and (not expected_token or expected_token == "change-me-in-production-at-least-32-chars"):
        raise HTTPException(
            status_code=status.HTTP_401_UNAUTHORIZED,
            detail="Insecure internal service token configuration in production."
        )
    if expected_token:
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
