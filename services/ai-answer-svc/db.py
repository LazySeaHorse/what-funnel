import uuid
from typing import Any, List, Optional
import asyncpg

class ScopedDB:
    def __init__(self, pool: asyncpg.Pool, account_id: uuid.UUID):
        if not account_id:
            raise ValueError("ScopedDB: account_id must not be empty/nil")
        self.pool = pool
        self.account_id = account_id

    async def fetch(self, query: str, *args: Any) -> List[asyncpg.Record]:
        return await self.pool.fetch(query, *args)

    async def fetchrow(self, query: str, *args: Any) -> Optional[asyncpg.Record]:
        return await self.pool.fetchrow(query, *args)

    async def fetchval(self, query: str, *args: Any) -> Any:
        return await self.pool.fetchval(query, *args)

    async def execute(self, query: str, *args: Any) -> str:
        return await self.pool.execute(query, *args)

async def create_db_pool(dsn: str) -> asyncpg.Pool:
    return await asyncpg.create_pool(
        dsn,
        min_size=2,
        max_size=10,
        timeout=30.0
    )
