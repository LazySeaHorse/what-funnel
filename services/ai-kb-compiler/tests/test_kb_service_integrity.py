import asyncio
import json
import uuid
from unittest.mock import AsyncMock, MagicMock, patch

import pytest
from fastapi import HTTPException

import kb_service
from schemas import UpdateConceptRequest, UpdatePatternRequest
from test_kb_service import mock_provider, setup_test_data, teardown_test_data


async def _add_suggestion(db, account_id, sugg_type, payload, source_message_ids=()):
    sugg_id = uuid.uuid4()
    await db.execute(
        """
        INSERT INTO automation_suggestions (id, account_id, type, source_message_ids, proposed_payload, confidence, status)
        VALUES ($1, $2, $3, $4, $5, 0.9, 'pending')
        """,
        sugg_id, account_id, sugg_type, list(source_message_ids), json.dumps(payload),
    )
    return sugg_id


def failing_provider():
    client = MagicMock()
    client.embed = AsyncMock(side_effect=RuntimeError("provider down"))
    return client


@pytest.mark.asyncio
async def test_update_concept_fails_and_changes_nothing_when_embedding_fails():
    pool, db, account_id, user_id = await setup_test_data()
    try:
        concept_id = uuid.uuid4()
        await db.execute(
            """
            INSERT INTO kb_concepts (id, account_id, slug, type, title, body_text, source, embedding)
            VALUES ($1, $2, 'hours', 'faq', 'Hours', 'Open at 9', 'owner_pasted', $3::vector)
            """,
            concept_id, account_id, str([0.5] * 1536),
        )
        with pytest.raises(HTTPException) as exc:
            await kb_service.update_concept(
                db, concept_id, UpdateConceptRequest(body_text="Open at 10"),
                client_factory=lambda _: failing_provider(),
            )
        assert exc.value.status_code == 502
        row = await db.fetchrow("SELECT body_text FROM kb_concepts WHERE id = $1", concept_id)
        assert row["body_text"] == "Open at 9"

        # Metadata-only edits never need the provider.
        client = mock_provider()
        updated = await kb_service.update_concept(
            db, concept_id, UpdateConceptRequest(tags=["a"]), client_factory=lambda _: client,
        )
        assert updated["tags"] == ["a"]
        client.embed.assert_not_awaited()

        # A text change re-embeds.
        await kb_service.update_concept(
            db, concept_id, UpdateConceptRequest(body_text="Open at 10"), client_factory=lambda _: client,
        )
        client.embed.assert_awaited_once()
    finally:
        await teardown_test_data(pool, account_id)


@pytest.mark.asyncio
async def test_update_pattern_fails_when_embedding_fails_and_normalizes_triggers():
    pool, db, account_id, user_id = await setup_test_data()
    try:
        pattern_id = uuid.uuid4()
        await db.execute(
            """
            INSERT INTO patterns (id, account_id, canonical_question, answer_text, trigger_phrases)
            VALUES ($1, $2, 'When?', 'At 9', ARRAY['when'])
            """,
            pattern_id, account_id,
        )
        with pytest.raises(HTTPException) as exc:
            await kb_service.update_pattern(
                db, pattern_id, UpdatePatternRequest(answer_text="At 10"),
                client_factory=lambda _: failing_provider(),
            )
        assert exc.value.status_code == 502
        assert (await db.fetchval("SELECT answer_text FROM patterns WHERE id = $1", pattern_id)) == "At 9"

        updated = await kb_service.update_pattern(
            db, pattern_id, UpdatePatternRequest(trigger_phrases=["  Open  Hours ", "open hours", ""]),
            client_factory=lambda _: failing_provider(),  # not called: no embedded text changed
        )
        assert updated["trigger_phrases"] == ["open hours"]
    finally:
        await teardown_test_data(pool, account_id)


@pytest.mark.asyncio
async def test_concurrent_double_approve_creates_a_single_concept():
    pool, db, account_id, user_id = await setup_test_data()
    try:
        sugg_id = await _add_suggestion(
            db, account_id, "new_kb_concept", {"title": "Parking", "body_text": "Free parking"}
        )
        results = await asyncio.gather(
            *[
                kb_service.approve_suggestion(
                    db, sugg_id, user_id, client_factory=lambda _: mock_provider()
                )
                for _ in range(3)
            ],
            return_exceptions=True,
        )
        failures = [r for r in results if isinstance(r, Exception)]
        assert len(failures) == 2
        assert all(isinstance(f, HTTPException) and f.status_code == 400 for f in failures)
        assert await db.fetchval("SELECT COUNT(*) FROM kb_concepts WHERE account_id = $1", account_id) == 1
        approvals = await db.fetchval(
            "SELECT COUNT(*) FROM audit_logs WHERE account_id = $1 AND action = 'automation_suggestion.approved'",
            account_id,
        )
        assert approvals == 1
    finally:
        await teardown_test_data(pool, account_id)


@pytest.mark.asyncio
async def test_concurrent_approvals_with_same_title_get_distinct_slugs():
    pool, db, account_id, user_id = await setup_test_data()
    try:
        ids = [
            await _add_suggestion(db, account_id, "new_kb_concept", {"title": "Same Title", "body_text": f"b{i}"})
            for i in range(4)
        ]
        await asyncio.gather(*[
            kb_service.approve_suggestion(db, sid, user_id, client_factory=lambda _: mock_provider())
            for sid in ids
        ])
        slugs = sorted(r["slug"] for r in await db.fetch("SELECT slug FROM kb_concepts WHERE account_id = $1", account_id))
        assert slugs == ["same-title", "same-title-1", "same-title-2", "same-title-3"]
    finally:
        await teardown_test_data(pool, account_id)


@pytest.mark.asyncio
async def test_approve_new_pattern_normalizes_triggers_and_appends_canonical_question():
    pool, db, account_id, user_id = await setup_test_data()
    try:
        sugg_id = await _add_suggestion(db, account_id, "new_pattern", {
            "canonical_question": "What Are Your Hours?",
            "answer_text": "9 to 5",
            "trigger_phrases": ["  Opening   HOURS ", "opening hours", ""],
        })
        await kb_service.approve_suggestion(db, sugg_id, user_id, client_factory=lambda _: mock_provider())
        row = await db.fetchrow("SELECT trigger_phrases FROM patterns WHERE account_id = $1", account_id)
        assert list(row["trigger_phrases"]) == ["opening hours", "what are your hours?"]
    finally:
        await teardown_test_data(pool, account_id)


@pytest.mark.asyncio
@pytest.mark.parametrize(
    "edited",
    [
        {"title": 5, "body_text": "x"},
        {"title": "ok", "body_text": ""},
        {"title": "ok", "body_text": "x", "tags": "not-a-list"},
        {"title": "ok", "body_text": "x", "type": ["faq"]},
    ],
)
async def test_approve_rejects_invalid_edited_payload_with_422(edited):
    pool, db, account_id, user_id = await setup_test_data()
    try:
        sugg_id = await _add_suggestion(db, account_id, "new_kb_concept", {"title": "T", "body_text": "B"})
        with pytest.raises(HTTPException) as exc:
            await kb_service.approve_suggestion(
                db, sugg_id, user_id, edited_payload=edited, client_factory=lambda _: mock_provider()
            )
        assert exc.value.status_code == 422
        assert await db.fetchval("SELECT status FROM automation_suggestions WHERE id = $1", sugg_id) == "pending"
    finally:
        await teardown_test_data(pool, account_id)


@pytest.mark.asyncio
async def test_approve_rejects_non_object_stored_payload_with_422():
    pool, db, account_id, user_id = await setup_test_data()
    try:
        sugg_id = await _add_suggestion(db, account_id, "new_pattern", ["not", "an", "object"])
        with pytest.raises(HTTPException) as exc:
            await kb_service.approve_suggestion(db, sugg_id, user_id, client_factory=lambda _: mock_provider())
        assert exc.value.status_code == 422
    finally:
        await teardown_test_data(pool, account_id)


@pytest.mark.asyncio
async def test_approved_concept_source_matches_audit_metadata():
    pool, db, account_id, user_id = await setup_test_data()
    try:
        mined = await _add_suggestion(
            db, account_id, "new_kb_concept", {"title": "Mined", "body_text": "b"},
            source_message_ids=[uuid.uuid4()],
        )
        pasted = await _add_suggestion(db, account_id, "new_kb_concept", {"title": "Pasted", "body_text": "b"})
        for sid in (mined, pasted):
            await kb_service.approve_suggestion(db, sid, user_id, client_factory=lambda _: mock_provider())

        rows = {r["title"]: r["source"] for r in await db.fetch("SELECT title, source FROM kb_concepts WHERE account_id = $1", account_id)}
        assert rows == {"Mined": "ai_compiled", "Pasted": "owner_pasted"}
        audit = {}
        for r in await db.fetch(
            "SELECT metadata FROM audit_logs WHERE account_id = $1 AND action = 'kb_concept.created'", account_id
        ):
            meta = json.loads(r["metadata"]) if isinstance(r["metadata"], str) else r["metadata"]
            audit[meta["title"]] = meta["source"]
        assert audit == rows
    finally:
        await teardown_test_data(pool, account_id)


@pytest.mark.asyncio
async def test_compile_paste_is_all_or_nothing():
    pool, db, account_id, user_id = await setup_test_data()
    try:
        result = {
            "concepts": [
                {"type": "faq", "title": "One", "tags": [], "body_text": "a"},
                {"type": "faq", "title": "Two", "tags": [], "body_text": "b"},
            ],
            "patterns": [],
        }
        client = mock_provider(complete_result=result)
        real_audit = kb_service.write_audit_log
        calls = {"n": 0}

        async def flaky_audit(*args, **kwargs):
            calls["n"] += 1
            if calls["n"] == 2:
                raise RuntimeError("audit write failed")
            return await real_audit(*args, **kwargs)

        with patch("kb_service.write_audit_log", flaky_audit):
            with pytest.raises(RuntimeError):
                await kb_service.compile_paste(db, "raw", user_id, client_factory=lambda _: client)

        assert await db.fetchval("SELECT COUNT(*) FROM kb_concepts WHERE account_id = $1", account_id) == 0
        assert await db.fetchval("SELECT COUNT(*) FROM audit_logs WHERE account_id = $1", account_id) == 0
    finally:
        await teardown_test_data(pool, account_id)
