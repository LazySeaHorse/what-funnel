import asyncio
import json
import re
from dataclasses import dataclass
from typing import Any

import httpx


class ProviderError(RuntimeError):
    pass


def _clean_json_content(content: str) -> str:
    content = re.sub(r"<thought>.*?</thought>", "", content, flags=re.DOTALL).strip()
    match = re.search(r"```(?:json)?\s*([\s\S]*?)\s*```", content)
    return match.group(1).strip() if match else content


@dataclass(frozen=True)
class ProviderClient:
    api_key: str
    base_url: str
    timeout_seconds: float = 60.0
    max_attempts: int = 3

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
                await asyncio.sleep(2.0 * (attempt + 1))

        raise ProviderError(
            f"AI provider request failed for model {payload.get('model', '')}"
        ) from last_error

    async def complete(
        self,
        model: str,
        messages: list[dict[str, str]],
        response_schema: Any,
    ) -> dict[str, Any]:
        schema_json = json.dumps(response_schema.model_json_schema())
        payload = {
            "model": model,
            "messages": [
                {
                    "role": "system",
                    "content": (
                        "You are a helpful assistant that always outputs JSON "
                        f"matching this schema:\n{schema_json}"
                    ),
                },
                *messages,
            ],
            "response_format": {"type": "json_object"},
            "temperature": 0.0,
        }

        timeout = httpx.Timeout(self.timeout_seconds)
        async with httpx.AsyncClient(timeout=timeout) as client:
            response = await self._post(client, "/chat/completions", payload)

        try:
            content = response.json()["choices"][0]["message"]["content"]
            parsed = json.loads(_clean_json_content(content))
            validated = response_schema.model_validate(parsed)
        except (KeyError, IndexError, TypeError, json.JSONDecodeError) as error:
            raise ProviderError("AI provider returned an invalid completion response") from error
        except Exception as error:
            raise ProviderError("AI provider response failed schema validation") from error
        return validated.model_dump()

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
