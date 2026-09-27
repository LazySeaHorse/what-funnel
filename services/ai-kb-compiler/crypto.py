import base64
import os
from cryptography.hazmat.primitives.ciphers.aead import AESGCM

def get_key_bytes(key_str: str) -> bytes:
    """
    Decodes the encryption key string to 32 bytes.
    Accepts 64-character hex strings or 32-byte raw strings.
    """
    key_str = key_str.strip()
    if len(key_str) == 64:
        try:
            return bytes.fromhex(key_str)
        except ValueError as exc:
            raise ValueError(f"crypto: invalid 64-character hex key: {exc}") from exc
    elif len(key_str) == 32:
        return key_str.encode("utf-8")
    else:
        raise ValueError(
            f"crypto: key must be 64 hex characters or 32 raw bytes, got {len(key_str)} characters"
        )

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
