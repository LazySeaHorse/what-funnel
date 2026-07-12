import base64
import os
from cryptography.hazmat.primitives.ciphers.aead import AESGCM

def get_key_bytes(key_str: str) -> bytes:
    """
    Decodes the encryption key string to 32 bytes.
    Supports base64, hex, and raw fallback.
    """
    # 1. Try base64
    try:
        decoded = base64.b64decode(key_str, validate=True)
        if len(decoded) == 32:
            return decoded
    except Exception:
        pass

    # 2. Try hex
    try:
        decoded = bytes.fromhex(key_str)
        if len(decoded) == 32:
            return decoded
    except Exception:
        pass

    # 3. Fallback to raw bytes
    raw = key_str.encode("utf-8")
    if len(raw) < 32:
        return raw.ljust(32, b"\0")
    return raw[:32]

def encrypt(key: bytes, plaintext: bytes) -> str:
    """
    Encrypts plaintext using AES-256-GCM and returns hex(nonce || ciphertext_with_tag)
    to match the Go implementation.
    """
    nonce = os.urandom(12)
    aesgcm = AESGCM(key)
    sealed = aesgcm.encrypt(nonce, plaintext, None)
    return (nonce + sealed).hex()

def decrypt(key: bytes, encrypted_str: str) -> bytes:
    """
    Decrypts ciphertext. Supports both hex (Go style) and base64 (spec style).
    """
    # Try decoding as hex first
    try:
        data = bytes.fromhex(encrypted_str)
        if len(data) >= 12:
            nonce = data[:12]
            ciphertext = data[12:]
            aesgcm = AESGCM(key)
            return aesgcm.decrypt(nonce, ciphertext, None)
    except Exception:
        pass

    # Try decoding as base64
    try:
        data = base64.b64decode(encrypted_str)
        if len(data) >= 12:
            nonce = data[:12]
            ciphertext = data[12:]
            aesgcm = AESGCM(key)
            return aesgcm.decrypt(nonce, ciphertext, None)
    except Exception:
        pass

    raise ValueError("crypto: invalid ciphertext")
