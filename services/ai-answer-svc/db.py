import asyncpg

from whatfunnel_ai.db import ScopedDB, json_object, load_account_settings
from whatfunnel_ai.db import create_db_pool as _create_db_pool

__all__ = ["ScopedDB", "create_db_pool", "json_object", "load_account_settings", "message_text"]


def message_text(content) -> str:
    """The text of a messages.content value ("" for non-text or malformed content)."""
    text = json_object(content).get("text")
    return text if isinstance(text, str) else ""


async def _init_conn(conn: asyncpg.Connection) -> None:
    await conn.execute("SET hnsw.iterative_scan = 'relaxed_order'")


async def create_db_pool(dsn: str) -> asyncpg.Pool:
    return await _create_db_pool(dsn, init=_init_conn)
