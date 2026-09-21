import pytest
import uuid
import time
from unittest.mock import AsyncMock, MagicMock, patch

from debounce import (
    calculate_debounce_delay,
    record_inbound_message,
    cancel_debounce,
    pop_due_conversations,
    requeue_in_flight,
    DEBOUNCE_QUEUE_KEY,
    DEBOUNCE_META_PREFIX,
    DEBOUNCE_SEEN_PREFIX,
)
from config import config


def test_calculate_debounce_delay():
    assert calculate_debounce_delay(1) == 10.0
    assert calculate_debounce_delay(2) == 5.0
    assert calculate_debounce_delay(3) == 10.0
    assert calculate_debounce_delay(4) == 10.0


@pytest.mark.asyncio
async def test_record_inbound_message_first_bubble():
    redis_mock = AsyncMock()
    # First message: sadd returns 1 (new message)
    redis_mock.sadd.return_value = 1
    redis_mock.hincrby.return_value = 1

    account_id = uuid.uuid4()
    convo_id = uuid.uuid4()
    msg_id = uuid.uuid4()

    scheduled, delay = await record_inbound_message(
        redis_mock, account_id, convo_id, msg_id, is_text=True
    )

    assert scheduled is True
    assert delay == 10.0

    redis_mock.sadd.assert_awaited_once_with(f"{DEBOUNCE_SEEN_PREFIX}{convo_id}", str(msg_id))
    redis_mock.hincrby.assert_awaited_once_with(f"{DEBOUNCE_META_PREFIX}{convo_id}", "bubble_count", 1)
    redis_mock.hset.assert_awaited_once_with(
        f"{DEBOUNCE_META_PREFIX}{convo_id}",
        mapping={"account_id": str(account_id), "latest_message_id": str(msg_id)},
    )
    redis_mock.zadd.assert_awaited_once()
    zadd_call = redis_mock.zadd.call_args
    assert zadd_call.args[0] == DEBOUNCE_QUEUE_KEY
    assert str(convo_id) in zadd_call.args[1]


@pytest.mark.asyncio
async def test_record_inbound_message_subsequent_bubble():
    redis_mock = AsyncMock()
    redis_mock.sadd.return_value = 1
    # 2nd bubble
    redis_mock.hincrby.return_value = 2

    account_id = uuid.uuid4()
    convo_id = uuid.uuid4()
    msg_id = uuid.uuid4()

    scheduled, delay = await record_inbound_message(
        redis_mock, account_id, convo_id, msg_id, is_text=True
    )

    assert scheduled is True
    assert delay == 5.0


@pytest.mark.asyncio
async def test_record_inbound_message_burst_bubble():
    redis_mock = AsyncMock()
    redis_mock.sadd.return_value = 1
    # 3rd bubble jumps back up to 10s
    redis_mock.hincrby.return_value = 3

    account_id = uuid.uuid4()
    convo_id = uuid.uuid4()
    msg_id = uuid.uuid4()

    scheduled, delay = await record_inbound_message(
        redis_mock, account_id, convo_id, msg_id, is_text=True
    )

    assert scheduled is True
    assert delay == 10.0


@pytest.mark.asyncio
async def test_record_inbound_message_duplicate_webhook_ignored():
    redis_mock = AsyncMock()
    # Duplicate delivery: sadd returns 0
    redis_mock.sadd.return_value = 0

    account_id = uuid.uuid4()
    convo_id = uuid.uuid4()
    msg_id = uuid.uuid4()

    scheduled, delay = await record_inbound_message(
        redis_mock, account_id, convo_id, msg_id, is_text=True
    )

    assert scheduled is False
    assert delay == 0.0
    redis_mock.hincrby.assert_not_awaited()
    redis_mock.zadd.assert_not_awaited()


@pytest.mark.asyncio
async def test_record_inbound_message_non_text():
    redis_mock = AsyncMock()
    redis_mock.sadd.return_value = 1
    redis_mock.hincrby.return_value = 1

    account_id = uuid.uuid4()
    convo_id = uuid.uuid4()
    msg_id = uuid.uuid4()

    scheduled, delay = await record_inbound_message(
        redis_mock, account_id, convo_id, msg_id, is_text=False
    )

    assert scheduled is True
    redis_mock.hset.assert_awaited_once_with(
        f"{DEBOUNCE_META_PREFIX}{convo_id}",
        mapping={
            "account_id": str(account_id),
            "latest_message_id": str(msg_id),
            "has_non_text": "1",
        },
    )


@pytest.mark.asyncio
async def test_cancel_debounce():
    redis_mock = AsyncMock()
    convo_id = uuid.uuid4()

    await cancel_debounce(redis_mock, convo_id)

    redis_mock.zrem.assert_awaited_once_with(DEBOUNCE_QUEUE_KEY, str(convo_id))
    redis_mock.delete.assert_awaited_once_with(f"{DEBOUNCE_META_PREFIX}{convo_id}")


@pytest.mark.asyncio
async def test_pop_due_conversations():
    redis_mock = AsyncMock()
    convo_id = uuid.uuid4()
    account_id = uuid.uuid4()
    msg_id = uuid.uuid4()

    redis_mock.eval.return_value = [str(convo_id).encode("utf-8")]
    redis_mock.hgetall.return_value = {
        b"account_id": str(account_id).encode("utf-8"),
        b"latest_message_id": str(msg_id).encode("utf-8"),
        b"bubble_count": b"2",
        b"has_non_text": b"1",
    }

    due = await pop_due_conversations(redis_mock)

    assert len(due) == 1
    assert due[0]["conversation_id"] == convo_id
    assert due[0]["account_id"] == account_id
    assert due[0]["latest_message_id"] == msg_id
    assert due[0]["has_non_text"] is True
    assert due[0]["bubble_count"] == 2


@pytest.mark.asyncio
async def test_requeue_in_flight():
    redis_mock = AsyncMock()
    convo_id = uuid.uuid4()
    account_id = uuid.uuid4()
    msg_id = uuid.uuid4()

    await requeue_in_flight(redis_mock, convo_id, account_id, msg_id, has_non_text=False, delay=2.5)

    redis_mock.hset.assert_awaited_once()
    redis_mock.zadd.assert_awaited_once()
    zadd_call = redis_mock.zadd.call_args
    assert str(convo_id) in zadd_call.args[1]
