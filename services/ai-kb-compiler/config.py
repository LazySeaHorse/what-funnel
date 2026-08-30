import os

class Config:
    DATABASE_URL: str = os.getenv(
        "DATABASE_URL",
        "postgres://whatfunnel:whatfunnel@localhost:5432/whatfunnel?sslmode=disable"
    )
    # The key is base64 encoded in the env var
    APP_ENCRYPTION_KEY: str = os.getenv("APP_ENCRYPTION_KEY", "")
    PORT: int = int(os.getenv("PORT", "8085"))
    LOG_LEVEL: str = os.getenv("LOG_LEVEL", "INFO")
    MINING_INTERVAL_HOURS: int = int(os.getenv("MINING_INTERVAL_HOURS", "6"))
    AI_REQUEST_TIMEOUT_SECONDS: float = float(os.getenv("AI_REQUEST_TIMEOUT_SECONDS", "60"))

config = Config()
