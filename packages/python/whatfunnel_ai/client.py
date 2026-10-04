import asyncio
import json
import random
import re
import time
from datetime import datetime, timezone
from email.utils import parsedate_to_datetime
from dataclasses import dataclass, field
from typing import Any

import httpx


class ProviderError(RuntimeError):
    pass


def _clean_json_content(content: str) -> str:
    content = re.sub(r"<thought>.*?</thought>", "", content, flags=re.DOTALL).strip()
    match = re.search(r"```(?:json)?\s*([\s\S]*?)\s*```", content)
    return match.group(1).strip() if match else content


def parse_retry_after(value: str | None, now: datetime | None = None) -> float | None:
    """Seconds to wait from a Retry-After header (delta-seconds or HTTP-date), or None."""
    if not value:
        return None
    value = value.strip()
    try:
        seconds = float(value)
    except ValueError:
        try:
            when = parsedate_to_datetime(value)
        except (TypeError, ValueError):
            return None
        if when.tzinfo is None:
            when = when.replace(tzinfo=timezone.utc)
        seconds = (when - (now or datetime.now(timezone.utc))).total_seconds()
    if seconds != seconds:  # NaN
        return None
    return max(0.0, seconds)


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
    # Exponential backoff base for 429/5xx retries: base * 2**retry, with jitter.
    retry_backoff_seconds: float = 1.0
    # Longest single wait before a retry. A Retry-After beyond this is not waited out: the call
    # fails (fast) instead of outliving the caller's own deadline.
    max_retry_wait_seconds: float = 10.0

    def _retry_delay(self, retry: int, retry_after: float | None) -> float:
        if retry_after is not None:
            return retry_after + random.uniform(0.0, min(1.0, retry_after * 0.1))
        ceiling = min(self.max_retry_wait_seconds, self.retry_backoff_seconds * (2 ** retry))
        return random.uniform(ceiling / 2, ceiling)  # equal jitter

    def _headers(self) -> dict[str, str]:
        return {
            "Authorization": f"Bearer {self.api_key}",
            "Content-Type": "application/json",
        }

    async def _post(self, client: httpx.AsyncClient, path: str, payload: dict[str, Any]) -> httpx.Response:
        """POST with bounded retries for rate limiting (429) and server errors (5xx) only.

        Other 4xx responses, timeouts and connection errors are not retried: they fail at once so
        the caller can fail closed within its deadline instead of waiting out another attempt.
        """
        url = f"{self.base_url.rstrip('/')}{path}"
        model = payload.get("model", "")
        last_error: Exception | None = None
        for attempt in range(self.max_attempts):
            retry_after: float | None = None
            try:
                response = await client.post(url, json=payload, headers=self._headers())
                response.raise_for_status()
                return response
            except httpx.HTTPStatusError as error:
                status = error.response.status_code
                if status != 429 and status < 500:
                    raise ProviderError(f"AI provider rejected model {model}") from error
                last_error = error
                retry_after = parse_retry_after(error.response.headers.get("Retry-After"))
            except (httpx.TimeoutException, httpx.NetworkError) as error:
                raise ProviderError(f"AI provider request failed for model {model}") from error

            if attempt + 1 < self.max_attempts:
                if retry_after is not None and retry_after > self.max_retry_wait_seconds:
                    break
                await asyncio.sleep(self._retry_delay(attempt, retry_after))

        raise ProviderError(f"AI provider request failed for model {model}") from last_error

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
