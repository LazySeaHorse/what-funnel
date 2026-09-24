import asyncio
import json
import logging
import os
import socket
from unittest.mock import AsyncMock, MagicMock, patch

import pytest
from redis.exceptions import ConnectionError, ResponseError

from main import (
    _process_stream_message,
    consume_stream,
    cooldown_scheduler,
    debounce_scheduler,
    get_consumer_name,
)


# ==============================================================================
# Happy Path Tests
# ==============================================================================


def test_get_consumer_name_default_and_override():
    """Verifies hostname fallback, HOSTNAME env var, and config.REDIS_CONSUMER_NAME override."""
    # 1. config.REDIS_CONSUMER_NAME takes precedence over everything
    with patch("main.config.REDIS_CONSUMER_NAME", "custom-consumer-override"):
        assert get_consumer_name() == "custom-consumer-override"

    # 2. When config.REDIS_CONSUMER_NAME is empty, respects HOSTNAME env var
    with patch("main.config.REDIS_CONSUMER_NAME", ""):
        with patch.dict(os.environ, {"HOSTNAME": "k8s-pod-ai-answer-1"}):
            assert get_consumer_name() == "ai-answer-svc-k8s-pod-ai-answer-1"

    # 3. When HOSTNAME is unset, falls back to socket.gethostname()
    with patch("main.config.REDIS_CONSUMER_NAME", ""):
        with patch.dict(os.environ, {}, clear=False):
            os.environ.pop("HOSTNAME", None)
            with patch("socket.gethostname", return_value="worker-node-42"):
                assert get_consumer_name() == "ai-answer-svc-worker-node-42"

    # 4. When socket.gethostname() raises an exception, falls back to 'local'
    with patch("main.config.REDIS_CONSUMER_NAME", ""):
        with patch.dict(os.environ, {}, clear=False):
            os.environ.pop("HOSTNAME", None)
            with patch("socket.gethostname", side_effect=OSError("network lookup failure")):
                assert get_consumer_name() == "ai-answer-svc-local"


@pytest.mark.asyncio
async def test_consume_stream_happy_path_read_and_ack():
    """Mocks xgroup_create, xautoclaim (empty), and xreadgroup (valid message).

    Verifies handler is called with parsed JSON, xack is called with msg_id, and exits via stop_event.
    """
    redis_mock = AsyncMock()
    db_pool_mock = MagicMock()
    stop_event = asyncio.Event()

    redis_mock.xgroup_create.return_value = True
    redis_mock.xautoclaim.return_value = ["0-0", []]

    test_msg_id = "1710000000000-0"
    test_payload = {"account_id": "acc-101", "conversation_id": "convo-202", "text": "hello"}

    async def readgroup_side_effect(*args, **kwargs):
        return [("conversation.updated", [(test_msg_id, {"payload": json.dumps(test_payload)})])]

    redis_mock.xreadgroup.side_effect = readgroup_side_effect

    handler = AsyncMock()

    async def handler_side_effect(data, db, redis):
        # Stop the consumer loop after processing
        stop_event.set()

    handler.side_effect = handler_side_effect

    await consume_stream(
        redis_client=redis_mock,
        db_pool=db_pool_mock,
        stream_name="conversation.updated",
        group_name="ai-answer-svc-group",
        consumer_name="test-consumer-1",
        handler=handler,
        stop_event=stop_event,
    )

    redis_mock.xgroup_create.assert_awaited_once_with(
        "conversation.updated", "ai-answer-svc-group", id="0", mkstream=True
    )
    redis_mock.xautoclaim.assert_awaited_once_with(
        name="conversation.updated",
        groupname="ai-answer-svc-group",
        consumername="test-consumer-1",
        min_idle_time=30000,
        start_id="0-0",
        count=1,
    )
    redis_mock.xreadgroup.assert_awaited_once_with(
        groupname="ai-answer-svc-group",
        consumername="test-consumer-1",
        streams={"conversation.updated": ">"},
        count=1,
        block=1000,
    )
    handler.assert_awaited_once_with(test_payload, db_pool_mock, redis_mock)
    redis_mock.xack.assert_awaited_once_with(
        "conversation.updated", "ai-answer-svc-group", test_msg_id
    )


@pytest.mark.asyncio
async def test_consume_stream_autoclaim_happy_path():
    """Mocks xautoclaim returning a stale message, verifies handler processes it and xack is called."""
    redis_mock = AsyncMock()
    db_pool_mock = MagicMock()
    stop_event = asyncio.Event()

    redis_mock.xgroup_create.return_value = True

    stale_msg_id = "1709999999999-0"
    stale_payload = {"account_id": "acc-stale", "message_id": "msg-recovered"}

    # First call returns the stale message, second returns empty
    redis_mock.xautoclaim.side_effect = [
        ["0-0", [(stale_msg_id, {b"payload": json.dumps(stale_payload).encode("utf-8")})]],
        ["0-0", []],
    ]
    redis_mock.xreadgroup.return_value = []

    handler = AsyncMock()

    async def handler_side_effect(data, db, redis):
        stop_event.set()

    handler.side_effect = handler_side_effect

    await consume_stream(
        redis_client=redis_mock,
        db_pool=db_pool_mock,
        stream_name="conversation.updated",
        group_name="ai-answer-svc-group",
        consumer_name="test-consumer-autoclaim",
        handler=handler,
        stop_event=stop_event,
        min_idle_time=45000,
    )

    redis_mock.xautoclaim.assert_awaited_with(
        name="conversation.updated",
        groupname="ai-answer-svc-group",
        consumername="test-consumer-autoclaim",
        min_idle_time=45000,
        start_id="0-0",
        count=1,
    )
    handler.assert_awaited_once_with(stale_payload, db_pool_mock, redis_mock)
    redis_mock.xack.assert_awaited_once_with(
        "conversation.updated", "ai-answer-svc-group", stale_msg_id
    )


@pytest.mark.asyncio
async def test_consume_stream_deregisters_consumer_on_exit():
    """Verifies that when stop_event is set, xgroup_delconsumer is called with stream, group, consumer."""
    redis_mock = AsyncMock()
    db_pool_mock = MagicMock()
    stop_event = asyncio.Event()
    stop_event.set()  # Already signaled stop

    await consume_stream(
        redis_client=redis_mock,
        db_pool=db_pool_mock,
        stream_name="conversation.closed",
        group_name="ai-answer-svc-group",
        consumer_name="consumer-to-del",
        handler=AsyncMock(),
        stop_event=stop_event,
    )

    redis_mock.xgroup_delconsumer.assert_awaited_once_with(
        "conversation.closed", "ai-answer-svc-group", "consumer-to-del"
    )


@pytest.mark.asyncio
async def test_cooldown_scheduler_graceful_stop():
    """Verifies cooldown_scheduler stops cleanly when stop_event is set."""
    redis_mock = AsyncMock()
    db_pool_mock = MagicMock()
    stop_event = asyncio.Event()

    with patch("main.review_due_cooldown", new_callable=AsyncMock) as mock_review:
        mock_review.return_value = False

        task = asyncio.create_task(
            cooldown_scheduler(db_pool_mock, redis_mock, stop_event=stop_event)
        )
        await asyncio.sleep(0.02)
        assert not task.done()

        stop_event.set()
        await asyncio.wait_for(task, timeout=1.0)
        assert task.done()


@pytest.mark.asyncio
async def test_debounce_scheduler_graceful_stop():
    """Verifies debounce_scheduler stops cleanly when stop_event is set."""
    redis_mock = AsyncMock()
    db_pool_mock = MagicMock()
    stop_event = asyncio.Event()

    with patch("main.pop_due_conversations", new_callable=AsyncMock) as mock_pop:
        mock_pop.return_value = []

        task = asyncio.create_task(
            debounce_scheduler(db_pool_mock, redis_mock, stop_event=stop_event)
        )
        await asyncio.sleep(0.02)
        assert not task.done()

        stop_event.set()
        await asyncio.wait_for(task, timeout=1.0)
        assert task.done()


# ==============================================================================
# Non-Happy Path Tests
# ==============================================================================


@pytest.mark.asyncio
async def test_consume_stream_busygroup_ignored():
    """Verifies xgroup_create raising BUSYGROUP is caught and ignored."""
    redis_mock = AsyncMock()
    db_pool_mock = MagicMock()
    stop_event = asyncio.Event()
    stop_event.set()

    redis_mock.xgroup_create.side_effect = ResponseError(
        "BUSYGROUP Consumer Group name already exists"
    )

    # Should not raise exception
    await consume_stream(
        redis_client=redis_mock,
        db_pool=db_pool_mock,
        stream_name="conversation.updated",
        group_name="ai-answer-svc-group",
        consumer_name="test-consumer",
        handler=AsyncMock(),
        stop_event=stop_event,
    )

    redis_mock.xgroup_create.assert_awaited_once()
    redis_mock.xgroup_delconsumer.assert_awaited_once()


@pytest.mark.asyncio
async def test_consume_stream_nogroup_recreates_group():
    """Verifies NOGROUP error from xreadgroup or xautoclaim triggers xgroup_create to recreate the group."""
    redis_mock = AsyncMock()
    db_pool_mock = MagicMock()
    stop_event = asyncio.Event()

    redis_mock.xgroup_create.return_value = True

    # 1. xautoclaim encounters NOGROUP on 1st call, then returns empty
    redis_mock.xautoclaim.side_effect = [
        ResponseError("NOGROUP No such key 'test_stream' or consumer group 'test_group' in XAUTOCLAIM with GROUP option"),
        ["0-0", []],
    ]

    # 2. xreadgroup encounters NOGROUP on 1st call, then stops loop on 2nd call
    readgroup_calls = 0

    async def mock_readgroup(*args, **kwargs):
        nonlocal readgroup_calls
        readgroup_calls += 1
        if readgroup_calls == 1:
            raise ResponseError("NOGROUP No such key 'test_stream' or consumer group 'test_group' in XREADGROUP with GROUP option")
        stop_event.set()
        return []

    redis_mock.xreadgroup.side_effect = mock_readgroup

    with patch("main.asyncio.sleep", new_callable=AsyncMock) as mock_sleep:
        await consume_stream(
            redis_client=redis_mock,
            db_pool=db_pool_mock,
            stream_name="test_stream",
            group_name="test_group",
            consumer_name="test-consumer",
            handler=AsyncMock(),
            stop_event=stop_event,
        )

    # 1 initial create + 1 after xautoclaim NOGROUP + 1 after xreadgroup NOGROUP = 3 calls
    assert redis_mock.xgroup_create.await_count == 3
    assert mock_sleep.await_count >= 1


@pytest.mark.asyncio
async def test_consume_stream_malformed_json_payload(caplog):
    """Verifies bad JSON payload does not crash the consumer loop and error is logged."""
    redis_mock = AsyncMock()
    db_pool_mock = MagicMock()
    stop_event = asyncio.Event()

    redis_mock.xgroup_create.return_value = True
    redis_mock.xautoclaim.return_value = ["0-0", []]

    bad_msg_id = "1720000000000-0"

    async def readgroup_side_effect(*args, **kwargs):
        stop_event.set()
        return [("conversation.updated", [(bad_msg_id, {"payload": "{bad_json: True, invalid}"})])]

    redis_mock.xreadgroup.side_effect = readgroup_side_effect
    handler = AsyncMock()

    with caplog.at_level(logging.ERROR):
        await consume_stream(
            redis_client=redis_mock,
            db_pool=db_pool_mock,
            stream_name="conversation.updated",
            group_name="ai-answer-svc-group",
            consumer_name="test-consumer",
            handler=handler,
            stop_event=stop_event,
        )

    handler.assert_not_awaited()
    # xack is not called on unhandled deserialization error
    redis_mock.xack.assert_not_awaited()
    assert f"Error processing message {bad_msg_id}" in caplog.text


@pytest.mark.asyncio
async def test_consume_stream_missing_payload_key(caplog):
    """Verifies message missing 'payload' key is safely acked and does not crash."""
    redis_mock = AsyncMock()
    db_pool_mock = MagicMock()
    stop_event = asyncio.Event()

    redis_mock.xgroup_create.return_value = True
    redis_mock.xautoclaim.return_value = ["0-0", []]

    missing_key_msg_id = "1730000000000-0"

    async def readgroup_side_effect(*args, **kwargs):
        stop_event.set()
        return [("conversation.updated", [(missing_key_msg_id, {"unexpected_field": "data"})])]

    redis_mock.xreadgroup.side_effect = readgroup_side_effect
    handler = AsyncMock()

    with caplog.at_level(logging.WARNING):
        await consume_stream(
            redis_client=redis_mock,
            db_pool=db_pool_mock,
            stream_name="conversation.updated",
            group_name="ai-answer-svc-group",
            consumer_name="test-consumer",
            handler=handler,
            stop_event=stop_event,
        )

    handler.assert_not_awaited()
    redis_mock.xack.assert_awaited_once_with(
        "conversation.updated", "ai-answer-svc-group", missing_key_msg_id
    )
    assert f"Message {missing_key_msg_id} in stream conversation.updated has no payload field, acking." in caplog.text


@pytest.mark.asyncio
async def test_consume_stream_handler_exception(caplog):
    """Verifies that if handler raises an exception, the error is logged and loop continues without crashing."""
    redis_mock = AsyncMock()
    db_pool_mock = MagicMock()
    stop_event = asyncio.Event()

    redis_mock.xgroup_create.return_value = True
    redis_mock.xautoclaim.return_value = ["0-0", []]

    msg_id = "1740000000000-0"
    payload = {"account_id": "acc-err", "conversation_id": "convo-err"}

    async def readgroup_side_effect(*args, **kwargs):
        stop_event.set()
        return [("conversation.updated", [(msg_id, {"payload": json.dumps(payload)})])]

    redis_mock.xreadgroup.side_effect = readgroup_side_effect
    handler = AsyncMock(side_effect=RuntimeError("Handler execution failed unexpectedly"))

    with caplog.at_level(logging.ERROR):
        await consume_stream(
            redis_client=redis_mock,
            db_pool=db_pool_mock,
            stream_name="conversation.updated",
            group_name="ai-answer-svc-group",
            consumer_name="test-consumer",
            handler=handler,
            stop_event=stop_event,
        )

    handler.assert_awaited_once_with(payload, db_pool_mock, redis_mock)
    # Failed message is not acked so it can be retried / reclaimed later
    redis_mock.xack.assert_not_awaited()
    assert f"Error processing message {msg_id}" in caplog.text


@pytest.mark.asyncio
async def test_consume_stream_autoclaim_error_resilience():
    """Verifies that if xautoclaim raises an error (e.g. unknown command), consumer falls back to xreadgroup."""
    redis_mock = AsyncMock()
    db_pool_mock = MagicMock()
    stop_event = asyncio.Event()

    redis_mock.xgroup_create.return_value = True
    redis_mock.xautoclaim.side_effect = ResponseError("ERR unknown command 'XAUTOCLAIM'")

    msg_id = "1750000000000-0"
    payload = {"account_id": "acc-fallback", "conversation_id": "convo-fallback"}

    async def readgroup_side_effect(*args, **kwargs):
        stop_event.set()
        return [("conversation.updated", [(msg_id, {"payload": json.dumps(payload)})])]

    redis_mock.xreadgroup.side_effect = readgroup_side_effect
    handler = AsyncMock()

    await consume_stream(
        redis_client=redis_mock,
        db_pool=db_pool_mock,
        stream_name="conversation.updated",
        group_name="ai-answer-svc-group",
        consumer_name="test-consumer",
        handler=handler,
        stop_event=stop_event,
    )

    redis_mock.xautoclaim.assert_awaited_once()
    redis_mock.xreadgroup.assert_awaited_once()
    handler.assert_awaited_once_with(payload, db_pool_mock, redis_mock)
    redis_mock.xack.assert_awaited_once_with("conversation.updated", "ai-answer-svc-group", msg_id)


@pytest.mark.asyncio
async def test_consume_stream_delconsumer_error_resilience(caplog):
    """Verifies that if xgroup_delconsumer raises an exception during shutdown, it does not bubble up."""
    redis_mock = AsyncMock()
    db_pool_mock = MagicMock()
    stop_event = asyncio.Event()
    stop_event.set()

    redis_mock.xgroup_delconsumer.side_effect = ConnectionError("Redis server went away")

    with caplog.at_level(logging.DEBUG):
        await consume_stream(
            redis_client=redis_mock,
            db_pool=db_pool_mock,
            stream_name="conversation.updated",
            group_name="ai-answer-svc-group",
            consumer_name="consumer-del-err",
            handler=AsyncMock(),
            stop_event=stop_event,
        )

    redis_mock.xgroup_delconsumer.assert_awaited_once_with(
        "conversation.updated", "ai-answer-svc-group", "consumer-del-err"
    )
    assert "Could not deregister consumer consumer-del-err from conversation.updated" in caplog.text


# ==============================================================================
# Additional Edge Case & Unit Tests
# ==============================================================================


@pytest.mark.asyncio
async def test_consume_stream_group_create_non_busygroup_error_raises():
    """Verifies that an unexpected ResponseError from xgroup_create is re-raised."""
    redis_mock = AsyncMock()
    db_pool_mock = MagicMock()
    stop_event = asyncio.Event()

    redis_mock.xgroup_create.side_effect = ResponseError("WRONGTYPE Operation against a key holding the wrong kind of value")

    with pytest.raises(ResponseError, match="WRONGTYPE"):
        await consume_stream(
            redis_client=redis_mock,
            db_pool=db_pool_mock,
            stream_name="conversation.updated",
            group_name="ai-answer-svc-group",
            consumer_name="consumer-err",
            handler=AsyncMock(),
            stop_event=stop_event,
        )


@pytest.mark.asyncio
async def test_process_stream_message_bytes_and_string_payload():
    """Verifies _process_stream_message properly decodes both bytes and string payload values."""
    redis_mock = AsyncMock()
    db_pool_mock = MagicMock()
    handler = AsyncMock()

    # Case 1: b'payload' as bytes
    payload_dict_1 = {"event": "bubble_1"}
    await _process_stream_message(
        "msg-1",
        {b"payload": json.dumps(payload_dict_1).encode("utf-8")},
        "stream-1",
        "group-1",
        handler,
        db_pool_mock,
        redis_mock,
    )
    handler.assert_awaited_once_with(payload_dict_1, db_pool_mock, redis_mock)
    redis_mock.xack.assert_awaited_once_with("stream-1", "group-1", "msg-1")

    handler.reset_mock()
    redis_mock.reset_mock()

    # Case 2: 'payload' as str
    payload_dict_2 = {"event": "bubble_2"}
    await _process_stream_message(
        "msg-2",
        {"payload": json.dumps(payload_dict_2)},
        "stream-1",
        "group-1",
        handler,
        db_pool_mock,
        redis_mock,
    )
    handler.assert_awaited_once_with(payload_dict_2, db_pool_mock, redis_mock)
    redis_mock.xack.assert_awaited_once_with("stream-1", "group-1", "msg-2")
