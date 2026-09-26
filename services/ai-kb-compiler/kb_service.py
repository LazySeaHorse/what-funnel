import json
import logging
import uuid
from datetime import datetime
from typing import Any, Callable, List, Optional

from fastapi import HTTPException

from audit import write_audit_log
from db import ScopedDB
from ingestions import compilation_prompt
from llm import get_ai_config, provider_client
from redis_client import publish_suggestion_created
from schemas import (
    CompilePasteResponse,
    CompilePasteSchema,
    PublishIngestionItem,
    PublishIngestionPattern,
    UpdateConceptRequest,
    UpdatePatternRequest,
)
from slug import get_unique_slug, slugify

logger = logging.getLogger("ai-kb-compiler")


def ingestion_payload(row, concepts=(), patterns=()) -> dict[str, Any]:
    """Format an ingestion database row along with concepts and patterns."""
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


# ===========================================================================
# Ingestions
# ===========================================================================

async def create_ingestion(
    db: ScopedDB,
    raw_text: str,
    x_user_id: Optional[str] = None
) -> dict[str, Any]:
    cleaned_text = raw_text.strip()
    if not cleaned_text:
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
        cleaned_text,
    )
    return ingestion_payload(row)


async def get_latest_ingestion(db: ScopedDB) -> Optional[dict[str, Any]]:
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
        return None

    concepts = await db.fetch(
        """
        SELECT id, position, type, title, tags, body_text, status, concept_id
        FROM kb_ingestion_items
        WHERE ingestion_id = $1
        ORDER BY position
        """,
        row["id"],
    )
    patterns = await db.fetch(
        """
        SELECT id, position, canonical_question, answer_text, trigger_phrases, status, pattern_id
        FROM kb_ingestion_patterns
        WHERE ingestion_id = $1
        ORDER BY position
        """,
        row["id"],
    )
    return ingestion_payload(row, concepts, patterns)


async def get_ingestion(db: ScopedDB, ingestion_id: uuid.UUID) -> dict[str, Any]:
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
        SELECT id, position, type, title, tags, body_text, status, concept_id
        FROM kb_ingestion_items
        WHERE ingestion_id = $1
        ORDER BY position
        """,
        ingestion_id,
    )
    patterns = await db.fetch(
        """
        SELECT id, position, canonical_question, answer_text, trigger_phrases, status, pattern_id
        FROM kb_ingestion_patterns
        WHERE ingestion_id = $1
        ORDER BY position
        """,
        ingestion_id,
    )
    return ingestion_payload(row, concepts, patterns)


async def publish_ingestion(
    db: ScopedDB,
    ingestion_id: uuid.UUID,
    concepts: List[PublishIngestionItem],
    patterns: List[PublishIngestionPattern],
) -> dict[str, Any]:
    if len({item.id for item in concepts}) != len(concepts):
        raise HTTPException(status_code=422, detail="Each ingestion concept may only be submitted once")
    if len({pattern.id for pattern in patterns}) != len(patterns):
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
                raise HTTPException(
                    status_code=409,
                    detail=f"Ingestion cannot be published while {ingestion['status']}",
                )

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
            submitted_concept_ids = {item.id for item in concepts}
            submitted_pattern_ids = {pattern.id for pattern in patterns}
            if submitted_concept_ids != stored_concept_ids or submitted_pattern_ids != stored_pattern_ids:
                raise HTTPException(
                    status_code=409,
                    detail="The ingestion drafts changed; reload them before publishing",
                )
            if not any(item.approved for item in concepts) and not any(pattern.approved for pattern in patterns):
                raise HTTPException(status_code=422, detail="Approve at least one concept or pattern")

            for item in concepts:
                await conn.execute(
                    """
                    UPDATE kb_ingestion_items
                    SET type = $2, title = $3, tags = $4, body_text = $5,
                        status = $6, updated_at = NOW()
                    WHERE id = $1 AND ingestion_id = $7
                    """,
                    item.id,
                    item.type.strip(),
                    item.title.strip(),
                    item.tags,
                    item.body_text.strip(),
                    "approved" if item.approved else "rejected",
                    ingestion_id,
                )
            for pattern in patterns:
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
                    SET canonical_question = $2, answer_text = $3, trigger_phrases = $4,
                        status = $5, updated_at = NOW()
                    WHERE id = $1 AND ingestion_id = $6
                    """,
                    pattern.id,
                    pattern.canonical_question.strip(),
                    pattern.answer_text.strip(),
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


# ===========================================================================
# Paste Compilation
# ===========================================================================

async def compile_paste(
    db: ScopedDB,
    raw_text: str,
    actor_user_id: Optional[uuid.UUID] = None,
    client_factory: Callable = provider_client,
) -> CompilePasteResponse:
    config = await get_ai_config(db)
    client = client_factory(config)

    prompt = compilation_prompt(raw_text)
    result = await client.complete(
        config.analysis_model,
        [{"role": "user", "content": prompt}],
        CompilePasteSchema,
    )
    concepts = result.get("concepts", [])
    patterns = result.get("patterns", [])

    if not concepts and not patterns:
        return CompilePasteResponse(added_concepts=[], added_patterns=[])

    if len(concepts) + len(patterns) <= 3:
        added_concepts = []
        added_patterns = []
        for c in concepts:
            base_slug = slugify(c["title"]) or "concept"
            unique_slug = await get_unique_slug(db, base_slug)

            text_to_embed = f"{c['title']}\n{c['body_text']}"
            vector = await client.embed(config.embedding_model, text_to_embed)

            row = await db.fetchrow(
                """
                INSERT INTO kb_concepts (account_id, slug, type, title, tags, body_text, embedding, source)
                VALUES ($1, $2, $3, $4, $5, $6, $7::vector, 'owner_pasted')
                RETURNING id, slug, type, title, tags, body_text, source, created_at, updated_at
                """,
                db.account_id,
                unique_slug,
                c["type"],
                c["title"],
                c["tags"],
                c["body_text"],
                str(vector),
            )

            record = dict(row)
            added_concepts.append(record)

            await write_audit_log(
                db=db,
                actor_user_id=actor_user_id,
                action="kb_concept.created",
                target_type="kb_concept",
                target_id=record["id"],
                metadata={"title": c["title"], "slug": unique_slug, "source": "owner_pasted"},
            )

        for p in patterns:
            canonical_question = p["canonical_question"].strip()
            answer_text = p["answer_text"].strip()
            trigger_phrases = list(dict.fromkeys(
                phrase.lower().strip()
                for phrase in p.get("trigger_phrases", [])
                if phrase.strip()
            ))
            canonical_trigger = canonical_question.lower()
            if canonical_trigger not in trigger_phrases:
                trigger_phrases.append(canonical_trigger)
            vector = await client.embed(config.embedding_model, canonical_question)
            row = await db.fetchrow(
                """
                INSERT INTO patterns (account_id, canonical_question, answer_text, trigger_phrases, embedding)
                VALUES ($1, $2, $3, $4, $5::vector)
                RETURNING id, canonical_question, answer_text, trigger_phrases, created_at, updated_at
                """,
                db.account_id,
                canonical_question,
                answer_text,
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
        # More than 3 concepts/patterns -> suggestion queue
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
                json.dumps(c),
            )
            suggestion_ids.append(str(sugg_id))
            await publish_suggestion_created(db.account_id, sugg_id, "new_kb_concept", c)

            await write_audit_log(
                db=db,
                actor_user_id=actor_user_id,
                action="automation_suggestion.created",
                target_type="automation_suggestion",
                target_id=sugg_id,
                metadata={"type": "new_kb_concept", "title": c["title"]},
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
            await publish_suggestion_created(db.account_id, sugg_id, "new_pattern", p)
            await write_audit_log(
                db=db,
                actor_user_id=actor_user_id,
                action="automation_suggestion.created",
                target_type="automation_suggestion",
                target_id=sugg_id,
                metadata={"type": "new_pattern", "canonical_question": p["canonical_question"]},
            )

        return CompilePasteResponse(suggestion_ids=suggestion_ids)


# ===========================================================================
# Concept Management
# ===========================================================================

async def list_concepts(db: ScopedDB) -> List[dict[str, Any]]:
    rows = await db.fetch(
        """
        SELECT id, slug, type, title, tags, body_text, source, created_at, updated_at
        FROM kb_concepts
        WHERE account_id = $1
        ORDER BY created_at DESC
        """,
        db.account_id,
    )
    return [dict(r) for r in rows]


async def purge_knowledge_base(
    db: ScopedDB,
    actor_user_id: Optional[uuid.UUID] = None
) -> dict[str, Any]:
    async with db.pool.acquire() as conn:
        async with conn.transaction():
            active_ingestion = await conn.fetchval(
                """
                SELECT id FROM kb_ingestions
                WHERE account_id = $1
                  AND status IN ('queued', 'processing', 'review_required', 'publishing')
                ORDER BY created_at DESC
                LIMIT 1
                FOR UPDATE
                """,
                db.account_id,
            )
            if active_ingestion:
                raise HTTPException(
                    status_code=409,
                    detail="Finish the active knowledge ingestion before purging the knowledge base.",
                )

            if actor_user_id:
                user_exists = await conn.fetchval(
                    "SELECT 1 FROM users WHERE id = $1 AND account_id = $2",
                    actor_user_id,
                    db.account_id,
                )
                if not user_exists:
                    actor_user_id = None

            cleared_concepts = await conn.fetchval(
                """
                WITH deleted AS (
                    DELETE FROM kb_concepts WHERE account_id = $1 RETURNING 1
                )
                SELECT count(*) FROM deleted
                """,
                db.account_id,
            )
            cleared_patterns = await conn.fetchval(
                """
                WITH deleted AS (
                    DELETE FROM patterns WHERE account_id = $1 RETURNING 1
                )
                SELECT count(*) FROM deleted
                """,
                db.account_id,
            )
            await conn.execute(
                """
                INSERT INTO audit_logs (account_id, actor_user_id, action, target_type, metadata)
                VALUES ($1, $2, 'knowledge_base.purged', 'knowledge_base', $3)
                """,
                db.account_id,
                actor_user_id,
                json.dumps({
                    "cleared_concepts": cleared_concepts,
                    "cleared_patterns": cleared_patterns,
                }),
            )

    return {
        "success": True,
        "cleared_concepts": cleared_concepts,
        "cleared_patterns": cleared_patterns,
    }


async def delete_concept(
    db: ScopedDB,
    concept_uuid: uuid.UUID,
    actor_user_id: Optional[uuid.UUID] = None
) -> None:
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

    await write_audit_log(
        db=db,
        actor_user_id=actor_user_id,
        action="kb_concept.deleted",
        target_type="kb_concept",
        target_id=concept_uuid,
        metadata={"title": row["title"], "slug": row["slug"]},
    )


async def update_concept(
    db: ScopedDB,
    concept_uuid: uuid.UUID,
    req: UpdateConceptRequest,
    actor_user_id: Optional[uuid.UUID] = None,
    client_factory: Callable = provider_client,
) -> dict[str, Any]:
    row = await db.fetchrow(
        "SELECT id, title, slug, type, tags, body_text FROM kb_concepts WHERE id = $1 AND account_id = $2",
        concept_uuid, db.account_id
    )
    if not row:
        raise HTTPException(status_code=404, detail="Concept not found")

    new_title = req.title if req.title is not None else row["title"]
    new_type = req.type if req.type is not None else row["type"]
    new_body = req.body_text if req.body_text is not None else row["body_text"]
    new_tags = req.tags if req.tags is not None else row["tags"]

    vector_str = None
    try:
        cfg = await get_ai_config(db)
        if cfg and cfg.embedding_model:
            cli = client_factory(cfg)
            text_to_embed = f"{new_title}\n{new_body}"
            vector = await cli.embed(cfg.embedding_model, text_to_embed)
            vector_str = str(vector)
    except Exception as e:
        logger.warning(f"Could not regenerate embedding for concept {concept_uuid}: {e}")

    if vector_str:
        updated = await db.fetchrow(
            """
            UPDATE kb_concepts
            SET title = $1, type = $2, body_text = $3, tags = $4, embedding = $5::vector, updated_at = NOW()
            WHERE id = $6 AND account_id = $7
            RETURNING id, slug, type, title, tags, body_text, source, created_at, updated_at
            """,
            new_title, new_type, new_body, new_tags, vector_str, concept_uuid, db.account_id
        )
    else:
        updated = await db.fetchrow(
            """
            UPDATE kb_concepts
            SET title = $1, type = $2, body_text = $3, tags = $4, updated_at = NOW()
            WHERE id = $5 AND account_id = $6
            RETURNING id, slug, type, title, tags, body_text, source, created_at, updated_at
            """,
            new_title, new_type, new_body, new_tags, concept_uuid, db.account_id
        )

    await write_audit_log(
        db=db,
        actor_user_id=actor_user_id,
        action="kb_concept.updated",
        target_type="kb_concept",
        target_id=concept_uuid,
        metadata={"title": new_title, "slug": row["slug"]},
    )

    return dict(updated)


# ===========================================================================
# Pattern Management
# ===========================================================================

async def list_patterns(db: ScopedDB) -> List[dict[str, Any]]:
    rows = await db.fetch(
        """
        SELECT id, canonical_question, answer_text, trigger_phrases, created_at, updated_at
        FROM patterns
        WHERE account_id = $1
        ORDER BY created_at DESC
        """,
        db.account_id,
    )
    return [dict(r) for r in rows]


async def delete_pattern(
    db: ScopedDB,
    pattern_uuid: uuid.UUID,
    actor_user_id: Optional[uuid.UUID] = None
) -> None:
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
        metadata={"canonical_question": row["canonical_question"]},
    )


async def update_pattern(
    db: ScopedDB,
    pattern_uuid: uuid.UUID,
    req: UpdatePatternRequest,
    actor_user_id: Optional[uuid.UUID] = None,
    client_factory: Callable = provider_client,
) -> dict[str, Any]:
    row = await db.fetchrow(
        "SELECT id, canonical_question, answer_text, trigger_phrases FROM patterns WHERE id = $1 AND account_id = $2",
        pattern_uuid, db.account_id
    )
    if not row:
        raise HTTPException(status_code=404, detail="Pattern not found")

    new_question = req.canonical_question if req.canonical_question is not None else row["canonical_question"]
    new_answer = req.answer_text if req.answer_text is not None else row["answer_text"]
    new_triggers = req.trigger_phrases if req.trigger_phrases is not None else row["trigger_phrases"]

    vector_str = None
    try:
        cfg = await get_ai_config(db)
        if cfg and cfg.embedding_model:
            cli = client_factory(cfg)
            text_to_embed = f"{new_question}\n{new_answer}"
            vector = await cli.embed(cfg.embedding_model, text_to_embed)
            vector_str = str(vector)
    except Exception as e:
        logger.warning(f"Could not regenerate embedding for pattern {pattern_uuid}: {e}")

    if vector_str:
        updated = await db.fetchrow(
            """
            UPDATE patterns
            SET canonical_question = $1, answer_text = $2, trigger_phrases = $3, embedding = $4::vector, updated_at = NOW()
            WHERE id = $5 AND account_id = $6
            RETURNING id, canonical_question, answer_text, trigger_phrases, created_at, updated_at
            """,
            new_question, new_answer, new_triggers, vector_str, pattern_uuid, db.account_id
        )
    else:
        updated = await db.fetchrow(
            """
            UPDATE patterns
            SET canonical_question = $1, answer_text = $2, trigger_phrases = $3, updated_at = NOW()
            WHERE id = $4 AND account_id = $5
            RETURNING id, canonical_question, answer_text, trigger_phrases, created_at, updated_at
            """,
            new_question, new_answer, new_triggers, pattern_uuid, db.account_id
        )

    await write_audit_log(
        db=db,
        actor_user_id=actor_user_id,
        action="pattern.updated",
        target_type="pattern",
        target_id=pattern_uuid,
        metadata={"canonical_question": new_question},
    )

    return dict(updated)


# ===========================================================================
# Suggestion Review
# ===========================================================================

async def list_suggestions(db: ScopedDB, status_filter: str = "pending") -> List[dict[str, Any]]:
    rows = await db.fetch(
        """
        SELECT id, type, source_message_ids, proposed_payload, confidence, status, reviewed_by, reviewed_at, created_at
        FROM automation_suggestions
        WHERE account_id = $1 AND status = $2
        ORDER BY created_at DESC
        """,
        db.account_id,
        status_filter,
    )
    return [dict(r) for r in rows]


async def approve_suggestion(
    db: ScopedDB,
    sugg_uuid: uuid.UUID,
    reviewed_by_uuid: uuid.UUID,
    edited_payload: Optional[dict] = None,
    client_factory: Callable = provider_client,
) -> None:
    row = await db.fetchrow(
        "SELECT type, proposed_payload, status FROM automation_suggestions WHERE id = $1 AND account_id = $2",
        sugg_uuid, db.account_id
    )
    if not row:
        raise HTTPException(status_code=404, detail="Suggestion not found")
    if row["status"] != "pending":
        raise HTTPException(status_code=400, detail=f"Suggestion is already {row['status']}")

    proposed = row["proposed_payload"]
    payload = edited_payload if edited_payload is not None else (
        json.loads(proposed) if isinstance(proposed, str) else proposed
    )
    sugg_type = row["type"]

    config = await get_ai_config(db)
    client = client_factory(config)

    if sugg_type == "new_kb_concept":
        title = payload.get("title")
        body_text = payload.get("body_text")
        c_type = payload.get("type", "faq")
        tags = payload.get("tags", [])

        if not title or not body_text:
            raise HTTPException(status_code=400, detail="Concept title and body_text are required")

        base_slug = slugify(title) or "concept"
        unique_slug = await get_unique_slug(db, base_slug)

        text_to_embed = f"{title}\n{body_text}"
        vector = await client.embed(config.embedding_model, text_to_embed)

        concept_id = uuid.uuid4()
        await db.execute(
            """
            INSERT INTO kb_concepts (id, account_id, slug, type, title, tags, body_text, embedding, source)
            VALUES ($1, $2, $3, $4, $5, $6, $7, $8::vector, 'owner_pasted')
            """,
            concept_id,
            db.account_id,
            unique_slug,
            c_type,
            title,
            tags,
            body_text,
            str(vector),
        )

        await write_audit_log(
            db=db,
            actor_user_id=reviewed_by_uuid,
            action="kb_concept.created",
            target_type="kb_concept",
            target_id=concept_id,
            metadata={"title": title, "slug": unique_slug, "source": "ai_compiled"},
        )

    elif sugg_type == "new_pattern":
        canonical_question = payload.get("canonical_question")
        answer_text = payload.get("answer_text")
        trigger_phrases = payload.get("trigger_phrases", [])

        if not canonical_question or not answer_text:
            raise HTTPException(status_code=400, detail="Pattern canonical_question and answer_text are required")

        text_to_embed = f"{canonical_question}\n{answer_text}"
        vector = await client.embed(config.embedding_model, text_to_embed)

        pattern_id = uuid.uuid4()
        await db.execute(
            """
            INSERT INTO patterns (id, account_id, trigger_phrases, canonical_question, answer_text, embedding)
            VALUES ($1, $2, $3, $4, $5, $6::vector)
            """,
            pattern_id,
            db.account_id,
            trigger_phrases,
            canonical_question,
            answer_text,
            str(vector),
        )

        await write_audit_log(
            db=db,
            actor_user_id=reviewed_by_uuid,
            action="pattern.created",
            target_type="pattern",
            target_id=pattern_id,
            metadata={"canonical_question": canonical_question},
        )

    elif sugg_type == "edited_answer":
        pattern_id_str = payload.get("pattern_id")
        answer_text = payload.get("answer_text")

        if not pattern_id_str or not answer_text:
            raise HTTPException(status_code=400, detail="pattern_id and answer_text are required")

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

        text_to_embed = f"{pattern_row['canonical_question']}\n{answer_text}"
        vector = await client.embed(config.embedding_model, text_to_embed)

        await db.execute(
            """
            UPDATE patterns
            SET answer_text = $1, embedding = $2::vector, updated_at = NOW()
            WHERE id = $3 AND account_id = $4
            """,
            answer_text,
            str(vector),
            pattern_uuid,
            db.account_id,
        )

        await write_audit_log(
            db=db,
            actor_user_id=reviewed_by_uuid,
            action="pattern.updated",
            target_type="pattern",
            target_id=pattern_uuid,
            metadata={"canonical_question": pattern_row["canonical_question"]},
        )

    else:
        raise HTTPException(status_code=400, detail=f"Unsupported suggestion type: {sugg_type}")

    await db.execute(
        """
        UPDATE automation_suggestions
        SET status = 'approved', reviewed_by = $1, reviewed_at = NOW()
        WHERE id = $2 AND account_id = $3
        """,
        reviewed_by_uuid,
        sugg_uuid,
        db.account_id,
    )

    await write_audit_log(
        db=db,
        actor_user_id=reviewed_by_uuid,
        action="automation_suggestion.approved",
        target_type="automation_suggestion",
        target_id=sugg_uuid,
        metadata={"type": sugg_type},
    )


async def reject_suggestion(
    db: ScopedDB,
    sugg_uuid: uuid.UUID,
    reviewed_by_uuid: uuid.UUID
) -> None:
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
        db.account_id,
    )

    await write_audit_log(
        db=db,
        actor_user_id=reviewed_by_uuid,
        action="automation_suggestion.rejected",
        target_type="automation_suggestion",
        target_id=sugg_uuid,
        metadata={"type": row["type"]},
    )


# ===========================================================================
# Mining Runs
# ===========================================================================

async def latest_mining_run(db: ScopedDB) -> Optional[dict[str, Any]]:
    row = await db.fetchrow(
        """
        SELECT run_at, window_start, window_end, messages_scanned, clusters_found, suggestions_created
        FROM kb_mining_runs
        WHERE account_id = $1
        ORDER BY run_at DESC
        LIMIT 1
        """,
        db.account_id,
    )
    if not row:
        return None
    record = dict(row)
    for key, val in record.items():
        if isinstance(val, datetime):
            record[key] = val.isoformat()
    return record
