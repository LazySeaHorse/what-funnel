import logging
import uuid
from contextlib import asynccontextmanager
from fastapi import FastAPI, Header, Depends, HTTPException, status
from pydantic import BaseModel, Field
from typing import List, Optional

from config import config
from db import ScopedDB, create_db_pool

# Set up logging
logging.basicConfig(level=getattr(logging, config.LOG_LEVEL.upper(), logging.INFO))
logger = logging.getLogger("ai-kb-compiler")

@asynccontextmanager
async def lifespan(app: FastAPI):
    # Startup
    logger.info("Connecting to database...")
    app.state.db = await create_db_pool(config.DATABASE_URL)
    logger.info("Database connection pool initialized.")
    yield
    # Shutdown
    logger.info("Closing database pool...")
    await app.state.db.close()
    logger.info("Database pool closed.")

app = FastAPI(
    title="WhatFunnel AI KB Compiler",
    version="1.0.0",
    lifespan=lifespan
)

# Dependency to retrieve a tenant-scoped database client.
# X-Account-ID is forwarded by the API Gateway after auth validation.
async def get_db(x_account_id: str = Header(..., alias="X-Account-ID")) -> ScopedDB:
    try:
        account_uuid = uuid.UUID(x_account_id)
    except ValueError:
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail="Invalid X-Account-ID header format. Must be a valid UUID."
        )
    return ScopedDB(app.state.db, account_uuid)

@app.get("/healthz")
async def healthz():
    return {"status": "ok"}
