import logging
import time
import uuid
from typing import Optional

from config import config

logger = logging.getLogger("ai-answer-svc.debounce")

DEBOUNCE_QUEUE_KEY = "ai_debounce:queue"
DEBOUNCE_META_PREFIX = "ai_debounce:meta:"
DEBOUNCE_SEEN_PREFIX = "ai_debounce:seen:"

POP_DUE_LUA = """
local queue_key = KEYS[1]
local now = tonumber(ARGV[1])
local limit = tonumber(ARGV[2])
local ready = redis.call('ZRANGEBYSCORE', queue_key, '-inf', now, 'LIMIT', 0, limit)
if #ready > 0 then
    redis.call('ZREM', queue_key, unpack(ready))
end
return ready
"""


def calculate_debounce_delay(bubble_count: int) -> float:
    """Calculate the sliding debounce window based on bubble count.
    - 1st bubble: AI_DEBOUNCE_FIRST_SECONDS (default 10s)
    - 2nd bubble: AI_DEBOUNCE_SUBSEQUENT_SECONDS (default 5s)
    - 3rd+ bubbles: AI_DEBOUNCE_BURST_SECONDS (default 10s)
    """
    if bubble_count <= 1:
        return config.AI_DEBOUNCE_FIRST_SECONDS
    elif bubble_count == 2:
        return config.AI_DEBOUNCE_SUBSEQUENT_SECONDS
    else:
        return config.AI_DEBOUNCE_BURST_SECONDS


async def record_inbound_message(
    redis_client,
    account_id: uuid.UUID,
    conversation_id: uuid.UUID,
    message_id: uuid.UUID,
    is_text: bool = True,
) -> tuple[bool, float]:
    """Record an incoming message into the debounce manager.
    Returns (scheduled, delay_seconds).
    If message is a duplicate webhook delivery, scheduled=False.
    """
    convo_str = str(conversation_id)
    msg_str = str(message_id)

    # 1. De-duplicate message_id
    seen_key = f"{DEBOUNCE_SEEN_PREFIX}{convo_str}"
    added = await redis_client.sadd(seen_key, msg_str)
    await redis_client.expire(seen_key, 3600)
    if not added:
        logger.debug("Duplicate message_id %s received for conversation %s, skipping debounce reset", msg_str, convo_str)
        return False, 0.0

    # 2. Update metadata
    meta_key = f"{DEBOUNCE_META_PREFIX}{convo_str}"
    bubble_count = await redis_client.hincrby(meta_key, "bubble_count", 1)
    mapping = {
        "account_id": str(account_id),
        "latest_message_id": msg_str,
    }
    if not is_text:
        mapping["has_non_text"] = "1"
    await redis_client.hset(meta_key, mapping=mapping)
    await redis_client.expire(meta_key, 3600)

    # 3. Schedule next trigger time in ZSET
    delay = calculate_debounce_delay(bubble_count)
    deadline = time.time() + delay
    await redis_client.zadd(DEBOUNCE_QUEUE_KEY, {convo_str: deadline})

    logger.info(
        "Debounce updated for convo %s: bubble_count=%d, delay=%.1fs, is_text=%s",
        convo_str, bubble_count, delay, is_text
    )
    return True, delay


async def cancel_debounce(redis_client, conversation_id: uuid.UUID) -> None:
    """Cancel any active debounce timer for a conversation (e.g. human takeover or chat closed)."""
    convo_str = str(conversation_id)
    await redis_client.zrem(DEBOUNCE_QUEUE_KEY, convo_str)
    await redis_client.delete(f"{DEBOUNCE_META_PREFIX}{convo_str}")
    logger.debug("Cancelled debounce for conversation %s", convo_str)


async def pop_due_conversations(redis_client, batch_size: int = 50) -> list[dict]:
    """Atomically retrieve conversations whose debounce deadline has expired."""
    now = time.time()
    try:
        ready_ids = await redis_client.eval(POP_DUE_LUA, 1, DEBOUNCE_QUEUE_KEY, now, batch_size)
    except Exception as e:
        logger.error("Error executing pop_due_conversations lua script: %s", e)
        return []

    if not ready_ids:
        return []

    due_items = []
    for raw_id in ready_ids:
        convo_id = raw_id.decode("utf-8") if isinstance(raw_id, bytes) else str(raw_id)
        meta_key = f"{DEBOUNCE_META_PREFIX}{convo_id}"
        meta_raw = await redis_client.hgetall(meta_key)
        await redis_client.delete(meta_key)

        meta: dict[str, str] = {}
        for k, v in meta_raw.items():
            k_str = k.decode("utf-8") if isinstance(k, bytes) else str(k)
            v_str = v.decode("utf-8") if isinstance(v, bytes) else str(v)
            meta[k_str] = v_str

        account_id_str = meta.get("account_id")
        latest_msg_str = meta.get("latest_message_id")
        if not account_id_str or not latest_msg_str:
            logger.warning("Missing metadata for debounced conversation %s", convo_id)
            continue

        try:
            due_items.append({
                "conversation_id": uuid.UUID(convo_id),
                "account_id": uuid.UUID(account_id_str),
                "latest_message_id": uuid.UUID(latest_msg_str),
                "has_non_text": meta.get("has_non_text") == "1",
                "bubble_count": int(meta.get("bubble_count", "1")),
            })
        except Exception as e:
            logger.error("Failed to parse debounced conversation payload for %s: %s", convo_id, e)

    return due_items


async def requeue_in_flight(
    redis_client,
    conversation_id: uuid.UUID,
    account_id: uuid.UUID,
    latest_message_id: uuid.UUID,
    has_non_text: bool = False,
    delay: float = 2.0,
) -> None:
    """Re-enqueue a conversation whose generation was blocked due to an in-flight LLM run."""
    convo_str = str(conversation_id)
    meta_key = f"{DEBOUNCE_META_PREFIX}{convo_str}"
    mapping = {
        "account_id": str(account_id),
        "latest_message_id": str(latest_message_id),
    }
    if has_non_text:
        mapping["has_non_text"] = "1"
    await redis_client.hset(meta_key, mapping=mapping)
    await redis_client.expire(meta_key, 3600)
    deadline = time.time() + delay
    await redis_client.zadd(DEBOUNCE_QUEUE_KEY, {convo_str: deadline})
    logger.info("Re-queued in-flight conversation %s for retry in %.1fs", convo_str, delay)
