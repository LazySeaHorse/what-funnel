import pytest
from unittest.mock import AsyncMock, MagicMock

from llm import get_ai_config


@pytest.mark.asyncio
async def test_missing_provider_config_does_not_return_mock_credentials():
    db = MagicMock()
    db.account_id = "account-without-provider"
    db.fetchrow = AsyncMock(return_value=None)

    with pytest.raises(ValueError, match="not configured"):
        await get_ai_config(db)
