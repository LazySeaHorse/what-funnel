import asyncio
import json
import re
import time
from dataclasses import dataclass, field
from typing import Any

import httpx


class ProviderError(RuntimeError):
    pass


def _clean_json_content(content: str) -> str:
    content = re.sub(r"<thought>.*?</thought>", "", content, flags=re.DOTALL).strip()
    match = re.search(r"```(?:json)?\s*([\s\S]*?)\s*```", content)
    return match.group(1).strip() if match else content


@dataclass(frozen=True)
class CompletionResult:
    """A validated structured completion plus call metadata (for decision logging)."""

    data: dict[str, Any]
    usage: dict[str, int] = field(default_factory=dict)
    latency_ms: int = 0


@dataclass(frozen=True)
class ProviderClient:
    api_key: str
    base_url: str
    timeout_seconds: float = 20.0
    max_attempts: int = 2
    retry_backoff_seconds: float = 0.5

    def _headers(self) -> dict[str, str]:
        return {
            "Authorization": f"Bearer {self.api_key}",
            "Content-Type": "application/json",
        }

    async def _post(self, client: httpx.AsyncClient, path: str, payload: dict[str, Any]) -> httpx.Response:
        url = f"{self.base_url.rstrip('/')}{path}"
        last_error: Exception | None = None
        for attempt in range(self.max_attempts):
            try:
                response = await client.post(url, json=payload, headers=self._headers())
                response.raise_for_status()
                return response
            except httpx.HTTPStatusError as error:
                if error.response.status_code != 429 and error.response.status_code < 500:
                    raise ProviderError(
                        f"AI provider rejected model {payload.get('model', '')}"
                    ) from error
                last_error = error
            except (httpx.TimeoutException, httpx.NetworkError) as error:
                last_error = error

            if attempt + 1 < self.max_attempts:
                await asyncio.sleep(self.retry_backoff_seconds * (attempt + 1))

        raise ProviderError(
            f"AI provider request failed for model {payload.get('model', '')}"
        ) from last_error

    async def complete(
        self,
        model: str,
        messages: list[dict[str, str]],
        response_schema: Any,
        max_tokens: int | None = None,
    ) -> dict[str, Any]:
        return (await self.complete_detailed(model, messages, response_schema, max_tokens)).data

    async def complete_detailed(
        self,
        model: str,
        messages: list[dict[str, str]],
        response_schema: Any,
        max_tokens: int | None = None,
    ) -> CompletionResult:
        schema = response_schema.model_json_schema()
        payload = {
            "model": model,
            "messages": [
                {
                    "role": "system",
                    "content": "Return output matching the provided response schema.",
                },
                *messages,
            ],
            "response_format": {
                "type": "json_schema",
                "json_schema": {
                    "name": response_schema.__name__,
                    "strict": True,
                    "schema": schema,
                },
            },
            "temperature": 0.0,
        }
        if max_tokens is not None:
            payload["max_tokens"] = max_tokens

        timeout = httpx.Timeout(self.timeout_seconds)
        started = time.monotonic()
        async with httpx.AsyncClient(timeout=timeout) as client:
            response = await self._post(client, "/chat/completions", payload)
        latency_ms = int((time.monotonic() - started) * 1000)

        try:
            body = response.json()
            content = body["choices"][0]["message"]["content"]
            parsed = json.loads(_clean_json_content(content))
            validated = response_schema.model_validate(parsed)
        except (KeyError, IndexError, TypeError, json.JSONDecodeError) as error:
            raise ProviderError("AI provider returned an invalid completion response") from error
        except Exception as error:
            raise ProviderError("AI provider response failed schema validation") from error
        raw_usage = body.get("usage") if isinstance(body, dict) else None
        usage: dict[str, int] = {}
        if isinstance(raw_usage, dict):
            for key in ("prompt_tokens", "completion_tokens", "total_tokens"):
                if isinstance(raw_usage.get(key), int):
                    usage[key] = raw_usage[key]
            details = raw_usage.get("prompt_tokens_details")
            if isinstance(details, dict) and isinstance(details.get("cached_tokens"), int):
                usage["cached_tokens"] = details["cached_tokens"]
        return CompletionResult(validated.model_dump(), usage, latency_ms)

    async def embed(self, model: str, text: str, dimensions: int = 1536) -> list[float]:
        payload = {"input": text, "model": model, "dimensions": dimensions}
        timeout = httpx.Timeout(self.timeout_seconds)
        async with httpx.AsyncClient(timeout=timeout) as client:
            response = await self._post(client, "/embeddings", payload)

        try:
            embedding = response.json()["data"][0]["embedding"]
        except (KeyError, IndexError, TypeError) as error:
            raise ProviderError("AI provider returned an invalid embedding response") from error
        if len(embedding) != dimensions:
            raise ProviderError(
                f"Unsupported embedding dimension: expected {dimensions}, got {len(embedding)}"
            )
        return embedding
