"""AES-256-GCM helpers shared by the Python services (wire format matches the Go implementation)."""

import base64
import binascii
import os

from cryptography.exceptions import InvalidTag
from cryptography.hazmat.primitives.ciphers.aead import AESGCM

NONCE_SIZE = 12


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
            raise ValueError(f"crypto: invalid 64-character hex encoding: {exc}") from exc
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
    nonce = os.urandom(NONCE_SIZE)
    sealed = AESGCM(key).encrypt(nonce, plaintext, None)
    return (nonce + sealed).hex()


def _open(key: bytes, data: bytes) -> bytes:
    if len(data) <= NONCE_SIZE:
        raise ValueError("ciphertext is too short")
    return AESGCM(key).decrypt(data[:NONCE_SIZE], data[NONCE_SIZE:], None)


def decrypt(key: bytes, encrypted_str: str) -> bytes:
    """
    Decrypts ciphertext. Supports both hex (Go style) and base64 (spec style).

    Only encoding problems fall through to the next format. An authentication failure
    (wrong key or tampered data) is reported as such instead of being swallowed.
    """
    try:
        data = bytes.fromhex(encrypted_str)
    except ValueError:
        data = None
    if data is not None:
        try:
            return _open(key, data)
        except (InvalidTag, ValueError):
            pass  # A base64 string can also look like hex; give the other encoding a chance.

    try:
        data = base64.b64decode(encrypted_str)
    except (binascii.Error, ValueError) as exc:
        if data is not None:
            raise ValueError("crypto: decryption failed (wrong key or corrupted ciphertext)") from exc
        raise ValueError("crypto: ciphertext is neither valid hex nor valid base64") from exc
    try:
        return _open(key, data)
    except InvalidTag as exc:
        raise ValueError("crypto: decryption failed (wrong key or corrupted ciphertext)") from exc
    except ValueError as exc:
        raise ValueError(f"crypto: invalid ciphertext: {exc}") from exc
