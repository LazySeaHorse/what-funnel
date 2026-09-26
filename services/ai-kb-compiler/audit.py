import json
import uuid
from typing import Optional
from db import ScopedDB


async def write_audit_log(
    db: ScopedDB,
    actor_user_id: Optional[uuid.UUID],
    action: str,
    target_type: str,
    target_id: Optional[uuid.UUID] = None,
    metadata: Optional[dict] = None
) -> None:
    """Record an audit log entry for the account, verifying actor existence if provided."""
    if metadata is None:
        metadata = {}

    if actor_user_id:
        user_exists = await db.fetchval(
            "SELECT 1 FROM users WHERE id = $1 AND account_id = $2",
            actor_user_id, db.account_id
        )
        if not user_exists:
            actor_user_id = None

    await db.execute(
        """
        INSERT INTO audit_logs (account_id, actor_user_id, action, target_type, target_id, metadata)
        VALUES ($1, $2, $3, $4, $5, $6)
        """,
        db.account_id,
        actor_user_id,
        action,
        target_type,
        target_id,
        json.dumps(metadata)
    )
