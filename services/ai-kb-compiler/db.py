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

async def create_db_pool(dsn: str) -> asyncpg.Pool:
    # Setup custom type conversion for vector type if needed,
    # but casting embedding::float8[] and $1::vector in SQL is safer and driver-agnostic.
    return await asyncpg.create_pool(
        dsn,
        min_size=2,
        max_size=10,
        timeout=30.0
    )
