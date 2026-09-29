import base64
import secrets

import pytest

from whatfunnel_ai.crypto import decrypt, encrypt, get_key_bytes


def test_roundtrip_hex():
    key = secrets.token_bytes(32)
    assert decrypt(key, encrypt(key, b"hello")) == b"hello"


def test_decrypts_base64_encoding():
    key = secrets.token_bytes(32)
    raw = bytes.fromhex(encrypt(key, b"hello base64"))
    assert decrypt(key, base64.b64encode(raw).decode()) == b"hello base64"


def test_wrong_key_reports_authentication_failure():
    ciphertext = encrypt(secrets.token_bytes(32), b"secret")
    with pytest.raises(ValueError, match="wrong key or corrupted ciphertext"):
        decrypt(secrets.token_bytes(32), ciphertext)


def test_tampered_ciphertext_is_rejected():
    key = secrets.token_bytes(32)
    ciphertext = encrypt(key, b"secret")
    flipped = "0" if ciphertext[-1] != "0" else "1"
    with pytest.raises(ValueError):
        decrypt(key, ciphertext[:-1] + flipped)


@pytest.mark.parametrize("garbage", ["", "zz", "abcd", "not base64 at all!!!"])
def test_garbage_is_rejected_with_meaningful_error(garbage):
    with pytest.raises(ValueError, match="crypto:"):
        decrypt(secrets.token_bytes(32), garbage)


def test_key_parsing():
    assert get_key_bytes("ab" * 32) == bytes.fromhex("ab" * 32)
    assert get_key_bytes("change-me-32-byte-hex-key-padded") == b"change-me-32-byte-hex-key-padded"
    with pytest.raises(ValueError, match="invalid 64-character hex encoding"):
        get_key_bytes("ab" * 31 + "zz")
    with pytest.raises(ValueError, match="64 hex characters or 32 raw bytes"):
        get_key_bytes("short")
