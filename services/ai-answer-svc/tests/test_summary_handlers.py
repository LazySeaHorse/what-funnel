import uuid
from unittest.mock import AsyncMock, patch

import pytest

import main


@pytest.mark.asyncio
async def test_closed_handler_cancels_debounce_and_uses_closed_trigger():
    a, c = uuid.uuid4(), uuid.uuid4()
    with patch("main.cancel_debounce", AsyncMock()) as cancel, patch("main.generate_summary", AsyncMock()) as gen:
        await main.process_conversation_closed({"account_id": str(a), "conversation_id": str(c)}, "pool", "redis")
    cancel.assert_awaited_once_with("redis", c)
    gen.assert_awaited_once_with("pool", "redis", a, c, trigger="closed")


@pytest.mark.asyncio
async def test_closed_handler_accepts_go_style_keys():
    a, c = uuid.uuid4(), uuid.uuid4()
    with patch("main.cancel_debounce", AsyncMock()), patch("main.generate_summary", AsyncMock()) as gen:
        await main.process_conversation_closed({"AccountID": str(a), "ConversationID": str(c)}, "p", "r")
    gen.assert_awaited_once()


@pytest.mark.asyncio
async def test_requested_handler_uses_requested_trigger():
    a, c = uuid.uuid4(), uuid.uuid4()
    with patch("main.generate_summary", AsyncMock()) as gen:
        await main.process_summary_requested({"account_id": str(a), "conversation_id": str(c), "requested_by": "x"}, "p", "r")
    gen.assert_awaited_once_with("p", "r", a, c, trigger="requested", requested_by="x")


@pytest.mark.asyncio
@pytest.mark.parametrize("payload", [{}, {"account_id": "nope", "conversation_id": "x"}, {"account_id": str(uuid.uuid4())}])
async def test_malformed_payloads_are_dropped(payload):
    with patch("main.cancel_debounce", AsyncMock()) as cancel, patch("main.generate_summary", AsyncMock()) as gen:
        await main.process_summary_requested(payload, "p", "r")
        await main.process_conversation_closed(payload, "p", "r")
    gen.assert_not_awaited()
    cancel.assert_not_awaited()
