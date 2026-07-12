import os

class Config:
    DATABASE_URL: str = os.getenv(
        "DATABASE_URL",
        "postgres://whatfunnel:whatfunnel@localhost:5432/whatfunnel?sslmode=disable"
    )
    REDIS_URL: str = os.getenv("REDIS_URL", "localhost:6379")
    APP_ENCRYPTION_KEY: str = os.getenv("APP_ENCRYPTION_KEY", "")
    LOG_LEVEL: str = os.getenv("LOG_LEVEL", "INFO")

config = Config()
