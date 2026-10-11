import asyncpg

from whatfunnel_ai.db import ScopedDB, json_object, load_account_settings
from whatfunnel_ai.db import create_db_pool as _create_db_pool

__all__ = ["ScopedDB", "create_db_pool", "json_object", "load_account_settings"]


async def _init_conn(conn: asyncpg.Connection) -> None:
    await conn.execute("SET hnsw.iterative_scan = 'relaxed_order'")


async def create_db_pool(dsn: str) -> asyncpg.Pool:
    return await _create_db_pool(dsn, init=_init_conn)
