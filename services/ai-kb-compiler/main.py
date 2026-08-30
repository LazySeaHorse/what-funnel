import json
import asyncio
import logging
import re
import uuid
from contextlib import asynccontextmanager
from typing import List, Optional, Tuple, Any
from datetime import datetime

from fastapi import FastAPI, Header, Depends, HTTPException, status
from pydantic import BaseModel, Field

from config import config
from db import ScopedDB, create_db_pool
from llm import get_ai_config, embed, complete
from mining import run_mining
from scheduler import start_scheduler
from redis_client import publish_suggestion_created
from ingestions import compilation_prompt, run_worker, stop_worker



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

from playground import router as playground_router
from fastapi.responses import RedirectResponse

app = FastAPI(
    title="WhatFunnel AI KB Compiler",
    version="1.0.0",
    lifespan=lifespan
)

app.include_router(playground_router)

@app.get("/", include_in_schema=False)
async def root_redirect():
    return RedirectResponse(url="/playground")

# Dependency to retrieve a tenant-scoped database client.
async def get_db(x_account_id: str = Header(..., alias="X-Account-ID")) -> ScopedDB:
    try:
        account_uuid = uuid.UUID(x_account_id)
    except ValueError:
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail="Invalid X-Account-ID header format. Must be a valid UUID."
        )
    return ScopedDB(app.state.db, account_uuid)

# Helper to write audit logs
async def write_audit_log(
    db: ScopedDB,
    actor_user_id: Optional[uuid.UUID],
    action: str,
    target_type: str,
    target_id: Optional[uuid.UUID],
    metadata: dict
):
    if actor_user_id:
        user_exists = await db.fetchval(
            "SELECT 1 FROM users WHERE id = $1 AND account_id = $2",
            actor_user_id, db.account_id
        )
        if not user_exists:
            actor_user_id = None

    await db.execute(
        """
        INSERT INTO audit_logs (account_id, actor_user_id, action, target_type, target_id, metadata)
        VALUES ($1, $2, $3, $4, $5, $6)
        """,
        db.account_id,
        actor_user_id,
        action,
        target_type,
        target_id,
        json.dumps(metadata)
    )

# Slug generation with collision handling
def slugify(title: str) -> str:
    s = title.lower()
    s = re.sub(r'[^a-z0-9\s-]', '', s)
    s = re.sub(r'[\s-]+', '-', s)
    return s.strip('-')

async def get_unique_slug(db: ScopedDB, base_slug: str) -> str:
    slug = base_slug
    suffix = 1
    while True:
        row = await db.fetchrow(
            "SELECT id FROM kb_concepts WHERE account_id = $1 AND slug = $2",
            db.account_id, slug
        )
        if not row:
            return slug
        slug = f"{base_slug}-{suffix}"
        suffix += 1

# Pydantic models for Paste Compilation
class OKFConceptDraft(BaseModel):
    type: str = Field(description="OKF concept type, e.g. faq, policy, hours, service, pricing")
    title: str = Field(description="Concept title")
    tags: List[str] = Field(default_factory=list, description="Tags associated with the concept")
    body_markdown: str = Field(description="Markdown body of the concept")

class OKFPatternDraft(BaseModel):
    canonical_question: str = Field(description="The standard representative customer question")
    answer_markdown: str = Field(description="The exact definitive answer in markdown")
    trigger_phrases: List[str] = Field(
        default_factory=list,
        description="Four to eight realistic lowercase customer query variations"
    )

class CompilePasteSchema(BaseModel):
    concepts: List[OKFConceptDraft] = Field(description="List of concepts compiled from raw text")
    patterns: List[OKFPatternDraft] = Field(description="Deterministic customer question and answer patterns")

class CompilePasteRequest(BaseModel):
    raw_text: str

class CompilePasteResponse(BaseModel):
    added_concepts: Optional[List[dict]] = None
    added_patterns: Optional[List[dict]] = None
    suggestion_ids: Optional[List[str]] = None


class CreateIngestionRequest(BaseModel):
    raw_text: str = Field(min_length=1, max_length=500_000)


class PublishIngestionItem(BaseModel):
    id: uuid.UUID
    approved: bool = True
    type: str = Field(min_length=1, max_length=100)
    title: str = Field(min_length=1, max_length=500)
    tags: List[str] = Field(default_factory=list)
    body_markdown: str = Field(min_length=1, max_length=100_000)


class PublishIngestionPattern(BaseModel):
    id: uuid.UUID
    approved: bool = True
    canonical_question: str = Field(min_length=1, max_length=1000)
    answer_markdown: str = Field(min_length=1, max_length=100_000)
    trigger_phrases: List[str] = Field(default_factory=list, max_length=50)


class PublishIngestionRequest(BaseModel):
    concepts: List[PublishIngestionItem] = Field(default_factory=list)
    patterns: List[PublishIngestionPattern] = Field(default_factory=list)

@app.get("/healthz")
async def healthz():
    return {"status": "ok"}


def ingestion_payload(row, concepts=(), patterns=()):
    return {
        "id": str(row["id"]),
        "status": row["status"],
        "error": row["error"],
        "created_at": row["created_at"],
        "updated_at": row["updated_at"],
        "completed_at": row["completed_at"],
        "concepts": [dict(concept) for concept in concepts],
        "patterns": [dict(pattern) for pattern in patterns],
    }


@app.post("/internal/kb/ingestions", status_code=status.HTTP_202_ACCEPTED)
async def create_ingestion(
    req: CreateIngestionRequest,
    db: ScopedDB = Depends(get_db),
    x_user_id: Optional[str] = Header(None, alias="X-User-ID"),
):
    raw_text = req.raw_text.strip()
    if not raw_text:
        raise HTTPException(status_code=422, detail="raw_text must not be blank")
    requested_by = None
    if x_user_id:
        try:
            candidate = uuid.UUID(x_user_id)
            if await db.fetchval(
                "SELECT 1 FROM users WHERE id = $1 AND account_id = $2",
                candidate,
                db.account_id,
            ):
                requested_by = candidate
        except ValueError:
            pass
    row = await db.fetchrow(
        """
        INSERT INTO kb_ingestions (account_id, requested_by, raw_text)
        VALUES ($1, $2, $3)
        RETURNING id, status, error, created_at, updated_at, completed_at
        """,
        db.account_id,
        requested_by,
        raw_text,
    )
    return ingestion_payload(row)


@app.get("/internal/kb/ingestions/latest")
async def get_latest_ingestion(db: ScopedDB = Depends(get_db)):
    row = await db.fetchrow(
        """
        SELECT id, status, error, created_at, updated_at, completed_at
        FROM kb_ingestions
        WHERE account_id = $1
        ORDER BY created_at DESC
        LIMIT 1
        """,
        db.account_id,
    )
    if not row:
        return {"ingestion": None}
    concepts = await db.fetch(
        """
        SELECT id, position, type, title, tags, body_markdown, status, concept_id
        FROM kb_ingestion_items
        WHERE ingestion_id = $1
        ORDER BY position
        """,
        row["id"],
    )
    patterns = await db.fetch(
        """
        SELECT id, position, canonical_question, answer_markdown, trigger_phrases, status, pattern_id
        FROM kb_ingestion_patterns
        WHERE ingestion_id = $1
        ORDER BY position
        """,
        row["id"],
    )
    return {"ingestion": ingestion_payload(row, concepts, patterns)}


@app.get("/internal/kb/ingestions/{ingestion_id}")
async def get_ingestion(ingestion_id: uuid.UUID, db: ScopedDB = Depends(get_db)):
    row = await db.fetchrow(
        """
        SELECT id, status, error, created_at, updated_at, completed_at
        FROM kb_ingestions
        WHERE id = $1 AND account_id = $2
        """,
        ingestion_id,
        db.account_id,
    )
    if not row:
        raise HTTPException(status_code=404, detail="Knowledge ingestion not found")
    concepts = await db.fetch(
        """
        SELECT id, position, type, title, tags, body_markdown, status, concept_id
        FROM kb_ingestion_items
        WHERE ingestion_id = $1
        ORDER BY position
        """,
        ingestion_id,
    )
    patterns = await db.fetch(
        """
        SELECT id, position, canonical_question, answer_markdown, trigger_phrases, status, pattern_id
        FROM kb_ingestion_patterns
        WHERE ingestion_id = $1
        ORDER BY position
        """,
        ingestion_id,
    )
    return ingestion_payload(row, concepts, patterns)


@app.post("/internal/kb/ingestions/{ingestion_id}/publish", status_code=status.HTTP_202_ACCEPTED)
async def publish_ingestion(
    ingestion_id: uuid.UUID,
    req: PublishIngestionRequest,
    db: ScopedDB = Depends(get_db),
):
    if len({item.id for item in req.concepts}) != len(req.concepts):
        raise HTTPException(status_code=422, detail="Each ingestion concept may only be submitted once")
    if len({pattern.id for pattern in req.patterns}) != len(req.patterns):
        raise HTTPException(status_code=422, detail="Each ingestion pattern may only be submitted once")

    async with db.pool.acquire() as conn:
        async with conn.transaction():
            ingestion = await conn.fetchrow(
                """
                SELECT id, status, error, created_at, updated_at, completed_at
                FROM kb_ingestions
                WHERE id = $1 AND account_id = $2
                FOR UPDATE
                """,
                ingestion_id,
                db.account_id,
            )
            if not ingestion:
                raise HTTPException(status_code=404, detail="Knowledge ingestion not found")
            if ingestion["status"] in ("publishing", "complete"):
                return ingestion_payload(ingestion)
            if ingestion["status"] != "review_required":
                raise HTTPException(status_code=409, detail=f"Ingestion cannot be published while {ingestion['status']}")

            stored_concept_ids = set(
                await conn.fetchval(
                    "SELECT COALESCE(array_agg(id), ARRAY[]::uuid[]) FROM kb_ingestion_items WHERE ingestion_id = $1",
                    ingestion_id,
                )
            )
            stored_pattern_ids = set(
                await conn.fetchval(
                    "SELECT COALESCE(array_agg(id), ARRAY[]::uuid[]) FROM kb_ingestion_patterns WHERE ingestion_id = $1",
                    ingestion_id,
                )
            )
            submitted_concept_ids = {item.id for item in req.concepts}
            submitted_pattern_ids = {pattern.id for pattern in req.patterns}
            if submitted_concept_ids != stored_concept_ids or submitted_pattern_ids != stored_pattern_ids:
                raise HTTPException(status_code=409, detail="The ingestion drafts changed; reload them before publishing")
            if not any(item.approved for item in req.concepts) and not any(pattern.approved for pattern in req.patterns):
                raise HTTPException(status_code=422, detail="Approve at least one concept or pattern")

            for item in req.concepts:
                await conn.execute(
                    """
                    UPDATE kb_ingestion_items
                    SET type = $2, title = $3, tags = $4, body_markdown = $5,
                        status = $6, updated_at = NOW()
                    WHERE id = $1 AND ingestion_id = $7
                    """,
                    item.id,
                    item.type.strip(),
                    item.title.strip(),
                    item.tags,
                    item.body_markdown.strip(),
                    "approved" if item.approved else "rejected",
                    ingestion_id,
                )
            for pattern in req.patterns:
                triggers = list(dict.fromkeys(
                    phrase.lower().strip()
                    for phrase in pattern.trigger_phrases
                    if phrase.strip()
                ))
                canonical_trigger = pattern.canonical_question.lower().strip()
                if canonical_trigger not in triggers:
                    triggers.append(canonical_trigger)
                await conn.execute(
                    """
                    UPDATE kb_ingestion_patterns
                    SET canonical_question = $2, answer_markdown = $3, trigger_phrases = $4,
                        status = $5, updated_at = NOW()
                    WHERE id = $1 AND ingestion_id = $6
                    """,
                    pattern.id,
                    pattern.canonical_question.strip(),
                    pattern.answer_markdown.strip(),
                    triggers,
                    "approved" if pattern.approved else "rejected",
                    ingestion_id,
                )
            ingestion = await conn.fetchrow(
                """
                UPDATE kb_ingestions
                SET status = 'publishing', error = NULL, updated_at = NOW()
                WHERE id = $1
                RETURNING id, status, error, created_at, updated_at, completed_at
                """,
                ingestion_id,
            )
    return ingestion_payload(ingestion)

# Stage 4 — Paste-to-OKF Pipeline
@app.post("/internal/kb/compile-paste", response_model=CompilePasteResponse)
async def compile_paste(
    req: CompilePasteRequest,
    db: ScopedDB = Depends(get_db),
    x_user_id: Optional[str] = Header(None, alias="X-User-ID")
):
    actor_user_id = None
    if x_user_id:
        try:
            actor_user_id = uuid.UUID(x_user_id)
        except ValueError:
            pass

    api_key, base_url, completion_model, embedding_model = await get_ai_config(db)

    prompt = compilation_prompt(req.raw_text)
    result = await complete(api_key, base_url, completion_model, prompt, CompilePasteSchema)
    concepts = result.get("concepts", [])
    patterns = result.get("patterns", [])

    if not concepts and not patterns:
        return CompilePasteResponse(added_concepts=[], added_patterns=[])

    if len(concepts) + len(patterns) <= 3:
        added_concepts = []
        added_patterns = []
        for c in concepts:
            # Generate unique slug
            base_slug = slugify(c["title"]) or "concept"
            unique_slug = await get_unique_slug(db, base_slug)

            # Generate embedding
            text_to_embed = f"{c['title']}\n{c['body_markdown']}"
            vector = await embed(api_key, base_url, embedding_model, text_to_embed)

            # Insert
            row = await db.fetchrow(
                """
                INSERT INTO kb_concepts (account_id, slug, type, title, tags, body_markdown, embedding, source)
                VALUES ($1, $2, $3, $4, $5, $6, $7::vector, 'owner_pasted')
                RETURNING id, slug, type, title, tags, body_markdown, source, created_at, updated_at
                """,
                db.account_id,
                unique_slug,
                c["type"],
                c["title"],
                c["tags"],
                c["body_markdown"],
                str(vector)
            )

            record = dict(row)
            # Remove embedding from response since it is a large float array
            added_concepts.append(record)

            # Audit log
            await write_audit_log(
                db=db,
                actor_user_id=actor_user_id,
                action="kb_concept.created",
                target_type="kb_concept",
                target_id=record["id"],
                metadata={"title": c["title"], "slug": unique_slug, "source": "owner_pasted"}
            )

        for p in patterns:
            canonical_question = p["canonical_question"].strip()
            answer_markdown = p["answer_markdown"].strip()
            trigger_phrases = list(dict.fromkeys(
                phrase.lower().strip()
                for phrase in p.get("trigger_phrases", [])
                if phrase.strip()
            ))
            canonical_trigger = canonical_question.lower()
            if canonical_trigger not in trigger_phrases:
                trigger_phrases.append(canonical_trigger)
            vector = await embed(api_key, base_url, embedding_model, canonical_question)
            row = await db.fetchrow(
                """
                INSERT INTO patterns (account_id, canonical_question, answer_markdown, trigger_phrases, embedding)
                VALUES ($1, $2, $3, $4, $5::vector)
                RETURNING id, canonical_question, answer_markdown, trigger_phrases, created_at, updated_at
                """,
                db.account_id,
                canonical_question,
                answer_markdown,
                trigger_phrases,
                str(vector),
            )
            record = dict(row)
            added_patterns.append(record)
            await write_audit_log(
                db=db,
                actor_user_id=actor_user_id,
                action="pattern.created",
                target_type="pattern",
                target_id=record["id"],
                metadata={"canonical_question": canonical_question, "source": "owner_pasted"},
            )

        return CompilePasteResponse(added_concepts=added_concepts, added_patterns=added_patterns)

    else:
        # More than 3 concepts -> suggestion queue
        suggestion_ids = []
        for c in concepts:
            sugg_id = uuid.uuid4()
            await db.execute(
                """
                INSERT INTO automation_suggestions (id, account_id, type, proposed_payload, confidence, status)
                VALUES ($1, $2, 'new_kb_concept', $3, 1.0, 'pending')
                """,
                sugg_id,
                db.account_id,
                json.dumps(c)
            )
            suggestion_ids.append(str(sugg_id))
            await publish_suggestion_created(db.account_id, sugg_id, 'new_kb_concept', c)

            # Audit log
            await write_audit_log(
                db=db,
                actor_user_id=actor_user_id,
                action="automation_suggestion.created",
                target_type="automation_suggestion",
                target_id=sugg_id,
                metadata={"type": "new_kb_concept", "title": c["title"]}
            )

        for p in patterns:
            sugg_id = uuid.uuid4()
            await db.execute(
                """
                INSERT INTO automation_suggestions (id, account_id, type, proposed_payload, confidence, status)
                VALUES ($1, $2, 'new_pattern', $3, 1.0, 'pending')
                """,
                sugg_id,
                db.account_id,
                json.dumps(p),
            )
            suggestion_ids.append(str(sugg_id))
            await publish_suggestion_created(db.account_id, sugg_id, 'new_pattern', p)
            await write_audit_log(
                db=db,
                actor_user_id=actor_user_id,
                action="automation_suggestion.created",
                target_type="automation_suggestion",
                target_id=sugg_id,
                metadata={"type": "new_pattern", "canonical_question": p["canonical_question"]},
            )

        return CompilePasteResponse(suggestion_ids=suggestion_ids)

# Stage 5 — Concept Management
@app.get("/internal/kb/concepts")
async def list_concepts(db: ScopedDB = Depends(get_db)):
    rows = await db.fetch(
        """
        SELECT id, slug, type, title, tags, body_markdown, source, created_at, updated_at
        FROM kb_concepts
        WHERE account_id = $1
        ORDER BY created_at DESC
        """,
        db.account_id
    )
    return {"concepts": [dict(r) for r in rows]}

@app.delete("/internal/kb/concepts/{concept_id}")
async def delete_concept(
    concept_id: str,
    db: ScopedDB = Depends(get_db),
    x_user_id: Optional[str] = Header(None, alias="X-User-ID")
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

    # Check existence
    row = await db.fetchrow(
        "SELECT id, title, slug FROM kb_concepts WHERE id = $1 AND account_id = $2",
        concept_uuid, db.account_id
    )
    if not row:
        raise HTTPException(status_code=404, detail="Concept not found")

    await db.execute(
        "DELETE FROM kb_concepts WHERE id = $1 AND account_id = $2",
        concept_uuid, db.account_id
    )

    # Audit log
    await write_audit_log(
        db=db,
        actor_user_id=actor_user_id,
        action="kb_concept.deleted",
        target_type="kb_concept",
        target_id=concept_uuid,
        metadata={"title": row["title"], "slug": row["slug"]}
    )

    return {"success": True}

# Stage 6 — Dormant Mining
@app.post("/internal/kb/mine/trigger")
async def trigger_mine(
    db: ScopedDB = Depends(get_db)
):
    result = await run_mining(db)
    return result

# Stage 7 — Suggestion Review

class ApproveSuggestionRequest(BaseModel):
    reviewed_by: str
    edited_payload: Optional[dict] = None

class RejectSuggestionRequest(BaseModel):
    reviewed_by: str

@app.get("/internal/kb/suggestions")
async def list_suggestions(status_filter: str = "pending", db: ScopedDB = Depends(get_db)):
    rows = await db.fetch(
        """
        SELECT id, type, source_message_ids, proposed_payload, confidence, status, reviewed_by, reviewed_at, created_at
        FROM automation_suggestions
        WHERE account_id = $1 AND status = $2
        ORDER BY created_at DESC
        """,
        db.account_id,
        status_filter
    )
    return {"suggestions": [dict(r) for r in rows]}

@app.post("/internal/kb/suggestions/{suggestion_id}/approve")
async def approve_suggestion(
    suggestion_id: str,
    req: ApproveSuggestionRequest,
    db: ScopedDB = Depends(get_db)
):
    try:
        sugg_uuid = uuid.UUID(suggestion_id)
        reviewed_by_uuid = uuid.UUID(req.reviewed_by)
    except ValueError:
        raise HTTPException(status_code=400, detail="Invalid suggestion ID or user ID format")

    row = await db.fetchrow(
        "SELECT type, proposed_payload, status FROM automation_suggestions WHERE id = $1 AND account_id = $2",
        sugg_uuid, db.account_id
    )
    if not row:
        raise HTTPException(status_code=404, detail="Suggestion not found")
    if row["status"] != "pending":
        raise HTTPException(status_code=400, detail=f"Suggestion is already {row['status']}")

    proposed = row["proposed_payload"]
    payload = req.edited_payload if req.edited_payload is not None else (json.loads(proposed) if isinstance(proposed, str) else proposed)
    sugg_type = row["type"]


    api_key, base_url, _, embedding_model = await get_ai_config(db)

    # Approve depending on type
    if sugg_type == "new_kb_concept":
        title = payload.get("title")
        body_markdown = payload.get("body_markdown")
        c_type = payload.get("type", "faq")
        tags = payload.get("tags", [])

        if not title or not body_markdown:
            raise HTTPException(status_code=400, detail="Concept title and body_markdown are required")

        base_slug = slugify(title) or "concept"
        unique_slug = await get_unique_slug(db, base_slug)

        text_to_embed = f"{title}\n{body_markdown}"
        vector = await embed(api_key, base_url, embedding_model, text_to_embed)

        concept_id = uuid.uuid4()
        await db.execute(
            """
            INSERT INTO kb_concepts (id, account_id, slug, type, title, tags, body_markdown, embedding, source)
            VALUES ($1, $2, $3, $4, $5, $6, $7, $8::vector, 'owner_pasted')
            """,
            concept_id,
            db.account_id,
            unique_slug,
            c_type,
            title,
            tags,
            body_markdown,
            str(vector)
        )

        # Audit concept creation
        await write_audit_log(
            db=db,
            actor_user_id=reviewed_by_uuid,
            action="kb_concept.created",
            target_type="kb_concept",
            target_id=concept_id,
            metadata={"title": title, "slug": unique_slug, "source": "ai_compiled"}
        )

    elif sugg_type == "new_pattern":
        canonical_question = payload.get("canonical_question")
        answer_markdown = payload.get("answer_markdown")
        trigger_phrases = payload.get("trigger_phrases", [])

        if not canonical_question or not answer_markdown:
            raise HTTPException(status_code=400, detail="Pattern canonical_question and answer_markdown are required")

        text_to_embed = f"{canonical_question}\n{answer_markdown}"
        vector = await embed(api_key, base_url, embedding_model, text_to_embed)

        pattern_id = uuid.uuid4()
        await db.execute(
            """
            INSERT INTO patterns (id, account_id, trigger_phrases, canonical_question, answer_markdown, embedding)
            VALUES ($1, $2, $3, $4, $5, $6::vector)
            """,
            pattern_id,
            db.account_id,
            trigger_phrases,
            canonical_question,
            answer_markdown,
            str(vector)
        )

        # Audit pattern creation
        await write_audit_log(
            db=db,
            actor_user_id=reviewed_by_uuid,
            action="pattern.created",
            target_type="pattern",
            target_id=pattern_id,
            metadata={"canonical_question": canonical_question}
        )

    elif sugg_type == "edited_answer":
        # Edited answer case
        pattern_id_str = payload.get("pattern_id")
        answer_markdown = payload.get("answer_markdown")

        if not pattern_id_str or not answer_markdown:
            raise HTTPException(status_code=400, detail="pattern_id and answer_markdown are required")

        try:
            pattern_uuid = uuid.UUID(pattern_id_str)
        except ValueError:
            raise HTTPException(status_code=400, detail="Invalid pattern_id format")

        pattern_row = await db.fetchrow(
            "SELECT canonical_question FROM patterns WHERE id = $1 AND account_id = $2",
            pattern_uuid, db.account_id
        )
        if not pattern_row:
            raise HTTPException(status_code=404, detail="Pattern to edit not found")

        text_to_embed = f"{pattern_row['canonical_question']}\n{answer_markdown}"
        vector = await embed(api_key, base_url, embedding_model, text_to_embed)

        await db.execute(
            """
            UPDATE patterns
            SET answer_markdown = $1, embedding = $2::vector, updated_at = NOW()
            WHERE id = $3 AND account_id = $4
            """,
            answer_markdown,
            str(vector),
            pattern_uuid,
            db.account_id
        )

        # Audit pattern update
        await write_audit_log(
            db=db,
            actor_user_id=reviewed_by_uuid,
            action="pattern.updated",
            target_type="pattern",
            target_id=pattern_uuid,
            metadata={"canonical_question": pattern_row["canonical_question"]}
        )

    else:
        raise HTTPException(status_code=400, detail=f"Unsupported suggestion type: {sugg_type}")

    # Update suggestion status
    await db.execute(
        """
        UPDATE automation_suggestions
        SET status = 'approved', reviewed_by = $1, reviewed_at = NOW()
        WHERE id = $2 AND account_id = $3
        """,
        reviewed_by_uuid,
        sugg_uuid,
        db.account_id
    )

    # Audit suggestion approval
    await write_audit_log(
        db=db,
        actor_user_id=reviewed_by_uuid,
        action="automation_suggestion.approved",
        target_type="automation_suggestion",
        target_id=sugg_uuid,
        metadata={"type": sugg_type}
    )

    return {"success": True}

@app.post("/internal/kb/suggestions/{suggestion_id}/reject")
async def reject_suggestion(
    suggestion_id: str,
    req: RejectSuggestionRequest,
    db: ScopedDB = Depends(get_db)
):
    try:
        sugg_uuid = uuid.UUID(suggestion_id)
        reviewed_by_uuid = uuid.UUID(req.reviewed_by)
    except ValueError:
        raise HTTPException(status_code=400, detail="Invalid suggestion ID or user ID format")

    row = await db.fetchrow(
        "SELECT type, status FROM automation_suggestions WHERE id = $1 AND account_id = $2",
        sugg_uuid, db.account_id
    )
    if not row:
        raise HTTPException(status_code=404, detail="Suggestion not found")
    if row["status"] != "pending":
        raise HTTPException(status_code=400, detail=f"Suggestion is already {row['status']}")

    await db.execute(
        """
        UPDATE automation_suggestions
        SET status = 'rejected', reviewed_by = $1, reviewed_at = NOW()
        WHERE id = $2 AND account_id = $3
        """,
        reviewed_by_uuid,
        sugg_uuid,
        db.account_id
    )

    # Audit suggestion rejection
    await write_audit_log(
        db=db,
        actor_user_id=reviewed_by_uuid,
        action="automation_suggestion.rejected",
        target_type="automation_suggestion",
        target_id=sugg_uuid,
        metadata={"type": row["type"]}
    )

    return {"success": True}

@app.get("/internal/kb/patterns")
async def list_patterns(db: ScopedDB = Depends(get_db)):
    rows = await db.fetch(
        """
        SELECT id, canonical_question, answer_markdown, trigger_phrases, created_at, updated_at
        FROM patterns
        WHERE account_id = $1
        ORDER BY created_at DESC
        """,
        db.account_id
    )
    return {"patterns": [dict(r) for r in rows]}

@app.delete("/internal/kb/patterns/{pattern_id}")
async def delete_pattern(
    pattern_id: str,
    db: ScopedDB = Depends(get_db),
    x_user_id: Optional[str] = Header(None, alias="X-User-ID")
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

    row = await db.fetchrow(
        "SELECT id, canonical_question FROM patterns WHERE id = $1 AND account_id = $2",
        pattern_uuid, db.account_id
    )
    if not row:
        raise HTTPException(status_code=404, detail="Pattern not found")

    await db.execute(
        "DELETE FROM patterns WHERE id = $1 AND account_id = $2",
        pattern_uuid, db.account_id
    )

    await write_audit_log(
        db=db,
        actor_user_id=actor_user_id,
        action="pattern.deleted",
        target_type="pattern",
        target_id=pattern_uuid,
        metadata={"canonical_question": row["canonical_question"]}
    )

    return {"success": True}

@app.get("/internal/kb/mining-runs/latest")
async def latest_mining_run(db: ScopedDB = Depends(get_db)):
    row = await db.fetchrow(
        """
        SELECT run_at, window_start, window_end, messages_scanned, clusters_found, suggestions_created
        FROM kb_mining_runs
        WHERE account_id = $1
        ORDER BY run_at DESC
        LIMIT 1
        """,
        db.account_id
    )
    if not row:
        return {"last_run": None}
    record = dict(row)
    for key, val in record.items():
        if isinstance(val, datetime):
            record[key] = val.isoformat()
    return {"last_run": record}
