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
    # Per-attempt timeout for each provider call (router, RAG completion, embedding).
    AI_REQUEST_TIMEOUT_SECONDS: float = float(os.getenv("AI_REQUEST_TIMEOUT_SECONDS", "20"))
    # Attempts per provider call (1 = no retry). Retries sleep 0.5 s * attempt.
    AI_PROVIDER_MAX_ATTEMPTS: int = max(1, int(os.getenv("AI_PROVIDER_MAX_ATTEMPTS", "2")))
    # Hard deadline for the whole decision (router + KB retrieval + KB answer). On expiry the cascade
    # fails closed to a human handoff, so a customer gets a reply within seconds or a handoff.
    AI_CASCADE_DEADLINE_SECONDS: float = float(os.getenv("AI_CASCADE_DEADLINE_SECONDS", "45"))
    # A run still marked 'replying' after this long is considered dead and may be reclaimed. It must
    # exceed the cascade deadline, otherwise a slow (but alive) generation gets duplicated by a second worker.
    AI_RUN_RECLAIM_SECONDS: float = float(
        os.getenv("AI_RUN_RECLAIM_SECONDS") or (float(os.getenv("AI_CASCADE_DEADLINE_SECONDS", "45")) + 60.0)
    )
    # Generated (RAG) answers are only drafted for a human to approve until the KB stage is measured.
    # Canned FAQ and greeting replies follow the normal reply-mode rules. The account setting
    # ai_rag_auto_send (true/false) overrides this default per workspace.
    AI_RAG_AUTO_SEND: bool = os.getenv("AI_RAG_AUTO_SEND", "false").lower() in ("true", "1", "yes")
    # Router and KB answer output budgets (tokens). The router emits four enum fields.
    AI_ROUTER_MAX_TOKENS: int = max(16, int(os.getenv("AI_ROUTER_MAX_TOKENS", "150")))
    AI_KB_MAX_TOKENS: int = max(64, int(os.getenv("AI_KB_MAX_TOKENS", "400")))
    # Minimum cosine similarity for a kb_concept to be shown to the KB answer step. Not calibrated
    # against any specific embedding model: tune it with the eval harness.
    AI_KB_MIN_SIMILARITY: float = float(os.getenv("AI_KB_MIN_SIMILARITY", "0.30"))
    AI_CASCADE_CONCURRENCY: int = max(1, int(os.getenv("AI_CASCADE_CONCURRENCY", "8")))
    AI_DEBOUNCE_MAX_ATTEMPTS: int = max(1, int(os.getenv("AI_DEBOUNCE_MAX_ATTEMPTS", "3")))
    STREAM_MAX_DELIVERIES: int = max(1, int(os.getenv("STREAM_MAX_DELIVERIES", "5")))
    AI_DEBOUNCE_ENABLED: bool = os.getenv("AI_DEBOUNCE_ENABLED", "true").lower() in ("true", "1", "yes")
    # Sliding window after the 1st / 2nd / 3rd+ bubble of a batch. The first is short so a single
    # complete question is answered in seconds; later bubbles wait slightly longer for a burst to finish.
    AI_DEBOUNCE_FIRST_SECONDS: float = float(os.getenv("AI_DEBOUNCE_FIRST_SECONDS", "4.0"))
    AI_DEBOUNCE_SUBSEQUENT_SECONDS: float = float(os.getenv("AI_DEBOUNCE_SUBSEQUENT_SECONDS", "4.0"))
    AI_DEBOUNCE_BURST_SECONDS: float = float(os.getenv("AI_DEBOUNCE_BURST_SECONDS", "6.0"))
    # Conversation summaries. Close-triggered runs are debounced (this many seconds since the last
    # summary and new messages). A user request bypasses that, but each conversation has a short
    # cooldown after any attempt and a lock so concurrent/spammed requests do not duplicate LLM calls.
    SUMMARY_MIN_INTERVAL_SECONDS: float = float(os.getenv("SUMMARY_MIN_INTERVAL_SECONDS", "60"))
    SUMMARY_REQUEST_COOLDOWN_SECONDS: int = max(1, int(os.getenv("SUMMARY_REQUEST_COOLDOWN_SECONDS", "10")))
    SUMMARY_MAX_MESSAGES: int = max(1, int(os.getenv("SUMMARY_MAX_MESSAGES", "50")))
    SUMMARY_FIELD_MAX_CHARS: int = max(40, int(os.getenv("SUMMARY_FIELD_MAX_CHARS", "600")))

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
