import os
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
    min_size = int(os.getenv("DB_POOL_MIN_SIZE", "1"))
    max_size = int(os.getenv("DB_POOL_MAX_SIZE", "3"))

    async def init_conn(conn):
        await conn.execute("SET hnsw.iterative_scan = 'relaxed_order'")

    kwargs = {
        "min_size": min_size,
        "max_size": max_size,
        "timeout": 30.0,
        "init": init_conn,
    }
    if os.getenv("DB_PGBOUNCER", "").lower() == "true" or ":6432" in dsn:
        kwargs["statement_cache_size"] = 0

    return await asyncpg.create_pool(dsn, **kwargs)
