import json
import os
import secrets
import pytest
from crypto import get_key_bytes, encrypt, decrypt

@pytest.mark.asyncio
async def test_crypto_interop():
    go_enc_file = "tests/go_encrypted.json"
    python_enc_file = "tests/python_encrypted.json"

    # 1. Generate Python encrypted fixtures for Go to decrypt
    test_cases = [
        {"plaintext": "hello from Python! standard ascii"},
        {"plaintext": "special characters: !@#$%^&*()_+={}[]|\\:;'<>,.?/~`"},
        {"plaintext": "unicode support: 🚀 🧑‍💻 🇨🇦 中文 UTF-8"},
    ]

    for tc in test_cases:
        key = secrets.token_bytes(32)
        key_hex = key.hex()
        tc["key_hex"] = key_hex
        
        ciphertext = encrypt(key, tc["plaintext"].encode("utf-8"))
        tc["ciphertext"] = ciphertext

    # Save to JSON file
    with open(python_enc_file, "w") as f:
        json.dump(test_cases, f, indent=2)
    print(f"Wrote Python encrypted fixtures to {python_enc_file}")

    # 2. Read Go encrypted fixtures and decrypt them
    assert os.path.exists(go_enc_file), f"Go encrypted fixtures not found at {go_enc_file}. Make sure to run Go tests first."
    
    with open(go_enc_file, "r") as f:
        go_test_cases = json.load(f)

    for tc in go_test_cases:
        key = get_key_bytes(tc["key_hex"])
        decrypted_bytes = decrypt(key, tc["ciphertext"])
        decrypted_text = decrypted_bytes.decode("utf-8")
        assert decrypted_text == tc["plaintext"], "Python decrypted plaintext does not match Go's original"
        
    print("Successfully decrypted all Go fixtures!")
