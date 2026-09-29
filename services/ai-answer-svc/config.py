import os

class Config:
    DATABASE_URL: str = os.getenv(
        "DATABASE_URL",
        "postgres://whatfunnel:whatfunnel@localhost:5432/whatfunnel?sslmode=disable"
    )
    REDIS_URL: str = os.getenv("REDIS_URL", "localhost:6379")
    REDIS_CONSUMER_NAME: str = os.getenv("REDIS_CONSUMER_NAME", "")
    REDIS_AUTOCLAIM_MIN_IDLE_MS: int = int(os.getenv("REDIS_AUTOCLAIM_MIN_IDLE_MS", "30000"))
    APP_ENCRYPTION_KEY: str = os.getenv("APP_ENCRYPTION_KEY") or os.getenv("ENCRYPTION_KEY", "")
    INTERNAL_SERVICE_TOKEN: str = os.getenv("INTERNAL_SERVICE_TOKEN", "")
    # Dev-only escape hatch: allow inter-service calls without a token.
    ALLOW_INSECURE_INTERNAL_AUTH: bool = os.getenv("ALLOW_INSECURE_INTERNAL_AUTH", "").lower() in ("true", "1", "yes")
    LOG_LEVEL: str = os.getenv("LOG_LEVEL", "INFO")
    AI_REQUEST_TIMEOUT_SECONDS: float = float(os.getenv("AI_REQUEST_TIMEOUT_SECONDS", "1000"))
    # A run still marked 'replying' after this long is considered dead and may be reclaimed.
    # It must exceed the worst case of one embedding call plus one completion call, otherwise a slow
    # (but alive) generation gets duplicated by a second worker.
    AI_RUN_RECLAIM_SECONDS: float = float(
        os.getenv("AI_RUN_RECLAIM_SECONDS") or max(120.0, 2 * float(os.getenv("AI_REQUEST_TIMEOUT_SECONDS", "1000")) + 60.0)
    )
    AI_CASCADE_CONCURRENCY: int = max(1, int(os.getenv("AI_CASCADE_CONCURRENCY", "8")))
    AI_DEBOUNCE_MAX_ATTEMPTS: int = max(1, int(os.getenv("AI_DEBOUNCE_MAX_ATTEMPTS", "3")))
    STREAM_MAX_DELIVERIES: int = max(1, int(os.getenv("STREAM_MAX_DELIVERIES", "5")))
    AI_DEBOUNCE_ENABLED: bool = os.getenv("AI_DEBOUNCE_ENABLED", "true").lower() in ("true", "1", "yes")
    AI_DEBOUNCE_FIRST_SECONDS: float = float(os.getenv("AI_DEBOUNCE_FIRST_SECONDS", "10.0"))
    AI_DEBOUNCE_SUBSEQUENT_SECONDS: float = float(os.getenv("AI_DEBOUNCE_SUBSEQUENT_SECONDS", "5.0"))
    AI_DEBOUNCE_BURST_SECONDS: float = float(os.getenv("AI_DEBOUNCE_BURST_SECONDS", "10.0"))

config = Config()


def internal_service_token() -> str:
    """Return the dedicated inter-service token, read at call time.

    There is deliberately no fallback to SESSION_SECRET. With no token configured
    this fails closed unless ALLOW_INSECURE_INTERNAL_AUTH=true is set explicitly.
    """
    token = os.getenv("INTERNAL_SERVICE_TOKEN", config.INTERNAL_SERVICE_TOKEN).strip()
    if token:
        return token
    if os.getenv("ALLOW_INSECURE_INTERNAL_AUTH", "").lower() in ("true", "1", "yes") or config.ALLOW_INSECURE_INTERNAL_AUTH:
        return ""
    raise RuntimeError(
        "INTERNAL_SERVICE_TOKEN is not configured; refusing inter-service call "
        "(set ALLOW_INSECURE_INTERNAL_AUTH=true only for local development)"
    )
