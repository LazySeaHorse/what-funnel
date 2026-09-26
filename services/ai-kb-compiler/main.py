import asyncio
import logging
import uuid
from contextlib import asynccontextmanager
from typing import Optional

from fastapi import Depends, FastAPI, Header, HTTPException, status

import kb_service
from audit import write_audit_log
from auth import get_db, get_internal_token, verify_internal_auth
from config import config
from db import ScopedDB, create_db_pool
from ingestions import run_worker, stop_worker
from llm import get_ai_config, provider_client
from mining import run_mining
from scheduler import start_scheduler
from schemas import (
    ApproveSuggestionRequest,
    CompilePasteRequest,
    CompilePasteResponse,
    CompilePasteSchema,
    CreateIngestionRequest,
    PublishIngestionRequest,
    RejectSuggestionRequest,
    UpdateConceptRequest,
    UpdatePatternRequest,
)
from slug import get_unique_slug, slugify

# Set up logging
logging.basicConfig(level=getattr(logging, config.LOG_LEVEL.upper(), logging.INFO))
logger = logging.getLogger("ai-kb-compiler")


@asynccontextmanager
async def lifespan(app: FastAPI):
    # Startup
    logger.info("Connecting to database...")
    app.state.db = await create_db_pool(config.DATABASE_URL)
    logger.info("Database connection pool initialized.")
    app.state.ingestion_worker = asyncio.create_task(
        run_worker(app.state.db, CompilePasteSchema),
        name="kb-ingestion-worker",
    )
    # Start periodic mining scheduler
    app.state.scheduler = start_scheduler(app.state.db)
    yield
    # Shutdown
    logger.info("Stopping scheduler...")
    app.state.scheduler.shutdown()
    await stop_worker(getattr(app.state, "ingestion_worker", None))
    logger.info("Closing database pool...")
    await app.state.db.close()
    logger.info("Database pool closed.")


app = FastAPI(
    title="WhatFunnel AI KB Compiler",
    version="1.0.0",
    lifespan=lifespan,
)


@app.get("/", include_in_schema=False)
async def root():
    return {"status": "ok", "service": "ai-kb-compiler"}


@app.get("/healthz")
async def healthz():
    return {"status": "ok"}


# ===========================================================================
# Ingestions
# ===========================================================================

@app.post("/internal/kb/ingestions", status_code=status.HTTP_202_ACCEPTED)
async def create_ingestion(
    req: CreateIngestionRequest,
    db: ScopedDB = Depends(get_db),
    x_user_id: Optional[str] = Header(None, alias="X-User-ID"),
):
    return await kb_service.create_ingestion(db, req.raw_text, x_user_id)


@app.get("/internal/kb/ingestions/latest")
async def get_latest_ingestion(db: ScopedDB = Depends(get_db)):
    result = await kb_service.get_latest_ingestion(db)
    return {"ingestion": result}


@app.get("/internal/kb/ingestions/{ingestion_id}")
async def get_ingestion(ingestion_id: uuid.UUID, db: ScopedDB = Depends(get_db)):
    return await kb_service.get_ingestion(db, ingestion_id)


@app.post("/internal/kb/ingestions/{ingestion_id}/publish", status_code=status.HTTP_202_ACCEPTED)
async def publish_ingestion(
    ingestion_id: uuid.UUID,
    req: PublishIngestionRequest,
    db: ScopedDB = Depends(get_db),
):
    return await kb_service.publish_ingestion(db, ingestion_id, req.concepts, req.patterns)


# ===========================================================================
# Paste-to-OKF Compilation Pipeline
# ===========================================================================

@app.post("/internal/kb/compile-paste", response_model=CompilePasteResponse)
async def compile_paste(
    req: CompilePasteRequest,
    db: ScopedDB = Depends(get_db),
    x_user_id: Optional[str] = Header(None, alias="X-User-ID"),
):
    actor_user_id = None
    if x_user_id:
        try:
            actor_user_id = uuid.UUID(x_user_id)
        except ValueError:
            pass

    return await kb_service.compile_paste(
        db=db,
        raw_text=req.raw_text,
        actor_user_id=actor_user_id,
        client_factory=provider_client,
    )


# ===========================================================================
# Concept Management
# ===========================================================================

@app.get("/internal/kb/concepts")
async def list_concepts(db: ScopedDB = Depends(get_db)):
    concepts = await kb_service.list_concepts(db)
    return {"concepts": concepts}


@app.delete("/internal/kb/purge")
async def purge_knowledge_base(
    db: ScopedDB = Depends(get_db),
    x_user_id: Optional[str] = Header(None, alias="X-User-ID"),
):
    actor_user_id = None
    if x_user_id:
        try:
            actor_user_id = uuid.UUID(x_user_id)
        except ValueError:
            pass

    return await kb_service.purge_knowledge_base(db, actor_user_id)


@app.delete("/internal/kb/concepts/{concept_id}")
async def delete_concept(
    concept_id: str,
    db: ScopedDB = Depends(get_db),
    x_user_id: Optional[str] = Header(None, alias="X-User-ID"),
):
    try:
        concept_uuid = uuid.UUID(concept_id)
    except ValueError:
        raise HTTPException(status_code=400, detail="Invalid concept ID format")

    actor_user_id = None
    if x_user_id:
        try:
            actor_user_id = uuid.UUID(x_user_id)
        except ValueError:
            pass

    await kb_service.delete_concept(db, concept_uuid, actor_user_id)
    return {"success": True}


@app.put("/internal/kb/concepts/{concept_id}")
async def update_concept(
    concept_id: str,
    req: UpdateConceptRequest,
    db: ScopedDB = Depends(get_db),
    x_user_id: Optional[str] = Header(None, alias="X-User-ID"),
):
    try:
        concept_uuid = uuid.UUID(concept_id)
    except ValueError:
        raise HTTPException(status_code=400, detail="Invalid concept ID format")

    actor_user_id = None
    if x_user_id:
        try:
            actor_user_id = uuid.UUID(x_user_id)
        except ValueError:
            pass

    updated = await kb_service.update_concept(
        db=db,
        concept_uuid=concept_uuid,
        req=req,
        actor_user_id=actor_user_id,
        client_factory=provider_client,
    )
    return {"success": True, "concept": updated}


# ===========================================================================
# Dormant Mining
# ===========================================================================

@app.post("/internal/kb/mine/trigger")
async def trigger_mine(db: ScopedDB = Depends(get_db)):
    return await run_mining(db)


@app.get("/internal/kb/mining-runs/latest")
async def latest_mining_run(db: ScopedDB = Depends(get_db)):
    record = await kb_service.latest_mining_run(db)
    return {"last_run": record}


# ===========================================================================
# Suggestion Review
# ===========================================================================

@app.get("/internal/kb/suggestions")
async def list_suggestions(
    status_filter: str = "pending",
    db: ScopedDB = Depends(get_db),
):
    suggestions = await kb_service.list_suggestions(db, status_filter)
    return {"suggestions": suggestions}


@app.post("/internal/kb/suggestions/{suggestion_id}/approve")
async def approve_suggestion(
    suggestion_id: str,
    req: ApproveSuggestionRequest,
    db: ScopedDB = Depends(get_db),
):
    try:
        sugg_uuid = uuid.UUID(suggestion_id)
        reviewed_by_uuid = uuid.UUID(req.reviewed_by)
    except ValueError:
        raise HTTPException(status_code=400, detail="Invalid suggestion ID or user ID format")

    await kb_service.approve_suggestion(
        db=db,
        sugg_uuid=sugg_uuid,
        reviewed_by_uuid=reviewed_by_uuid,
        edited_payload=req.edited_payload,
        client_factory=provider_client,
    )
    return {"success": True}


@app.post("/internal/kb/suggestions/{suggestion_id}/reject")
async def reject_suggestion(
    suggestion_id: str,
    req: RejectSuggestionRequest,
    db: ScopedDB = Depends(get_db),
):
    try:
        sugg_uuid = uuid.UUID(suggestion_id)
        reviewed_by_uuid = uuid.UUID(req.reviewed_by)
    except ValueError:
        raise HTTPException(status_code=400, detail="Invalid suggestion ID or user ID format")

    await kb_service.reject_suggestion(db, sugg_uuid, reviewed_by_uuid)
    return {"success": True}


# ===========================================================================
# Pattern Management
# ===========================================================================

@app.get("/internal/kb/patterns")
async def list_patterns(db: ScopedDB = Depends(get_db)):
    patterns = await kb_service.list_patterns(db)
    return {"patterns": patterns}


@app.delete("/internal/kb/patterns/{pattern_id}")
async def delete_pattern(
    pattern_id: str,
    db: ScopedDB = Depends(get_db),
    x_user_id: Optional[str] = Header(None, alias="X-User-ID"),
):
    try:
        pattern_uuid = uuid.UUID(pattern_id)
    except ValueError:
        raise HTTPException(status_code=400, detail="Invalid pattern ID format")

    actor_user_id = None
    if x_user_id:
        try:
            actor_user_id = uuid.UUID(x_user_id)
        except ValueError:
            pass

    await kb_service.delete_pattern(db, pattern_uuid, actor_user_id)
    return {"success": True}


@app.put("/internal/kb/patterns/{pattern_id}")
async def update_pattern(
    pattern_id: str,
    req: UpdatePatternRequest,
    db: ScopedDB = Depends(get_db),
    x_user_id: Optional[str] = Header(None, alias="X-User-ID"),
):
    try:
        pattern_uuid = uuid.UUID(pattern_id)
    except ValueError:
        raise HTTPException(status_code=400, detail="Invalid pattern ID format")

    actor_user_id = None
    if x_user_id:
        try:
            actor_user_id = uuid.UUID(x_user_id)
        except ValueError:
            pass

    updated = await kb_service.update_pattern(
        db=db,
        pattern_uuid=pattern_uuid,
        req=req,
        actor_user_id=actor_user_id,
        client_factory=provider_client,
    )
    return {"success": True, "pattern": updated}
