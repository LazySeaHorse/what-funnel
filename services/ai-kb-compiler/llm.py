import json
import logging
import httpx
import uuid
from typing import List, Tuple, Any
from pydantic import BaseModel
from fastapi import HTTPException


from db import ScopedDB
from crypto import decrypt, get_key_bytes
from config import config as app_config

logger = logging.getLogger("ai-kb-compiler")

async def get_ai_config(db: ScopedDB) -> Tuple[str, str, str, str]:
    """
    Retrieves and decrypts the AI provider config for the scoped account.
    Returns: (api_key, base_url, completion_model, embedding_model)
    """
    row = await db.fetchrow(
        "SELECT ai_provider_config FROM accounts WHERE id = $1",
        db.account_id
    )
    if not row or not row["ai_provider_config"]:
        raise HTTPException(
            status_code=400,
            detail="AI provider is not configured for this account."
        )

    # Decrypt
    key = get_key_bytes(app_config.APP_ENCRYPTION_KEY)
    try:
        decrypted_bytes = decrypt(key, row["ai_provider_config"])
        cfg_str = decrypted_bytes.decode("utf-8")
    except Exception as e:
        logger.error(f"Failed to decrypt ai_provider_config for account {db.account_id}: {e}")
        raise HTTPException(
            status_code=500,
            detail="Failed to decrypt AI provider configuration."
        )

    try:
        cfg = json.loads(cfg_str)
    except Exception as e:
        logger.error(f"Failed to parse ai_provider_config JSON for account {db.account_id}: {e}")
        raise HTTPException(
            status_code=500,
            detail="AI provider configuration is not valid JSON."
        )

    api_key = cfg.get("api_key") or cfg.get("apiKey")
    base_url = cfg.get("base_url") or cfg.get("baseUrl") or "https://api.openai.com/v1"
    completion_model = cfg.get("completion_model") or cfg.get("completionModel") or "gpt-4o-mini"
    embedding_model = cfg.get("embedding_model") or cfg.get("embeddingModel") or "text-embedding-3-small"

    if not api_key:
        raise HTTPException(
            status_code=400,
            detail="AI provider API key is missing in the configuration."
        )

    return api_key, base_url, completion_model, embedding_model

async def embed(api_key: str, base_url: str, model: str, text: str) -> List[float]:
    """
    Generates embedding for a given text.
    Fails loudly if the returned embedding is not 1536 dimensions.
    """
    url = f"{base_url.rstrip('/')}/embeddings"
    headers = {
        "Authorization": f"Bearer {api_key}",
        "Content-Type": "application/json"
    }
    payload = {
        "input": text,
        "model": model
    }

    async with httpx.AsyncClient() as client:
        try:
            response = await client.post(url, json=payload, headers=headers, timeout=30.0)
            response.raise_for_status()
        except Exception as e:
            logger.error(f"Embedding request failed: {e}")
            raise HTTPException(
                status_code=502,
                detail=f"Failed to generate embeddings from AI provider: {e}"
            )

        res_json = response.json()
        try:
            embedding = res_json["data"][0]["embedding"]
        except (KeyError, IndexError) as e:
            logger.error(f"Invalid embedding response format: {res_json}")
            raise HTTPException(
                status_code=502,
                detail="Invalid response structure from AI provider embeddings endpoint."
            )

        if len(embedding) != 1536:
            raise HTTPException(
                status_code=400,
                detail=f"Unsupported embedding dimension: expected 1536, got {len(embedding)}. Only 1536-dimension models are supported."
            )

        return embedding

async def complete(api_key: str, base_url: str, model: str, prompt: str, response_schema: Any) -> dict:
    """
    Executes a structured chat completion against the OpenAI-compatible API.
    Validates output using the provided Pydantic model.
    """
    url = f"{base_url.rstrip('/')}/chat/completions"
    headers = {
        "Authorization": f"Bearer {api_key}",
        "Content-Type": "application/json"
    }

    # Format the prompt to request JSON structure
    schema_json = json.dumps(response_schema.model_json_schema())
    full_prompt = (
        f"{prompt}\n\n"
        f"You MUST return a JSON object matching this schema:\n"
        f"{schema_json}"
    )

    payload = {
        "model": model,
        "messages": [
            {"role": "system", "content": "You are a helpful assistant that always outputs JSON matching the requested schema."},
            {"role": "user", "content": full_prompt}
        ],
        "response_format": {
            "type": "json_object"
        },
        "temperature": 0.0
    }

    async with httpx.AsyncClient() as client:
        try:
            response = await client.post(url, json=payload, headers=headers, timeout=60.0)
            response.raise_for_status()
        except Exception as e:
            logger.error(f"Completion request failed: {e}")
            raise HTTPException(
                status_code=502,
                detail=f"Chat completion failed on AI provider: {e}"
            )

        res_json = response.json()
        try:
            content = res_json["choices"][0]["message"]["content"]
        except (KeyError, IndexError) as e:
            logger.error(f"Invalid completion response format: {res_json}")
            raise HTTPException(
                status_code=502,
                detail="Invalid response structure from AI provider completion endpoint."
            )

        try:
            parsed = json.loads(content)
        except Exception as e:
            logger.error(f"Failed to parse content as JSON: {content}")
            raise HTTPException(
                status_code=502,
                detail="AI provider response could not be parsed as JSON."
            )

        try:
            validated = response_schema.model_validate(parsed)
            return validated.model_dump()
        except Exception as e:
            logger.error(f"Pydantic validation failed for schema {response_schema.__name__}: {e}. Raw content: {content}")
            raise HTTPException(
                status_code=502,
                detail=f"AI provider response failed schema validation: {e}"
            )
