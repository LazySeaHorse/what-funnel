import json
import os
import secrets
import pytest
from crypto import get_key_bytes, encrypt, decrypt

TEST_CASES = [
    {"plaintext": "hello from Python! standard ascii"},
    {"plaintext": "special characters: !@#$%^&*()_+={}[]|\\:;'<>,.?/~`"},
    {"plaintext": "unicode support: 🚀 🧑‍💻 🇨🇦 中文 UTF-8"},
]

@pytest.mark.asyncio
async def test_crypto_roundtrip():
    for tc in TEST_CASES:
        key = secrets.token_bytes(32)
        ciphertext = encrypt(key, tc["plaintext"].encode("utf-8"))
        decrypted = decrypt(key, ciphertext).decode("utf-8")
        assert decrypted == tc["plaintext"]

@pytest.mark.asyncio
async def test_crypto_interop():
    test_dir = os.path.dirname(__file__)
    go_enc_file = os.path.join(test_dir, "go_encrypted.json")
    python_enc_file = os.path.join(test_dir, "python_encrypted.json")

    # 1. Generate Python encrypted fixtures for Go to decrypt
    test_cases = []
    for tc in TEST_CASES:
        key = secrets.token_bytes(32)
        key_hex = key.hex()
        ciphertext = encrypt(key, tc["plaintext"].encode("utf-8"))
        test_cases.append({
            "plaintext": tc["plaintext"],
            "key_hex": key_hex,
            "ciphertext": ciphertext,
        })

    # Save to JSON file for Go to decrypt if run subsequently
    try:
        with open(python_enc_file, "w") as f:
            json.dump(test_cases, f, indent=2)
        print(f"Wrote Python encrypted fixtures to {python_enc_file}")
    except OSError:
        pass

    # 2. Read Go encrypted fixtures and decrypt them (if available from previous Go test run)
    if not os.path.exists(go_enc_file):
        pytest.skip(f"Go encrypted fixtures not found at {go_enc_file} (run Go tests first to generate them)")

    with open(go_enc_file, "r") as f:
        go_test_cases = json.load(f)

    for tc in go_test_cases:
        key = get_key_bytes(tc["key_hex"])
        decrypted_bytes = decrypt(key, tc["ciphertext"])
        decrypted_text = decrypted_bytes.decode("utf-8")
        assert decrypted_text == tc["plaintext"], "Python decrypted plaintext does not match Go's original"

    print("Successfully decrypted all Go fixtures!")
