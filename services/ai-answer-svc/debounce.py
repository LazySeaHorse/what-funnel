import logging
import time
import uuid
from typing import Optional

from config import config

logger = logging.getLogger("ai-answer-svc.debounce")

DEBOUNCE_QUEUE_KEY = "ai_debounce:queue"
DEBOUNCE_META_PREFIX = "ai_debounce:meta:"
DEBOUNCE_SEEN_PREFIX = "ai_debounce:seen:"

# Atomically dedupe the message and update bubble metadata + the debounce deadline.
# KEYS: seen, meta, queue.  ARGV: message_id, account_id, is_text(0/1), now,
#       conversation_id, first_delay, subsequent_delay, burst_delay
# Returns {0} for a duplicate delivery, else {1, bubble_count, delay}.
RECORD_INBOUND_LUA = """
local added = redis.call('SADD', KEYS[1], ARGV[1])
redis.call('EXPIRE', KEYS[1], 3600)
if added == 0 then
    return {0}
end
local count = redis.call('HINCRBY', KEYS[2], 'bubble_count', 1)
redis.call('HSET', KEYS[2], 'account_id', ARGV[2], 'latest_message_id', ARGV[1])
if ARGV[3] == '0' then
    redis.call('HSET', KEYS[2], 'has_non_text', '1')
end
redis.call('EXPIRE', KEYS[2], 3600)
local delay = ARGV[6]
if count == 2 then
    delay = ARGV[7]
elseif count > 2 then
    delay = ARGV[8]
end
redis.call('ZADD', KEYS[3], tonumber(ARGV[4]) + tonumber(delay), ARGV[5])
return {1, count, delay}
"""

# Atomically pop due conversations together with their metadata, so a message that
# arrives between the pop and the metadata read can never be lost.
# KEYS: queue.  ARGV: now, limit, meta_prefix
# Returns a list of {conversation_id, {field, value, ...}}.
POP_DUE_LUA = """
local ready = redis.call('ZRANGEBYSCORE', KEYS[1], '-inf', ARGV[1], 'LIMIT', 0, tonumber(ARGV[2]))
local result = {}
for _, id in ipairs(ready) do
    local meta_key = ARGV[3] .. id
    redis.call('ZREM', KEYS[1], id)
    result[#result + 1] = {id, redis.call('HGETALL', meta_key)}
    redis.call('DEL', meta_key)
end
return result
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

    result = await redis_client.eval(
        RECORD_INBOUND_LUA,
        3,
        f"{DEBOUNCE_SEEN_PREFIX}{convo_str}",
        f"{DEBOUNCE_META_PREFIX}{convo_str}",
        DEBOUNCE_QUEUE_KEY,
        msg_str,
        str(account_id),
        "1" if is_text else "0",
        repr(time.time()),
        convo_str,
        repr(float(config.AI_DEBOUNCE_FIRST_SECONDS)),
        repr(float(config.AI_DEBOUNCE_SUBSEQUENT_SECONDS)),
        repr(float(config.AI_DEBOUNCE_BURST_SECONDS)),
    )
    if not result or int(result[0]) == 0:
        logger.debug("Duplicate message_id %s received for conversation %s, skipping debounce reset", msg_str, convo_str)
        return False, 0.0

    bubble_count = int(result[1])
    raw_delay = result[2]
    delay = float(raw_delay.decode("utf-8") if isinstance(raw_delay, bytes) else raw_delay)
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


def _to_str(value) -> str:
    return value.decode("utf-8") if isinstance(value, bytes) else str(value)


async def pop_due_conversations(redis_client, batch_size: int = 50) -> list[dict]:
    """Atomically pop conversations whose debounce deadline has expired, with their metadata."""
    now = time.time()
    try:
        popped = await redis_client.eval(
            POP_DUE_LUA, 1, DEBOUNCE_QUEUE_KEY, repr(now), batch_size, DEBOUNCE_META_PREFIX
        )
    except Exception as e:
        logger.error("Error executing pop_due_conversations lua script: %s", e)
        return []

    due_items = []
    for raw_id, flat_meta in popped or []:
        convo_id = _to_str(raw_id)
        flat = [_to_str(x) for x in flat_meta]
        meta = dict(zip(flat[0::2], flat[1::2]))

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
                "attempts": int(meta.get("attempts", "0")),
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
    attempts: int | None = None,
) -> None:
    """Re-enqueue a conversation whose generation was blocked or failed.

    Never overwrites metadata from a newer inbound message that arrived in the
    meantime, and never shortens a later debounce deadline.
    """
    convo_str = str(conversation_id)
    meta_key = f"{DEBOUNCE_META_PREFIX}{convo_str}"
    await redis_client.hsetnx(meta_key, "account_id", str(account_id))
    await redis_client.hsetnx(meta_key, "latest_message_id", str(latest_message_id))
    if has_non_text:
        await redis_client.hsetnx(meta_key, "has_non_text", "1")
    if attempts is not None:
        await redis_client.hset(meta_key, "attempts", str(attempts))
    await redis_client.expire(meta_key, 3600)
    deadline = time.time() + delay
    await redis_client.zadd(DEBOUNCE_QUEUE_KEY, {convo_str: deadline}, gt=True)
    logger.info("Re-queued conversation %s for retry in %.1fs", convo_str, delay)
