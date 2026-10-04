import asyncio
import json
import logging
import uuid
from datetime import datetime
from typing import Any, Callable, List, Optional

from fastapi import HTTPException
from pydantic import BaseModel, ValidationError

from audit import write_audit_log
from db import ScopedDB
from ingestions import compilation_prompt
from llm import get_ai_config, provider_client
from phrases import normalize_not_for, normalize_trigger_phrases
from redis_client import publish_suggestion_created
from schemas import (
    CompilePasteResponse,
    CompilePasteSchema,
    PublishIngestionItem,
    PublishIngestionPattern,
    SuggestionConceptPayload,
    SuggestionEditedAnswerPayload,
    SuggestionPatternPayload,
    UpdateConceptRequest,
    UpdatePatternRequest,
)
from slug import concept_base_slug, get_unique_slug, lock_concept_slugs

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
    actor_user_id: Optional[uuid.UUID] = None,
) -> dict[str, Any]:
    cleaned_text = raw_text.strip()
    if not cleaned_text:
        raise HTTPException(status_code=422, detail="raw_text must not be blank")

    requested_by = None
    if actor_user_id and await db.fetchval(
        "SELECT 1 FROM users WHERE id = $1 AND account_id = $2",
        actor_user_id,
        db.account_id,
    ):
        requested_by = actor_user_id

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
        SELECT id, position, canonical_question, answer_text, trigger_phrases, not_for, status, pattern_id
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
        SELECT id, position, canonical_question, answer_text, trigger_phrases, not_for, status, pattern_id
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
                triggers = normalize_trigger_phrases(pattern.trigger_phrases, pattern.canonical_question)
                await conn.execute(
                    """
                    UPDATE kb_ingestion_patterns
                    SET canonical_question = $2, answer_text = $3, trigger_phrases = $4,
                        status = $5, not_for = $7, updated_at = NOW()
                    WHERE id = $1 AND ingestion_id = $6
                    """,
                    pattern.id,
                    pattern.canonical_question.strip(),
                    pattern.answer_text.strip(),
                    triggers,
                    "approved" if pattern.approved else "rejected",
                    ingestion_id,
                    normalize_not_for(pattern.not_for),
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

    # FAQs (patterns) are answered verbatim to customers, so they are NEVER stored directly:
    # every pattern becomes a pending suggestion that a human approves in the knowledge panel.
    # Concepts (retrieval material for grounded answers) are stored directly when there are few.
    queue_concepts = len(concepts) > 3
    direct_concepts = [] if queue_concepts else concepts

    # Provider calls happen before the transaction so no locks are held over the network.
    concept_vectors = await asyncio.gather(*[
        client.embed(config.embedding_model, f"{c['title']}\n{c['body_text']}") for c in direct_concepts
    ])

    proposals = [
        ("new_pattern", {**p, "not_for": normalize_not_for(p.get("not_for", ""))}, {"canonical_question": p["canonical_question"]})
        for p in patterns
    ]
    if queue_concepts:
        proposals += [("new_kb_concept", c, {"title": c["title"]}) for c in concepts]

    added_concepts = []
    created: list[tuple[uuid.UUID, str, dict]] = []
    async with db.transaction() as tx:
        await lock_concept_slugs(tx)
        for c, vector in zip(direct_concepts, concept_vectors):
            unique_slug = await get_unique_slug(tx, concept_base_slug(c["title"]))
            row = await tx.fetchrow(
                """
                INSERT INTO kb_concepts (account_id, slug, type, title, tags, body_text, embedding, source)
                VALUES ($1, $2, $3, $4, $5, $6, $7::vector, 'owner_pasted')
                RETURNING id, slug, type, title, tags, body_text, source, created_at, updated_at
                """,
                tx.account_id,
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
                db=tx,
                actor_user_id=actor_user_id,
                action="kb_concept.created",
                target_type="kb_concept",
                target_id=record["id"],
                metadata={"title": c["title"], "slug": unique_slug, "source": "owner_pasted"},
            )

        for sugg_type, payload, audit_extra in proposals:
            sugg_id = uuid.uuid4()
            await tx.execute(
                """
                INSERT INTO automation_suggestions (id, account_id, type, proposed_payload, confidence, status)
                VALUES ($1, $2, $3, $4, 1.0, 'pending')
                """,
                sugg_id,
                tx.account_id,
                sugg_type,
                json.dumps(payload),
            )
            await write_audit_log(
                db=tx,
                actor_user_id=actor_user_id,
                action="automation_suggestion.created",
                target_type="automation_suggestion",
                target_id=sugg_id,
                metadata={"type": sugg_type, **audit_extra},
            )
            created.append((sugg_id, sugg_type, payload))

    # Notify only after the suggestions are committed.
    for sugg_id, sugg_type, payload in created:
        await publish_suggestion_created(db.account_id, sugg_id, sugg_type, payload)

    return CompilePasteResponse(
        added_concepts=added_concepts or None,
        added_patterns=None,
        suggestion_ids=[str(sugg_id) for sugg_id, _, _ in created] or None,
    )


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


async def _embed_or_fail(
    db: ScopedDB, client_factory: Callable, text: str, what: str, entity_id: uuid.UUID
) -> str:
    """Embed `text` for a stored row. A failed embedding fails the request: silently keeping the
    old vector would leave retrieval answering from stale content while reporting success."""
    cfg = await get_ai_config(db)  # raises HTTPException(409/500) when no provider is configured
    try:
        vector = await client_factory(cfg).embed(cfg.embedding_model, text)
    except Exception as e:
        logger.error(f"Could not regenerate embedding for {what} {entity_id}: {e}")
        raise HTTPException(
            status_code=502,
            detail=f"Could not regenerate the {what} embedding; nothing was changed. Try again.",
        ) from e
    return str(vector)


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

    if new_title != row["title"] or new_body != row["body_text"]:
        vector_str = await _embed_or_fail(
            db, client_factory, f"{new_title}\n{new_body}", "concept", concept_uuid
        )
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
    if not updated:
        raise HTTPException(status_code=404, detail="Concept not found")

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
        SELECT id, canonical_question, answer_text, trigger_phrases, not_for, approved_at, created_at, updated_at
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
    """Owner edit of an approved FAQ. FAQs are no longer embedded, so no provider call is needed."""
    row = await db.fetchrow(
        "SELECT id, canonical_question, answer_text, trigger_phrases, not_for FROM patterns WHERE id = $1 AND account_id = $2",
        pattern_uuid, db.account_id
    )
    if not row:
        raise HTTPException(status_code=404, detail="Pattern not found")

    new_question = req.canonical_question if req.canonical_question is not None else row["canonical_question"]
    new_answer = req.answer_text if req.answer_text is not None else row["answer_text"]
    new_triggers = (
        normalize_trigger_phrases(req.trigger_phrases)
        if req.trigger_phrases is not None
        else row["trigger_phrases"]
    )
    new_not_for = normalize_not_for(req.not_for) if req.not_for is not None else row["not_for"]

    updated = await db.fetchrow(
        """
        UPDATE patterns
        SET canonical_question = $1, answer_text = $2, trigger_phrases = $3, not_for = $4, updated_at = NOW()
        WHERE id = $5 AND account_id = $6
        RETURNING id, canonical_question, answer_text, trigger_phrases, not_for, approved_at, created_at, updated_at
        """,
        new_question, new_answer, new_triggers, new_not_for, pattern_uuid, db.account_id
    )
    if not updated:
        raise HTTPException(status_code=404, detail="Pattern not found")

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


_SUGGESTION_PAYLOAD_MODELS: dict[str, type[BaseModel]] = {
    "new_kb_concept": SuggestionConceptPayload,
    "new_pattern": SuggestionPatternPayload,
    "edited_answer": SuggestionEditedAnswerPayload,
}


def _validated_suggestion_payload(sugg_type: str, raw: Any) -> BaseModel:
    model = _SUGGESTION_PAYLOAD_MODELS.get(sugg_type)
    if model is None:
        raise HTTPException(status_code=400, detail=f"Unsupported suggestion type: {sugg_type}")
    if not isinstance(raw, dict):
        raise HTTPException(status_code=422, detail="Suggestion payload must be a JSON object")
    try:
        return model.model_validate(raw)
    except ValidationError as e:
        raise HTTPException(status_code=422, detail=json.loads(e.json(include_url=False, include_input=False)))


async def approve_suggestion(
    db: ScopedDB,
    sugg_uuid: uuid.UUID,
    reviewed_by_uuid: uuid.UUID,
    edited_payload: Optional[dict] = None,
    client_factory: Callable = provider_client,
) -> None:
    row = await db.fetchrow(
        """
        SELECT type, proposed_payload, status, source_message_ids
        FROM automation_suggestions WHERE id = $1 AND account_id = $2
        """,
        sugg_uuid, db.account_id
    )
    if not row:
        raise HTTPException(status_code=404, detail="Suggestion not found")
    if row["status"] != "pending":
        raise HTTPException(status_code=400, detail=f"Suggestion is already {row['status']}")

    proposed = row["proposed_payload"]
    if edited_payload is not None:
        raw_payload = edited_payload
    else:
        try:
            raw_payload = json.loads(proposed) if isinstance(proposed, (str, bytes)) else proposed
        except ValueError:
            raise HTTPException(status_code=422, detail="Stored suggestion payload is not valid JSON")
    sugg_type = row["type"]
    payload = _validated_suggestion_payload(sugg_type, raw_payload)
    # Suggestions mined from conversations reference their messages; pasted ones do not.
    concept_source = "ai_compiled" if row["source_message_ids"] else "owner_pasted"

    # Only concepts are embedded (retrieval for grounded answers). FAQs are shown to the router as text.
    vector = None
    if sugg_type == "new_kb_concept":
        config = await get_ai_config(db)
        client = client_factory(config)
        vector = await client.embed(config.embedding_model, f"{payload.title}\n{payload.body_text}")
    elif sugg_type == "edited_answer":
        pattern_row = await db.fetchrow(
            "SELECT canonical_question FROM patterns WHERE id = $1 AND account_id = $2",
            payload.pattern_id, db.account_id
        )
        if not pattern_row:
            raise HTTPException(status_code=404, detail="Pattern to edit not found")

    async with db.transaction() as tx:
        # Claim the suggestion: a concurrent approval waits here, then sees it is no longer pending.
        locked = await tx.fetchrow(
            "SELECT status FROM automation_suggestions WHERE id = $1 AND account_id = $2 FOR UPDATE",
            sugg_uuid, tx.account_id
        )
        if not locked:
            raise HTTPException(status_code=404, detail="Suggestion not found")
        if locked["status"] != "pending":
            raise HTTPException(status_code=400, detail=f"Suggestion is already {locked['status']}")

        if sugg_type == "new_kb_concept":
            await lock_concept_slugs(tx)
            unique_slug = await get_unique_slug(tx, concept_base_slug(payload.title))
            concept_id = uuid.uuid4()
            await tx.execute(
                """
                INSERT INTO kb_concepts (id, account_id, slug, type, title, tags, body_text, embedding, source)
                VALUES ($1, $2, $3, $4, $5, $6, $7, $8::vector, $9)
                """,
                concept_id,
                tx.account_id,
                unique_slug,
                payload.type,
                payload.title,
                payload.tags,
                payload.body_text,
                str(vector),
                concept_source,
            )
            await write_audit_log(
                db=tx,
                actor_user_id=reviewed_by_uuid,
                action="kb_concept.created",
                target_type="kb_concept",
                target_id=concept_id,
                metadata={"title": payload.title, "slug": unique_slug, "source": concept_source},
            )

        elif sugg_type == "new_pattern":
            pattern_id = uuid.uuid4()
            await tx.execute(
                """
                INSERT INTO patterns (
                    id, account_id, trigger_phrases, canonical_question, answer_text, not_for,
                    approved_at, approved_by
                )
                VALUES ($1, $2, $3, $4, $5, $6, NOW(), $7)
                """,
                pattern_id,
                tx.account_id,
                normalize_trigger_phrases(payload.trigger_phrases, payload.canonical_question),
                payload.canonical_question,
                payload.answer_text,
                normalize_not_for(payload.not_for),
                reviewed_by_uuid,
            )
            await write_audit_log(
                db=tx,
                actor_user_id=reviewed_by_uuid,
                action="pattern.created",
                target_type="pattern",
                target_id=pattern_id,
                metadata={"canonical_question": payload.canonical_question},
            )

        else:  # edited_answer
            updated_pattern = await tx.fetchrow(
                """
                UPDATE patterns
                SET answer_text = $1, updated_at = NOW()
                WHERE id = $2 AND account_id = $3
                RETURNING canonical_question
                """,
                payload.answer_text,
                payload.pattern_id,
                tx.account_id,
            )
            if not updated_pattern:
                raise HTTPException(status_code=404, detail="Pattern to edit not found")
            await write_audit_log(
                db=tx,
                actor_user_id=reviewed_by_uuid,
                action="pattern.updated",
                target_type="pattern",
                target_id=payload.pattern_id,
                metadata={"canonical_question": updated_pattern["canonical_question"]},
            )

        await tx.execute(
            """
            UPDATE automation_suggestions
            SET status = 'approved', reviewed_by = $1, reviewed_at = NOW()
            WHERE id = $2 AND account_id = $3
            """,
            reviewed_by_uuid,
            sugg_uuid,
            tx.account_id,
        )
        await write_audit_log(
            db=tx,
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
    async with db.transaction() as tx:
        row = await tx.fetchrow(
            "SELECT type, status FROM automation_suggestions WHERE id = $1 AND account_id = $2 FOR UPDATE",
            sugg_uuid, tx.account_id
        )
        if not row:
            raise HTTPException(status_code=404, detail="Suggestion not found")
        if row["status"] != "pending":
            raise HTTPException(status_code=400, detail=f"Suggestion is already {row['status']}")

        await tx.execute(
            """
            UPDATE automation_suggestions
            SET status = 'rejected', reviewed_by = $1, reviewed_at = NOW()
            WHERE id = $2 AND account_id = $3
            """,
            reviewed_by_uuid,
            sugg_uuid,
            tx.account_id,
        )
        await write_audit_log(
            db=tx,
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
