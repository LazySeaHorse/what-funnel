import json
import uuid
import logging
import asyncio
import os
from typing import Optional
from fastapi import APIRouter, Request, HTTPException
from fastapi.responses import HTMLResponse
from pydantic import BaseModel, field_validator
from redis.asyncio import Redis

from db import ScopedDB
from crypto import encrypt, decrypt, get_key_bytes
from config import config as app_config
from llm import complete, embed, get_ai_config
from plain_text import normalize_plain_text

logger = logging.getLogger("ai-kb-compiler.playground")
router = APIRouter(prefix="/playground", tags=["playground"])

PLAYGROUND_ACCOUNT_NAME = "WhatFunnel AI Playground"

async def get_or_create_playground_account(db_pool):
    async with db_pool.acquire() as conn:
        account_id = await conn.fetchval("SELECT id FROM accounts WHERE name = $1 LIMIT 1", PLAYGROUND_ACCOUNT_NAME)
        if not account_id:
            account_id = await conn.fetchval("INSERT INTO accounts (name, product_mode) VALUES ($1, 'chatbot_only') RETURNING id", PLAYGROUND_ACCOUNT_NAME)
        
        # Development playground credentials must be supplied explicitly. Never
        # seed or overwrite provider keys from source code.
        playground_api_key = os.getenv("PLAYGROUND_AI_API_KEY", "").strip()
        if playground_api_key:
            configured = await conn.fetchval("SELECT ai_provider_config IS NOT NULL FROM accounts WHERE id = $1", account_id)
            if not configured:
                api_config = {
                    "api_key": playground_api_key,
                    "base_url": os.getenv("PLAYGROUND_AI_BASE_URL", "https://api.openai.com/v1"),
                    "completion_model": os.getenv("PLAYGROUND_AI_COMPLETION_MODEL", "gpt-4o-mini"),
                    "embedding_model": os.getenv("PLAYGROUND_AI_EMBEDDING_MODEL", "text-embedding-3-small")
                }
                enc = encrypt(get_key_bytes(app_config.APP_ENCRYPTION_KEY), json.dumps(api_config).encode("utf-8"))
                await conn.execute("UPDATE accounts SET ai_provider_config = $1 WHERE id = $2", enc, account_id)

        # Ensure a default channel exists
        ch_id = await conn.fetchval("SELECT id FROM channels WHERE account_id = $1 LIMIT 1", account_id)
        if not ch_id:
            ch_id = await conn.fetchval("INSERT INTO channels (account_id, type) VALUES ($1, 'matrix_whatsapp') RETURNING id", account_id)

        return account_id, ch_id

class ChatMessageReq(BaseModel):
    message: str
    convo_id: Optional[str] = None

class CompileReq(BaseModel):
    raw_text: str

class PatternReq(BaseModel):
    canonical_question: str
    canned_response: str
    triggers: list[str]

class ConceptExtractionItem(BaseModel):
    title: str
    summary: str
    content: str
    category: str

    @field_validator("title", "summary", "content", "category")
    @classmethod
    def plain_fields(cls, value: str) -> str:
        return normalize_plain_text(value)

class PatternExtractionItem(BaseModel):
    canonical_question: str
    canned_response: str
    triggers: list[str]

    @field_validator("canonical_question", "canned_response")
    @classmethod
    def plain_fields(cls, value: str) -> str:
        return normalize_plain_text(value)

    @field_validator("triggers")
    @classmethod
    def plain_triggers(cls, values: list[str]) -> list[str]:
        return [normalize_plain_text(value) for value in values]

class KnowledgeCompilationResponse(BaseModel):
    concepts: list[ConceptExtractionItem]
    patterns: list[PatternExtractionItem]

@router.get("/state")
async def get_playground_state(request: Request):
    db_pool = request.app.state.db
    account_id, _ = await get_or_create_playground_account(db_pool)

    async with db_pool.acquire() as conn:
        concepts = await conn.fetch(
            "SELECT id, slug, type, title, tags, body_text, created_at FROM kb_concepts WHERE account_id = $1 ORDER BY created_at DESC",
            account_id
        )
        patterns = await conn.fetch(
            "SELECT id, canonical_question, answer_text, trigger_phrases, created_at FROM patterns WHERE account_id = $1 ORDER BY created_at DESC",
            account_id
        )
        recent_events = await conn.fetch(
            "SELECT id, conversation_id, stage_matched, confidence, action, created_at FROM ai_answer_events WHERE account_id = $1 ORDER BY created_at DESC LIMIT 10",
            account_id
        )

        return {
            "account_id": str(account_id),
            "concepts": [
                {
                    "id": str(c["id"]),
                    "slug": c["slug"],
                    "title": c["title"],
                    "category": c["type"] or (c["tags"][0] if c["tags"] else "general"),
                    "summary": (c["body_text"][:140] + "...") if len(c["body_text"]) > 140 else c["body_text"],
                    "content": c["body_text"],
                    "created_at": c["created_at"].isoformat() if c["created_at"] else None
                }
                for c in concepts
            ],
            "patterns": [
                {
                    "id": str(p["id"]),
                    "canonical_question": p["canonical_question"],
                    "answer_text": p["answer_text"],
                    "triggers": p["trigger_phrases"] or [],
                    "created_at": p["created_at"].isoformat() if p["created_at"] else None
                }
                for p in patterns
            ],
            "recent_events": [
                {
                    "id": str(e["id"]),
                    "conversation_id": str(e["conversation_id"]),
                    "stage_matched": e["stage_matched"],
                    "confidence": float(e["confidence"]) if e["confidence"] is not None else None,
                    "action": e["action"],
                    "created_at": e["created_at"].isoformat() if e["created_at"] else None
                }
                for e in recent_events
            ]
        }

@router.post("/chat")
async def send_chat_message(req: ChatMessageReq, request: Request):
    if not req.message.strip():
        raise HTTPException(status_code=400, detail="Message cannot be empty")

    db_pool = request.app.state.db
    account_id, channel_id = await get_or_create_playground_account(db_pool)

    redis_url = os.getenv("REDIS_URL", "redis://redis:6379")
    if not redis_url.startswith("redis://"):
        redis_url = f"redis://{redis_url}"
    r = Redis.from_url(redis_url)

    async with db_pool.acquire() as conn:
        # Ensure contact
        contact_id = await conn.fetchval(
            "SELECT id FROM contacts WHERE account_id = $1 AND external_identity = 'playground-client'",
            account_id
        )
        if not contact_id:
            contact_id = await conn.fetchval(
                """
                INSERT INTO contacts (account_id, channel_id, external_identity, display_name)
                VALUES ($1, $2, 'playground-client', 'Demo Customer (WhatsApp)')
                RETURNING id
                """,
                account_id, channel_id
            )

        # Ensure conversation
        convo_id = None
        if req.convo_id:
            try:
                convo_id = uuid.UUID(req.convo_id)
            except ValueError:
                convo_id = None

        if not convo_id:
            convo_id = await conn.fetchval(
                "SELECT id FROM conversations WHERE contact_id = $1 AND channel_id = $2",
                contact_id, channel_id
            )
            if not convo_id:
                convo_id = await conn.fetchval(
                    """
                    INSERT INTO conversations (account_id, contact_id, channel_id, status)
                    VALUES ($1, $2, $3, 'open')
                    RETURNING id
                    """,
                    account_id, contact_id, channel_id
                )

        # Insert inbound customer message
        msg_id = await conn.fetchval(
            """
            INSERT INTO messages (account_id, conversation_id, direction, sender_type, content_type, content)
            VALUES ($1, $2, 'inbound', 'contact', 'text', $3::jsonb)
            RETURNING id
            """,
            account_id, convo_id, json.dumps({"text": req.message.strip()})
        )

        # Publish conversation.updated to trigger ai-answer-svc cascade
        await r.xadd("conversation.updated", {"payload": json.dumps({
            "account_id": str(account_id),
            "conversation_id": str(convo_id),
            "message_id": str(msg_id)
        }).encode("utf-8")})
        await r.close()

        # Poll for ai_answer_events matching this specific message
        event = None
        for _ in range(300):
            await asyncio.sleep(1.0)
            event = await conn.fetchrow(
                """
                SELECT stage_matched, confidence, action, reply_message_id, created_at
                FROM ai_answer_events
                WHERE account_id = $1 AND message_id = $2
                ORDER BY created_at DESC
                LIMIT 1
                """,
                account_id, msg_id
            )
            if event and event["created_at"]:
                break

        # Check ai.reply_ready stream in Redis for drafted answer
        reply_text = ""
        try:
            r = Redis.from_url(redis_url)
            stream_entries = await r.xrevrange("ai.reply_ready", count=25)
            await r.close()
            for entry_id, fields in stream_entries:
                p_bytes = fields.get(b"payload") or fields.get("payload")
                if p_bytes:
                    p = json.loads(p_bytes if isinstance(p_bytes, str) else p_bytes.decode("utf-8"))
                    if p.get("message_id") == str(msg_id) and (p.get("draft_text") or p.get("text")):
                        reply_text = p.get("draft_text") or p.get("text")
                        break
        except Exception as err:
            logger.warning(f"Failed to read ai.reply_ready: {err}")

        if not reply_text and event and event["reply_message_id"]:
            reply_text = await conn.fetchval(
                "SELECT content FROM messages WHERE id = $1",
                event["reply_message_id"]
            )
            if isinstance(reply_text, str) and (reply_text.startswith("{") or reply_text.startswith('"')):
                try:
                    p = json.loads(reply_text)
                    reply_text = p.get("text") or reply_text
                except Exception:
                    pass

        if not reply_text:
            if event and event["action"] == "flagged_human":
                reply_text = "[Flagged for Human Agent] The AI flagged this query for human review."
            else:
                reply_text = "No response generated by AI cascade."

        return {
            "conversation_id": str(convo_id),
            "message_id": str(msg_id),
            "stage_matched": event["stage_matched"] if event else "none",
            "confidence": float(event["confidence"]) if event and event["confidence"] is not None else 0.0,
            "action": event["action"] if event else "none",
            "reply_text": reply_text
        }

@router.post("/compile")
async def compile_raw_docs(req: CompileReq, request: Request):
    if not req.raw_text.strip():
        raise HTTPException(status_code=400, detail="Raw text is required")

    db_pool = request.app.state.db
    account_id, _ = await get_or_create_playground_account(db_pool)

    db = ScopedDB(db_pool, account_id)
    api_key, base_url, model, embedding_model = await get_ai_config(db)

    # Extract concepts and patterns via LLM
    prompt = (
        "Analyze the following operational documentation for customer support.\n"
        "Extract TWO categories of knowledge:\n"
        "1. 'concepts': Atomic knowledge concepts (with title, category, concise summary, and detailed content markdown) for broad knowledge retrieval (Layer 3 Concept RAG).\n"
        "2. 'patterns': High-frequency customer Q&A patterns for instant pattern matching (Layer 1 Rapidfuzz) and vector matching (Layer 2 Embedding). Each pattern MUST contain:\n"
        "   - 'canonical_question': The standard representative question (e.g. 'What are your business hours?', 'Where is your office located?', 'What is your refund policy?').\n"
        "   - 'canned_response': The exact, definitive markdown answer.\n"
        "   - 'triggers': A list of 4 to 8 realistic customer query variations (lower-cased phrases, conversational variants, and short colloquial keywords, e.g. ['when are you open', 'opening hours', 'what time do you close', 'business hours']).\n\n"
        f"Documentation:\n{req.raw_text}"
    )
    result = await complete(
        api_key=api_key,
        base_url=base_url,
        model=model,
        prompt=prompt,
        response_schema=KnowledgeCompilationResponse
    )

    concepts = result.get("concepts", [])
    patterns = result.get("patterns", [])
    added_concepts = []
    added_patterns = []

    async with db_pool.acquire() as conn:
        for c in concepts:
            slug = c["title"].lower().replace(" ", "-")
            slug = "".join(ch for ch in slug if ch.isalnum() or ch == "-")[:80]
            emb = await embed(api_key, base_url, embedding_model, f"{c['title']}\n{c['summary']}\n{c['content']}")
            
            category = c.get("category", "general")
            body_text = f"{c['summary']}\n\n{c['content']}"
            cid = await conn.fetchval(
                """
                INSERT INTO kb_concepts (account_id, slug, title, type, tags, body_text, embedding, source)
                VALUES ($1, $2, $3, $4, $5, $6, $7::vector, 'owner_pasted')
                ON CONFLICT (account_id, slug) DO UPDATE
                SET title = EXCLUDED.title,
                    type = EXCLUDED.type,
                    tags = EXCLUDED.tags,
                    body_text = EXCLUDED.body_text,
                    embedding = EXCLUDED.embedding,
                    updated_at = NOW()
                RETURNING id
                """,
                account_id, slug, c["title"], category, [category], body_text, str(emb)
            )
            added_concepts.append({"id": str(cid), "title": c["title"], "category": category, "slug": slug})

        for p in patterns:
            can_q = p["canonical_question"].strip()
            canned_resp = p["canned_response"].strip()
            trigs = [t.lower().strip() for t in p.get("triggers", []) if t.strip()]
            if can_q.lower().strip() not in trigs:
                trigs.append(can_q.lower().strip())

            emb = await embed(api_key, base_url, embedding_model, can_q)
            pid = await conn.fetchval(
                """
                INSERT INTO patterns (account_id, canonical_question, answer_text, trigger_phrases, embedding)
                VALUES ($1, $2, $3, $4, $5::vector)
                RETURNING id
                """,
                account_id, can_q, canned_resp, trigs, str(emb)
            )
            added_patterns.append({"id": str(pid), "canonical_question": can_q, "triggers": trigs})

    return {
        "count": len(added_concepts),
        "concepts": added_concepts,
        "patterns_count": len(added_patterns),
        "patterns": added_patterns
    }

@router.post("/clear-kb")
@router.delete("/clear-kb")
async def clear_knowledge_base(request: Request):
    db_pool = request.app.state.db
    account_id, _ = await get_or_create_playground_account(db_pool)

    async with db_pool.acquire() as conn:
        del_c = await conn.fetchval("WITH d AS (DELETE FROM kb_concepts WHERE account_id = $1 RETURNING 1) SELECT count(*) FROM d", account_id)
        del_p = await conn.fetchval("WITH d AS (DELETE FROM patterns WHERE account_id = $1 RETURNING 1) SELECT count(*) FROM d", account_id)
        await conn.execute("DELETE FROM ai_answer_events WHERE account_id = $1", account_id)
        await conn.execute("DELETE FROM messages WHERE account_id = $1", account_id)
        await conn.execute("DELETE FROM conversations WHERE account_id = $1", account_id)

    return {
        "success": True,
        "cleared_concepts": del_c or 0,
        "cleared_patterns": del_p or 0
    }

@router.delete("/concepts/{concept_id}")
async def delete_concept(concept_id: str, request: Request):
    db_pool = request.app.state.db
    account_id, _ = await get_or_create_playground_account(db_pool)
    try:
        cid = uuid.UUID(concept_id)
    except ValueError:
        raise HTTPException(status_code=400, detail="Invalid ID")

    async with db_pool.acquire() as conn:
        await conn.execute("DELETE FROM kb_concepts WHERE id = $1 AND account_id = $2", cid, account_id)
    return {"success": True}

@router.post("/patterns")
async def add_pattern(req: PatternReq, request: Request):
    db_pool = request.app.state.db
    account_id, _ = await get_or_create_playground_account(db_pool)
    db = ScopedDB(db_pool, account_id)
    api_key, base_url, _, embedding_model = await get_ai_config(db)

    async with db_pool.acquire() as conn:
        emb = await embed(api_key, base_url, embedding_model, req.canonical_question)
        pid = await conn.fetchval(
            """
            INSERT INTO patterns (account_id, canonical_question, answer_text, trigger_phrases, embedding)
            VALUES ($1, $2, $3, $4, $5::vector)
            RETURNING id
            """,
            account_id, req.canonical_question, req.canned_response, req.triggers, str(emb)
        )
        return {"id": str(pid), "success": True}

@router.delete("/patterns/{pattern_id}")
async def delete_pattern(pattern_id: str, request: Request):
    db_pool = request.app.state.db
    account_id, _ = await get_or_create_playground_account(db_pool)
    try:
        pid = uuid.UUID(pattern_id)
    except ValueError:
        raise HTTPException(status_code=400, detail="Invalid ID")

    async with db_pool.acquire() as conn:
        await conn.execute("DELETE FROM patterns WHERE id = $1 AND account_id = $2", pid, account_id)
    return {"success": True}

STUDIO_HTML = """<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>AI Layer Test Harness</title>
  <style>
    * { box-sizing: border-box; margin: 0; padding: 0; }
    body {
      background: #ffffff;
      color: #000000;
      font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", "Courier New", monospace;
      min-height: 100vh;
      display: flex;
      flex-direction: column;
    }
    header {
      background: #ffffff;
      border-bottom: 2px solid #000000;
      padding: 10px 16px;
      display: flex;
      align-items: center;
      justify-content: space-between;
    }
    .brand {
      font-weight: 700;
      font-size: 14px;
      letter-spacing: -0.2px;
      color: #000000;
      text-transform: uppercase;
    }
    .header-actions {
      display: flex;
      align-items: center;
      gap: 8px;
    }
    .main-layout {
      flex: 1;
      display: grid;
      grid-template-columns: 1.1fr 0.9fr;
      gap: 0;
      height: calc(100vh - 46px);
    }
    @media (max-width: 960px) {
      .main-layout { grid-template-columns: 1fr; height: auto; }
    }
    .chat-panel {
      display: flex;
      flex-direction: column;
      border-right: 2px solid #000000;
      background: #ffffff;
    }
    .panel-header {
      padding: 10px 14px;
      border-bottom: 1px solid #000000;
      display: flex;
      align-items: center;
      justify-content: space-between;
      background: #fafafa;
    }
    .panel-title {
      font-size: 12px;
      font-weight: 700;
      text-transform: uppercase;
      letter-spacing: 0.5px;
    }
    .btn {
      background: #ffffff;
      color: #000000;
      border: 1px solid #000000;
      border-radius: 0;
      padding: 4px 10px;
      font-size: 12px;
      font-family: inherit;
      font-weight: 600;
      cursor: pointer;
      text-transform: uppercase;
    }
    .btn:hover {
      background: #000000;
      color: #ffffff;
    }
    .btn:disabled {
      opacity: 0.4;
      cursor: not-allowed;
      background: #ffffff;
      color: #000000;
    }
    .chat-messages {
      flex: 1;
      overflow-y: auto;
      padding: 16px;
      display: flex;
      flex-direction: column;
      gap: 12px;
      background: #ffffff;
    }
    .msg-row {
      display: flex;
      flex-direction: column;
      max-width: 85%;
    }
    .msg-row.user { align-self: flex-end; align-items: flex-end; }
    .msg-row.bot { align-self: flex-start; align-items: flex-start; }
    .msg-bubble {
      padding: 8px 12px;
      font-size: 13px;
      line-height: 1.45;
      word-break: break-word;
      border: 1px solid #000000;
    }
    .msg-row.user .msg-bubble {
      background: #000000;
      color: #ffffff;
    }
    .msg-row.bot .msg-bubble {
      background: #ffffff;
      color: #000000;
    }
    .cascade-badge {
      display: inline-block;
      font-size: 11px;
      padding: 3px 6px;
      border: 1px solid #000000;
      margin-top: 4px;
      background: #f4f4f5;
      color: #000000;
    }
    .chat-input-area {
      padding: 10px 14px;
      background: #fafafa;
      border-top: 1px solid #000000;
      display: flex;
      gap: 8px;
    }
    .chat-input-box {
      flex: 1;
      background: #ffffff;
      border: 1px solid #000000;
      border-radius: 0;
      color: #000000;
      padding: 8px 10px;
      font-size: 13px;
      font-family: inherit;
      outline: none;
    }
    .chat-input-box:focus {
      outline: 1px solid #000000;
    }
    .kb-panel {
      display: flex;
      flex-direction: column;
      background: #ffffff;
      overflow-y: auto;
    }
    .kb-section {
      padding: 14px;
      background: #ffffff;
      border-bottom: 1px solid #000000;
    }
    .section-title {
      font-size: 11px;
      font-weight: 700;
      text-transform: uppercase;
      letter-spacing: 0.5px;
      color: #000000;
      margin-bottom: 8px;
      display: flex;
      justify-content: space-between;
      align-items: center;
    }
    .kb-textarea {
      width: 100%;
      height: 90px;
      background: #ffffff;
      border: 1px solid #000000;
      border-radius: 0;
      color: #000000;
      padding: 8px;
      font-size: 12px;
      font-family: inherit;
      outline: none;
      resize: vertical;
      line-height: 1.4;
    }
    .kb-action-row {
      display: flex;
      gap: 8px;
      margin-top: 8px;
    }
    .kb-tabs {
      display: flex;
      border-bottom: 1px solid #000000;
      background: #fafafa;
    }
    .kb-tab {
      flex: 1;
      padding: 8px 12px;
      font-size: 11px;
      font-family: inherit;
      font-weight: 600;
      color: #555555;
      background: #fafafa;
      border: none;
      border-bottom: 2px solid transparent;
      cursor: pointer;
      text-align: center;
      text-transform: uppercase;
    }
    .kb-tab.active {
      color: #000000;
      background: #ffffff;
      border-bottom: 2px solid #000000;
      font-weight: 700;
    }
    .concept-card, .pattern-card {
      background: #ffffff;
      border: 1px solid #000000;
      padding: 10px;
      margin-bottom: 8px;
    }
    .card-header {
      display: flex;
      align-items: center;
      justify-content: space-between;
      margin-bottom: 4px;
    }
    .concept-title, .pattern-title {
      font-size: 12px;
      font-weight: 700;
      color: #000000;
    }
    .concept-tag, .pattern-tag {
      font-size: 10px;
      font-weight: 600;
      padding: 1px 4px;
      border: 1px solid #000000;
      background: #fafafa;
      color: #000000;
      text-transform: uppercase;
    }
    .card-summary {
      font-size: 12px;
      color: #222222;
      line-height: 1.4;
    }
    .trigger-container {
      display: flex;
      flex-wrap: wrap;
      gap: 4px;
      margin-top: 6px;
    }
    .trigger-pill {
      font-size: 10px;
      padding: 2px 4px;
      background: #ffffff;
      color: #000000;
      border: 1px solid #666666;
    }
    .card-del-btn {
      background: #ffffff;
      border: 1px solid #000000;
      color: #000000;
      cursor: pointer;
      font-size: 10px;
      padding: 1px 5px;
      text-transform: uppercase;
    }
    .card-del-btn:hover {
      background: #000000;
      color: #ffffff;
    }
  </style>
</head>
<body>
  <header>
    <div class="brand">
      AI Layer Test Harness
    </div>
    <div class="header-actions">
      <button class="btn" onclick="clearKnowledgeBase()">[ Clear KB & State ]</button>
    </div>
  </header>

  <div class="main-layout">
    <div class="chat-panel">
      <div class="panel-header">
        <div class="panel-title">Conversation Test</div>
        <button class="btn" onclick="resetChat()">[ Reset Thread ]</button>
      </div>

      <div class="chat-messages" id="chatMessages">
        <div class="msg-row bot">
          <div class="msg-bubble">
            Test harness ready. Enter a customer question below to test cascade routing.
          </div>
        </div>
      </div>

      <div class="chat-input-area">
        <input type="text" id="chatInput" class="chat-input-box" placeholder="Enter test query (e.g. refund policy, business hours, pricing)..." onkeydown="handleInputKey(event)">
        <button id="sendBtn" class="btn" onclick="sendMessage()">[ Send ]</button>
      </div>
    </div>

    <div class="kb-panel">
      <div class="kb-section">
        <div class="section-title">
          <span>Ingest Documentation</span>
        </div>
        <textarea id="rawDocInput" class="kb-textarea" placeholder="Paste business details, hours, policies, or FAQs..."></textarea>
        <div class="kb-action-row">
          <button id="compileBtn" class="btn" style="flex: 1;" onclick="compileDocs()">[ Compile & Embed ]</button>
        </div>
      </div>

      <div class="kb-tabs">
        <button class="kb-tab active" id="tabConceptsBtn" onclick="switchTab('concepts')">
          Concepts (<span id="conceptCount">0</span>)
        </button>
        <button class="kb-tab" id="tabPatternsBtn" onclick="switchTab('patterns')">
          Patterns & Triggers (<span id="patternCount">0</span>)
        </button>
      </div>

      <div class="kb-section" style="flex: 1; overflow-y: auto;">
        <div id="tabConceptsContent">
          <div class="section-title">
            <span>KB Concepts (Layer 3 RAG)</span>
            <button class="btn" style="font-size: 10px; padding: 1px 6px;" onclick="loadState()">[ Refresh ]</button>
          </div>
          <div id="conceptsList">
            <div style="font-size: 12px; color: #555555; text-align: center; padding: 20px; font-family: monospace;">Loading concepts...</div>
          </div>
        </div>

        <div id="tabPatternsContent" style="display: none;">
          <div class="section-title">
            <span>Patterns & Triggers (Layer 1 & 2)</span>
            <button class="btn" style="font-size: 10px; padding: 1px 6px;" onclick="loadState()">[ Refresh ]</button>
          </div>
          <div id="patternsList">
            <div style="font-size: 12px; color: #555555; text-align: center; padding: 20px; font-family: monospace;">Loading patterns...</div>
          </div>
        </div>
      </div>
    </div>
  </div>

  <script>
    let currentConvoId = null;
    let activeTab = 'concepts';

    function switchTab(tab) {
      activeTab = tab;
      document.getElementById('tabConceptsBtn').className = tab === 'concepts' ? 'kb-tab active' : 'kb-tab';
      document.getElementById('tabPatternsBtn').className = tab === 'patterns' ? 'kb-tab active' : 'kb-tab';
      document.getElementById('tabConceptsContent').style.display = tab === 'concepts' ? 'block' : 'none';
      document.getElementById('tabPatternsContent').style.display = tab === 'patterns' ? 'block' : 'none';
    }

    async function loadState() {
      try {
        const res = await fetch('/playground/state');
        const data = await res.json();
        renderConcepts(data.concepts || []);
        renderPatterns(data.patterns || []);
      } catch (err) {
        console.error('Failed to load state', err);
      }
    }

    function renderConcepts(concepts) {
      document.getElementById('conceptCount').innerText = concepts.length;
      const listEl = document.getElementById('conceptsList');
      if (concepts.length === 0) {
        listEl.innerHTML = '<div style="font-size: 12px; color: #555555; text-align: center; padding: 20px; font-family: monospace;">No concepts in knowledge base.</div>';
        return;
      }
      listEl.innerHTML = concepts.map(c => `
        <div class="concept-card">
          <div class="card-header">
            <span class="concept-title">${escapeHtml(c.title)}</span>
            <div style="display: flex; align-items: center; gap: 6px;">
              <span class="concept-tag">${escapeHtml(c.category)}</span>
              <button class="card-del-btn" onclick="deleteConcept('${c.id}')">[ Delete ]</button>
            </div>
          </div>
          <div class="card-summary">${escapeHtml(c.summary)}</div>
        </div>
      `).join('');
    }

    function renderPatterns(patterns) {
      document.getElementById('patternCount').innerText = patterns.length;
      const listEl = document.getElementById('patternsList');
      if (patterns.length === 0) {
        listEl.innerHTML = '<div style="font-size: 12px; color: #555555; text-align: center; padding: 20px; font-family: monospace;">No patterns registered.</div>';
        return;
      }
      listEl.innerHTML = patterns.map(p => `
        <div class="pattern-card">
          <div class="card-header">
            <span class="pattern-title">${escapeHtml(p.canonical_question)}</span>
            <div style="display: flex; align-items: center; gap: 6px;">
              <span class="pattern-tag">Pattern</span>
              <button class="card-del-btn" onclick="deletePattern('${p.id}')">[ Delete ]</button>
            </div>
          </div>
          <div class="card-summary" style="margin-bottom: 6px; font-style: italic;">"${escapeHtml(p.answer_text.slice(0, 120))}${p.answer_text.length > 120 ? '...' : ''}"</div>
          <div class="trigger-container">
            ${(p.triggers || []).map(t => `<span class="trigger-pill">${escapeHtml(t)}</span>`).join('')}
          </div>
        </div>
      `).join('');
    }

    async function compileDocs() {
      const text = document.getElementById('rawDocInput').value.trim();
      if (!text) return;
      const btn = document.getElementById('compileBtn');
      btn.disabled = true;
      btn.innerHTML = '[ Compiling... ]';
      try {
        const res = await fetch('/playground/compile', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ raw_text: text })
        });
        const data = await res.json();
        if (!res.ok) {
          throw new Error(data.detail || ('HTTP ' + res.status));
        }
        document.getElementById('rawDocInput').value = '';
        await loadState();
        alert(`Compiled ${data.count || 0} Concepts and ${data.patterns_count || 0} Patterns.`);
      } catch (err) {
        alert('Compilation failed: ' + err.message);
      } finally {
        btn.disabled = false;
        btn.innerHTML = '[ Compile & Embed ]';
      }
    }

    async function clearKnowledgeBase() {
      if (!confirm('Clear all knowledge base entries and chat messages?')) {
        return;
      }
      try {
        const res = await fetch('/playground/clear-kb', { method: 'POST' });
        const data = await res.json();
        if (!res.ok) {
          throw new Error(data.detail || ('HTTP ' + res.status));
        }
        resetChat();
        await loadState();
        alert(`Cleared ${data.cleared_concepts || 0} concepts and ${data.cleared_patterns || 0} patterns.`);
      } catch (err) {
        alert('Clear failed: ' + err.message);
      }
    }

    async function deleteConcept(id) {
      if (!confirm('Delete this concept?')) return;
      try {
        await fetch('/playground/concepts/' + id, { method: 'DELETE' });
        await loadState();
      } catch (err) {
        alert('Delete failed: ' + err.message);
      }
    }

    async function deletePattern(id) {
      if (!confirm('Delete this pattern?')) return;
      try {
        await fetch('/playground/patterns/' + id, { method: 'DELETE' });
        await loadState();
      } catch (err) {
        alert('Delete failed: ' + err.message);
      }
    }

    function resetChat() {
      currentConvoId = null;
      document.getElementById('chatMessages').innerHTML = `
        <div class="msg-row bot">
          <div class="msg-bubble">
            Conversation reset.
          </div>
        </div>
      `;
    }

    function handleInputKey(e) {
      if (e.key === 'Enter' && !e.shiftKey) {
        e.preventDefault();
        sendMessage();
      }
    }

    async function sendMessage() {
      const input = document.getElementById('chatInput');
      const text = input.value.trim();
      if (!text) return;
      input.value = '';

      const chatMessages = document.getElementById('chatMessages');
      chatMessages.innerHTML += `
        <div class="msg-row user">
          <div class="msg-bubble">${escapeHtml(text)}</div>
        </div>
      `;
      
      const loadingId = 'loading-' + Date.now();
      chatMessages.innerHTML += `
        <div class="msg-row bot" id="${loadingId}">
          <div class="msg-bubble" style="font-family: monospace;">
            [ Processing query... ]
          </div>
        </div>
      `;
      chatMessages.scrollTop = chatMessages.scrollHeight;

      const sendBtn = document.getElementById('sendBtn');
      sendBtn.disabled = true;

      try {
        const res = await fetch('/playground/chat', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ message: text, convo_id: currentConvoId })
        });
        const data = await res.json();
        currentConvoId = data.conversation_id;

        let stageLabel = 'Layer 3: Concept RAG';
        if (data.stage_matched === 'pattern') {
          stageLabel = 'Layer 1: Exact Pattern';
        } else if (data.stage_matched === 'embedding') {
          stageLabel = 'Layer 2: Vector Embedding';
        } else if (data.action === 'flagged_human') {
          stageLabel = 'Flagged for Human Agent';
        }

        const confPct = Math.round((data.confidence || 0) * 100);

        const el = document.getElementById(loadingId);
        if (el) {
          el.innerHTML = `
            <div class="msg-bubble">${escapeHtml(data.reply_text)}</div>
            <div class="cascade-badge">
              [ Stage: ${stageLabel} | Conf: ${confPct}% | Action: ${escapeHtml(data.action)} ]
            </div>
          `;
        }
      } catch (err) {
        const el = document.getElementById(loadingId);
        if (el) {
          el.innerHTML = `<div class="msg-bubble">Error: ${escapeHtml(err.message)}</div>`;
        }
      } finally {
        sendBtn.disabled = false;
        chatMessages.scrollTop = chatMessages.scrollHeight;
      }
    }

    function escapeHtml(str) {
      if (!str) return '';
      return String(str)
        .replace(/&/g, '&amp;')
        .replace(/</g, '&lt;')
        .replace(/>/g, '&gt;')
        .replace(/"/g, '&quot;');
    }

    loadState();
  </script>
</body>
</html>
"""

@router.get("", response_class=HTMLResponse)
@router.get("/", response_class=HTMLResponse)
async def serve_playground_ui():
    return HTMLResponse(content=STUDIO_HTML)
