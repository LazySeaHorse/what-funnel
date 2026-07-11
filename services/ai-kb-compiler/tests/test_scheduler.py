import os
import pytest
from unittest.mock import patch, MagicMock, AsyncMock
from scheduler import scheduled_mining_job, start_scheduler
from db import create_db_pool

DATABASE_URL = os.getenv(
    "DATABASE_URL",
    "postgres://whatfunnel:whatfunnel@postgres:5432/whatfunnel?sslmode=disable"
)

@pytest.mark.asyncio
async def test_scheduled_mining_job():
    pool = await create_db_pool(DATABASE_URL)
    try:
        # Fetch actual account count in the test database
        async with pool.acquire() as conn:
            rows = await conn.fetch("SELECT id FROM accounts")
            expected_calls = len(rows)

        with patch("scheduler.run_mining", new_callable=AsyncMock) as mock_run:
            await scheduled_mining_job(pool)
            assert mock_run.call_count == expected_calls
    finally:
        await pool.close()

def test_start_scheduler():
    mock_pool = MagicMock()
    scheduler = start_scheduler(mock_pool)
    assert scheduler is not None
    assert scheduler.running
    assert len(scheduler.get_jobs()) == 1
    scheduler.shutdown()
