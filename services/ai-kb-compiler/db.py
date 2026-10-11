"""Thin re-export: the implementation lives in the shared whatfunnel_ai package."""
from whatfunnel_ai.db import ScopedDB, create_db_pool

__all__ = ["ScopedDB", "create_db_pool"]
