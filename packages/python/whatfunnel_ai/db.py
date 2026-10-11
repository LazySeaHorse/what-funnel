import os
import uuid
from contextlib import asynccontextmanager
from typing import Any, Awaitable, Callable, List, Optional

import asyncpg


class ScopedDB:
    def __init__(self, pool: asyncpg.Pool, account_id: uuid.UUID):
        if not account_id:
            raise ValueError("ScopedDB: account_id must not be empty/nil")
        self.pool = pool
        self.account_id = account_id

    def append_account_id(self, args: list) -> tuple[int, list]:
        """
        Appends the scoped account_id to the arguments list and returns
        the 1-based index placeholder position and updated list.
        """
        args.append(self.account_id)
        return len(args), args

    async def fetch(self, query: str, *args: Any) -> List[asyncpg.Record]:
        return await self.pool.fetch(query, *args)

    async def fetchrow(self, query: str, *args: Any) -> Optional[asyncpg.Record]:
        return await self.pool.fetchrow(query, *args)

    async def fetchval(self, query: str, *args: Any) -> Any:
        return await self.pool.fetchval(query, *args)

    async def execute(self, query: str, *args: Any) -> str:
        return await self.pool.execute(query, *args)

    @asynccontextmanager
    async def transaction(self):
        """Yield a ScopedDB bound to a single connection inside a transaction.

        Everything run through the yielded object commits or rolls back together.
        """
        async with self.pool.acquire() as conn:
            async with conn.transaction():
                yield ScopedDB(conn, self.account_id)


async def create_db_pool(
    dsn: str, init: Optional[Callable[[asyncpg.Connection], Awaitable[None]]] = None
) -> asyncpg.Pool:
    """Create the service pool; `init` runs on every new connection."""
    kwargs = {
        "min_size": int(os.getenv("DB_POOL_MIN_SIZE", "1")),
        "max_size": int(os.getenv("DB_POOL_MAX_SIZE", "3")),
        "timeout": 30.0,
        "init": init,
    }
    if os.getenv("DB_PGBOUNCER", "").lower() == "true" or ":6432" in dsn:
        kwargs["statement_cache_size"] = 0

    return await asyncpg.create_pool(dsn, **kwargs)
