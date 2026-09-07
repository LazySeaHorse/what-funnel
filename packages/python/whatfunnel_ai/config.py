from dataclasses import dataclass
from typing import Any

from cryptography.hazmat.primitives.ciphers.aead import AESGCM


class AIConfigurationError(ValueError):
    pass


@dataclass(frozen=True)
class AIConfiguration:
    api_key: str
    base_url: str
    analysis_model: str
    reply_model: str
    embedding_model: str


def _key_bytes(key: str) -> bytes:
    try:
        decoded = bytes.fromhex(key)
        if len(decoded) == 32:
            return decoded
    except ValueError:
        pass

    raw = key.encode("utf-8")
    if len(raw) != 32:
        raise AIConfigurationError("AI provider encryption key must be 32 bytes")
    return raw


def _decrypt_api_key(encryption_key: str, ciphertext: str) -> str:
    try:
        data = bytes.fromhex(ciphertext)
        if len(data) < 13:
            raise ValueError("ciphertext is too short")
        plaintext = AESGCM(_key_bytes(encryption_key)).decrypt(
            data[:12], data[12:], None
        )
        return plaintext.decode("utf-8")
    except AIConfigurationError:
        raise
    except Exception as error:
        raise AIConfigurationError("Failed to decrypt AI provider API key") from error


async def load_ai_configuration(db: Any, encryption_key: str) -> AIConfiguration:
    row = await db.fetchrow(
        """
        SELECT base_url, encrypted_api_key, analysis_model, reply_model, embedding_model
        FROM account_ai_providers
        WHERE account_id = $1
        """,
        db.account_id,
    )
    if not row:
        raise AIConfigurationError("AI provider is not configured for this workspace")

    config = AIConfiguration(
        api_key=_decrypt_api_key(encryption_key, row["encrypted_api_key"]),
        base_url=str(row["base_url"]).rstrip("/"),
        analysis_model=str(row["analysis_model"]).strip(),
        reply_model=str(row["reply_model"]).strip(),
        embedding_model=str(row["embedding_model"]).strip(),
    )
    if not all(
        (
            config.api_key,
            config.base_url,
            config.analysis_model,
            config.reply_model,
            config.embedding_model,
        )
    ):
        raise AIConfigurationError("AI provider configuration is incomplete")
    return config
