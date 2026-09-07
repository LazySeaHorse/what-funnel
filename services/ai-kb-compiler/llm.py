from fastapi import HTTPException

from config import config as app_config
from whatfunnel_ai import (
    AIConfiguration,
    AIConfigurationError,
    ProviderClient,
    load_ai_configuration,
)


async def get_ai_config(db) -> AIConfiguration:
    try:
        return await load_ai_configuration(db, app_config.APP_ENCRYPTION_KEY)
    except AIConfigurationError as error:
        status_code = 409 if "not configured" in str(error).lower() else 500
        raise HTTPException(status_code=status_code, detail=str(error)) from error


def provider_client(config: AIConfiguration) -> ProviderClient:
    return ProviderClient(
        api_key=config.api_key,
        base_url=config.base_url,
        timeout_seconds=app_config.AI_REQUEST_TIMEOUT_SECONDS,
    )
