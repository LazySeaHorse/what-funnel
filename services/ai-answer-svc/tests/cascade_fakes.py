"""Shared fakes for cascade tests: a scripted DB and a fake provider client (no network)."""

import json
import uuid
from contextlib import ExitStack
from unittest.mock import AsyncMock, MagicMock, patch

from whatfunnel_ai import AIConfiguration, CompletionResult, ProviderError


class MockRecord(dict):
    def __getattr__(self, name):
        try:
            return self[name]
        except KeyError:
            raise AttributeError(name)


def ai_config():
    return AIConfiguration(
        api_key="key",
        base_url="https://provider.example/v1",
        analysis_model="analysis-model",
        reply_model="reply-model",
        embedding_model="embedding-model",
    )


def faq_row(question, answer, examples=(), not_for="", faq_id=None):
    return MockRecord({
        "id": faq_id or uuid.uuid4(),
        "canonical_question": question,
        "answer_text": answer,
        "trigger_phrases": list(examples),
        "not_for": not_for,
    })


def router_reply(route="handoff", faq_id="none", coverage="none", reason="none"):
    """coverage: "full" | "partial" | "none" (True/False are accepted as full/partial for brevity)."""
    if coverage is True:
        coverage = "full"
    elif coverage is False:
        coverage = "none"
    return {"route": route, "faq_id": faq_id, "faq_coverage": coverage, "handoff_reason": reason}


class FakeClient:
    """Fake ProviderClient. Router/KB replies are scripted; replies are validated against the real schema."""

    def __init__(self, router=None, kb=None, router_error=None, similarity_vector=None):
        self.router = router
        self.kb = kb
        self.router_error = router_error
        self.calls = []
        self.embed = AsyncMock(return_value=similarity_vector or [0.1] * 1536)

    async def complete_detailed(self, model, messages, schema, max_tokens=None):
        self.calls.append({"model": model, "messages": messages, "schema": schema.__name__, "max_tokens": max_tokens})
        if schema.__name__ == "RouterDecision":
            if self.router_error:
                raise self.router_error
            data = self.router
        else:
            data = self.kb
        try:
            validated = schema.model_validate(data).model_dump()
        except Exception as error:
            raise ProviderError("AI provider response failed schema validation") from error
        return CompletionResult(validated, {"prompt_tokens": 100, "completion_tokens": 20}, 5)

    async def complete(self, model, messages, schema, max_tokens=None):
        return (await self.complete_detailed(model, messages, schema, max_tokens)).data


def make_db(
    mode="draft_only",
    bubbles=("what are your hours?",),
    faqs=(),
    concepts=(),
    has_outbound=False,
    has_concepts=None,
    history=(),
    settings_extra=None,
    reply_override="inherit",
):
    """A MagicMock ScopedDB answering the queries the cascade makes."""
    conversation = MockRecord({
        "assigned_user_ids": [], "state": "active", "state_reason": None,
        "reply_override": reply_override, "run_state": "idle", "generation_epoch": 0,
        "cooldown_level": 0, "unanswered_count": 0, "unanswered_window_started_at": None,
    })
    settings = {"ai_enabled": True, "ai_reply_mode_default": mode}
    settings.update(settings_extra or {})
    account = MockRecord({"settings": json.dumps(settings)})
    unreplied = [
        MockRecord({"id": uuid.uuid4(), "content": json.dumps({"text": text}), "created_at": None})
        for text in bubbles
    ]
    if has_concepts is None:
        has_concepts = bool(concepts)

    async def fetchrow(query, *args):
        if "SELECT c.assigned_user_ids" in query:
            return conversation
        if "SELECT settings FROM accounts" in query:
            return account
        if "SET run_state = 'replying'" in query:
            return MockRecord({"generation_epoch": 1})
        if "SET state = 'cooldown'" in query:
            return MockRecord({"generation_epoch": 3})
        if "SELECT content FROM messages" in query:
            return MockRecord({"content": json.dumps({"text": bubbles[0] if bubbles else ""})})
        return None

    queries = []

    async def fetch(query, *args):
        queries.append(query)
        if "FROM patterns" in query:
            return list(faqs)
        if "FROM kb_concepts" in query:
            return list(concepts)
        if "AND direction = 'inbound' AND content_type = 'text'" in query:
            return unreplied
        if "FROM messages" in query:
            return [
                MockRecord({"sender_type": role, "content": json.dumps({"text": text})})
                for role, text in history
            ]
        return []

    async def fetchval(query, *args):
        if "direction = 'outbound')" in query:
            return has_outbound
        if "FROM kb_concepts" in query:
            return has_concepts
        if "RETURNING generation_epoch" in query:
            return 2
        if "review_flag_reason" in query:
            return 1
        return None

    db = MagicMock()
    db.fetchrow = fetchrow
    db.fetch = fetch
    db.fetchval = AsyncMock(side_effect=fetchval)
    db.execute = AsyncMock()
    db.account_id = uuid.uuid4()
    db.queries = queries
    return db


def executed(db, needle):
    return [c for c in db.execute.await_args_list if needle in c.args[0]]


class CascadeRun:
    """Runs execute_conversation_cascade against fakes and exposes what happened."""

    def __init__(self, db, client, send, insert_draft, redis_client):
        self.db, self.client, self.send, self.insert_draft, self.redis = db, client, send, insert_draft, redis_client

    def events(self):
        return executed(self.db, "INSERT INTO ai_answer_events")

    def router_logs(self):
        return executed(self.db, "INSERT INTO ai_router_decisions")

    def control_states(self):
        return [
            json.loads(c.args[1]["payload"])["state"]
            for c in self.redis.xadd.call_args_list
            if c.args[0] == "ai.control.updated"
        ]


async def run_cascade(db, client=None, send_result=None, extra_patches=()):
    from main import execute_conversation_cascade

    redis_client = AsyncMock()
    convo_id, msg_id = uuid.uuid4(), uuid.uuid4()
    send = AsyncMock(return_value=send_result or {"id": str(uuid.uuid4())})
    insert_draft = AsyncMock(return_value=uuid.uuid4())
    client = client or FakeClient(router=router_reply())
    with ExitStack() as stack:
        scoped = stack.enter_context(patch("main.ScopedDB"))
        scoped.return_value = db
        stack.enter_context(patch("main.get_ai_config", AsyncMock(return_value=ai_config())))
        stack.enter_context(patch("main.provider_client", return_value=client))
        stack.enter_context(patch("main.send_ai_message", send))
        stack.enter_context(patch("main.insert_pending_draft", insert_draft))
        for extra in extra_patches:
            stack.enter_context(extra)
        await execute_conversation_cascade(convo_id, db.account_id, msg_id, MagicMock(), redis_client)
    return CascadeRun(db, client, send, insert_draft, redis_client)
