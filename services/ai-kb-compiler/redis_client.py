import os
import json
import uuid
import logging
from redis.asyncio import Redis

logger = logging.getLogger("ai-kb-compiler")

async def publish_suggestion_created(account_id: uuid.UUID, sugg_id: uuid.UUID, sugg_type: str, proposed_payload: dict):
    redis_url = os.getenv("REDIS_URL")
    if not redis_url:
        logger.info("REDIS_URL not set, skipping suggestion publish.")
        return
    try:
        if not redis_url.startswith("redis://"):
            redis_url = f"redis://{redis_url}"
        r = Redis.from_url(redis_url)
        payload = {
            "account_id": str(account_id),
            "suggestion_id": str(sugg_id),
            "type": sugg_type,
            "payload": proposed_payload
        }
        serialized = json.dumps(payload)
        await r.xadd("automation_suggestion.created", {"payload": serialized.encode("utf-8")})
        logger.info(f"Published automation_suggestion.created to Redis for suggestion {sugg_id}")
        await r.close()
    except Exception as e:
        logger.error(f"Failed to publish automation_suggestion.created: {e}")
