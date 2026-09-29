"""Thin re-export: the implementation lives in the shared whatfunnel_ai package."""
from whatfunnel_ai.crypto import decrypt, encrypt, get_key_bytes

__all__ = ["decrypt", "encrypt", "get_key_bytes"]
