import json


async def publish_stream(redis_client, stream: str, payload: dict) -> None:
    """XADD one event in the shape every stream consumer reads: a JSON document under "payload"."""
    await redis_client.xadd(stream, {"payload": json.dumps(payload).encode("utf-8")})
