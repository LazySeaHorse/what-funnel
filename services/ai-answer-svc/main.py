import asyncio
import json
import logging
import os
import uuid
from datetime import datetime
import httpx
import redis
from redis.asyncio import Redis
from redis.exceptions import ResponseError
from pydantic import BaseModel, create_model

from config import config
from db import ScopedDB, create_db_pool
from llm import get_ai_config, embed, complete
from rapidfuzz import fuzz

# Set up logging
logging.basicConfig(level=getattr(logging, config.LOG_LEVEL.upper(), logging.INFO))
logger = logging.getLogger("ai-answer-svc")

async def publish_redis_stream(redis_client, stream_name: str, payload: dict):
    serialized = json.dumps(payload)
    await redis_client.xadd(stream_name, {"payload": serialized.encode("utf-8")})
    logger.debug(f"Published to stream {stream_name}: {payload}")

async def process_conversation_updated(data: dict, db_pool, redis_client):
    account_id = data.get("account_id") or data.get("AccountID")
    conversation_id = data.get("conversation_id") or data.get("ConversationID")
    message_id = data.get("message_id") or data.get("MessageID")

    if not account_id or not conversation_id or not message_id:
        logger.warning(f"Malformed conversation.updated payload: {data}")
        return

    account_uuid = uuid.UUID(account_id)
    convo_uuid = uuid.UUID(conversation_id)
    msg_uuid = uuid.UUID(message_id)

    db = ScopedDB(db_pool, account_uuid)

    # 1. Fetch message details
    msg_row = await db.fetchrow(
        "SELECT direction, content_type, content FROM messages WHERE id = $1 AND account_id = $2",
        msg_uuid, account_uuid
    )
    if not msg_row:
        logger.info(f"Message {msg_uuid} not found in DB, skipping.")
        return

    direction = msg_row["direction"]
    content_type = msg_row["content_type"]

    # Only process inbound text messages
    if direction != "inbound" or content_type != "text":
        logger.debug(f"Message {msg_uuid} is direction={direction}, content_type={content_type}. Skipping cascade.")
        return

    # Extract text content
    try:
        content = json.loads(msg_row["content"])
        inbound_text = content.get("text", "")
    except Exception as e:
        logger.error(f"Failed to parse content for message {msg_uuid}: {e}")
        return

    # Fetch conversation details
    convo_row = await db.fetchrow(
        "SELECT ai_mode_active, assigned_user_ids FROM conversations WHERE id = $1 AND account_id = $2",
        convo_uuid, account_uuid
    )
    if not convo_row:
        logger.warning(f"Conversation {convo_uuid} not found.")
        return

    ai_mode_active = convo_row["ai_mode_active"]
    assigned_user_ids = convo_row["assigned_user_ids"] or []

    # Fetch account settings
    account_row = await db.fetchrow(
        "SELECT settings FROM accounts WHERE id = $1",
        account_uuid
    )
    settings = {}
    if account_row and account_row["settings"]:
        try:
            settings = json.loads(account_row["settings"])
        except Exception:
            pass

    ai_reply_mode_default = settings.get("ai_reply_mode_default", "draft_only")
    allow_member_reply_mode_override = settings.get("allow_member_reply_mode_override", True)
    ai_may_auto_answer_mixed_conversations = settings.get("ai_may_auto_answer_mixed_conversations", False)

    # Human takeover pause check (§4)
    if not ai_mode_active:
        if not ai_may_auto_answer_mixed_conversations:
            # Cascade does not run at all. Log and return.
            await db.execute(
                """
                INSERT INTO ai_answer_events (account_id, conversation_id, message_id, stage_matched, confidence, action, reply_message_id)
                VALUES ($1, $2, $3, 'none', NULL, 'flagged_human', NULL)
                """,
                account_uuid, convo_uuid, msg_uuid
            )
            logger.info(f"AI mode inactive and mixed answering disabled for conversation {convo_uuid}. Flagged human.")
            return

    # Run the Cascade!
    stage_matched = "none"
    confidence = None
    action = "flagged_human"
    answer_markdown = ""
    reply_message_id = None

    # Step 1: Rapidfuzz trigger match
    patterns = await db.fetch(
        "SELECT trigger_phrases, answer_markdown FROM patterns WHERE account_id = $1",
        account_uuid
    )
    RAPIDFUZZ_THRESHOLD = 90.0
    matched_pattern = None

    for pat in patterns:
        triggers = pat["trigger_phrases"] or []
        for trig in triggers:
            score = fuzz.ratio(trig.lower().strip(), inbound_text.lower().strip())
            if score >= RAPIDFUZZ_THRESHOLD:
                matched_pattern = pat
                confidence = 1.0
                stage_matched = "pattern"
                answer_markdown = pat["answer_markdown"]
                break
        if matched_pattern:
            break

    # Step 2: Embedding stage
    if stage_matched == "none" and patterns:
        try:
            # Fetch AI config
            api_key, base_url, comp_model, embed_model = await get_ai_config(db)
            inbound_emb = await embed(api_key, base_url, embed_model, inbound_text)

            # Query database for closest pattern
            # pgvector distance operator: <=> (cosine distance). Cosine similarity = 1 - distance.
            closest_pat = await db.fetchrow(
                """
                SELECT answer_markdown, 1 - (embedding <=> $1::vector) as similarity
                FROM patterns
                WHERE account_id = $2 AND embedding IS NOT NULL
                ORDER BY similarity DESC
                LIMIT 1
                """,
                str(inbound_emb), account_uuid
            )
            EMBEDDING_THRESHOLD = 0.85
            if closest_pat and closest_pat["similarity"] >= EMBEDDING_THRESHOLD:
                stage_matched = "embedding"
                confidence = float(closest_pat["similarity"])
                answer_markdown = closest_pat["answer_markdown"]
        except Exception as e:
            logger.error(f"Embedding stage failed: {e}")

    # Step 3: Concept RAG stage
    if stage_matched == "none":
        try:
            api_key, base_url, comp_model, embed_model = await get_ai_config(db)
            inbound_emb = await embed(api_key, base_url, embed_model, inbound_text)

            # Retrieve top-5 kb_concepts
            concepts = await db.fetch(
                """
                SELECT title, body_markdown, 1 - (embedding <=> $1::vector) as similarity
                FROM kb_concepts
                WHERE account_id = $2 AND embedding IS NOT NULL
                ORDER BY similarity DESC
                LIMIT 5
                """,
                str(inbound_emb), account_uuid
            )
            if concepts:
                concepts_text = ""
                for c in concepts:
                    concepts_text += f"Title: {c['title']}\nBody:\n{c['body_markdown']}\n\n"

                # Fetch history
                history = await db.fetch(
                    """
                    SELECT direction, sender_type, content
                    FROM messages
                    WHERE conversation_id = $1 AND account_id = $2 AND content_type = 'text'
                    ORDER BY created_at DESC
                    LIMIT 10
                    """,
                    convo_uuid, account_uuid
                )
                history_list = []
                for h in reversed(history):
                    try:
                        t_body = json.loads(h["content"]).get("text", "")
                    except Exception:
                        t_body = ""
                    history_list.append(f"{h['direction']} ({h['sender_type']}): {t_body}")
                history_text = "\n".join(history_list)

                # Schema
                class CascadeLLMResponse(BaseModel):
                    answer_markdown: str
                    confidence: float
                    needs_human: bool

                # Prompt
                prompt_msgs = [
                    {
                        "role": "user",
                        "content": (
                            f"You are an AI assistant for a business.\n"
                            f"Use the following Knowledge Base Concepts to answer the customer's query.\n"
                            f"Do not invent any facts not in the concepts. If the answer cannot be confidently answered, set needs_human to True.\n\n"
                            f"Knowledge Base Concepts:\n{concepts_text}\n"
                            f"Recent Conversation History:\n{history_text}\n"
                            f"Customer Query: \"{inbound_text}\""
                        )
                    }
                ]
                llm_res = await complete(api_key, base_url, comp_model, prompt_msgs, CascadeLLMResponse)
                
                stage_matched = "llm_grounded"
                confidence = float(llm_res["confidence"])
                
                LLM_CONFIDENCE_THRESHOLD = 0.70
                if not llm_res["needs_human"] and confidence >= LLM_CONFIDENCE_THRESHOLD:
                    answer_markdown = llm_res["answer_markdown"]
        except Exception as e:
            logger.error(f"Concept RAG stage failed: {e}")

    # Determine candidate action
    if answer_markdown:
        # Resolve effective reply mode
        effective_mode = ai_reply_mode_default
        if allow_member_reply_mode_override and assigned_user_ids:
            # Query override of first assigned user
            first_user_id = assigned_user_ids[0]
            user_row = await db.fetchrow(
                "SELECT reply_mode_override FROM users WHERE id = $1 AND account_id = $2",
                first_user_id, account_uuid
            )
            if user_row and user_row["reply_mode_override"]:
                effective_mode = user_row["reply_mode_override"]

        if effective_mode == "auto_send":
            action = "auto_sent"
        else:
            action = "drafted"

    # Run LLM-judge pre-check if action is auto_sent and conversation has prior human messages
    if action == "auto_sent":
        # Check if conversation has prior human messages
        human_msgs_count = await db.fetchval(
            "SELECT COUNT(*) FROM messages WHERE conversation_id = $1 AND account_id = $2 AND sender_type = 'human'",
            convo_uuid, account_uuid
        )
        if human_msgs_count > 0:
            try:
                # LLM-judge check
                class LLMJudgeResponse(BaseModel):
                    should_ai_respond: bool
                    reason: str

                api_key, base_url, comp_model, embed_model = await get_ai_config(db)
                # Fetch history
                history = await db.fetch(
                    """
                    SELECT direction, sender_type, content
                    FROM messages
                    WHERE conversation_id = $1 AND account_id = $2 AND content_type = 'text'
                    ORDER BY created_at DESC
                    LIMIT 10
                    """,
                    convo_uuid, account_uuid
                )
                history_list = []
                for h in reversed(history):
                    try:
                        t_body = json.loads(h["content"]).get("text", "")
                    except Exception:
                        t_body = ""
                    history_list.append(f"{h['direction']} ({h['sender_type']}): {t_body}")
                history_text = "\n".join(history_list)

                judge_prompt_msgs = [
                    {
                        "role": "user",
                        "content": (
                            f"You are an AI supervisor. A human support agent has previously responded in this conversation.\n"
                            f"An AI assistant wants to automatically send this reply to the customer:\n"
                            f"\"{answer_markdown}\"\n\n"
                            f"Conversation History:\n{history_text}\n\n"
                            f"Determine if it is appropriate for the AI to respond automatically, or if the thread should remain flagged for a human agent.\n"
                            f"Output your decision matching the requested schema."
                        )
                    }
                ]
                judge_res = await complete(api_key, base_url, comp_model, judge_prompt_msgs, LLMJudgeResponse)
                if not judge_res["should_ai_respond"]:
                    logger.info(f"LLM Judge blocked auto_sent for conversation {convo_uuid}. Reason: {judge_res['reason']}")
                    action = "flagged_human"
            except Exception as e:
                logger.error(f"LLM Judge pre-check failed: {e}")
                action = "flagged_human"  # Safe default on error

    # Execute action
    if action == "auto_sent":
        try:
            # Call conversation-svc SendMessage API
            send_url = f"http://conversation-svc:8083/internal/conversations/{convo_uuid}/send"
            secret = os.getenv("SESSION_SECRET", "change-me-in-production-at-least-32-chars")
            headers = {
                "X-Internal-Token": secret,
                "X-Account-ID": str(account_uuid),
                "X-User-Role": "admin",
                "Content-Type": "application/json"
            }
            body = {
                "content_type": "text",
                "text": answer_markdown,
                "sender_type": "ai"
            }
            async with httpx.AsyncClient() as client:
                resp = await client.post(send_url, json=body, headers=headers, timeout=10.0)
                resp.raise_for_status()
                res_data = resp.json()
                reply_message_id = uuid.UUID(res_data["id"])
        except Exception as e:
            logger.error(f"Failed to auto-send reply message for conversation {convo_uuid}: {e}")
            action = "flagged_human"

    # Log to ai_answer_events
    await db.execute(
        """
        INSERT INTO ai_answer_events (account_id, conversation_id, message_id, stage_matched, confidence, action, reply_message_id)
        VALUES ($1, $2, $3, $4, $5, $6, $7)
        """,
        account_uuid, convo_uuid, msg_uuid, stage_matched, confidence, action, reply_message_id
    )

    logger.info(f"Cascade finished for message {msg_uuid}. Action: {action}. Stage Matched: {stage_matched}")

    # Publish to Redis ai.reply_ready stream if drafted or auto_sent
    if action in ("drafted", "auto_sent"):
        ws_payload = {
            "account_id": str(account_uuid),
            "conversation_id": str(convo_uuid),
            "action": action,
            "message_id": str(reply_message_id) if reply_message_id else str(msg_uuid)
        }
        if action == "drafted":
            ws_payload["draft_text"] = answer_markdown

        await publish_redis_stream(redis_client, "ai.reply_ready", ws_payload)

async def process_conversation_closed(data: dict, db_pool, redis_client):
    account_id = data.get("account_id") or data.get("AccountID")
    conversation_id = data.get("conversation_id") or data.get("ConversationID")

    if not account_id or not conversation_id:
        logger.warning(f"Malformed conversation.closed payload: {data}")
        return

    account_uuid = uuid.UUID(account_id)
    convo_uuid = uuid.UUID(conversation_id)

    db = ScopedDB(db_pool, account_uuid)

    # Debounce check §7:
    # 1. Fetch current message count
    current_message_count = await db.fetchval(
        "SELECT COUNT(*) FROM messages WHERE conversation_id = $1 AND account_id = $2",
        convo_uuid, account_uuid
    )
    if not current_message_count:
        current_message_count = 0

    # 2. Fetch last generated summary
    summary_row = await db.fetchrow(
        "SELECT generated_at, message_count_at_generation FROM conversation_summaries WHERE conversation_id = $1 AND account_id = $2",
        convo_uuid, account_uuid
    )

    should_regenerate = False
    if not summary_row:
        # Eligible if there is at least 1 message
        if current_message_count > 0:
            should_regenerate = True
    else:
        # Check conditions:
        # now - last_generated_at >= 60s
        # message_count_at_generation < current_message_count
        gen_at = summary_row["generated_at"]
        msg_count_at_gen = summary_row["message_count_at_generation"]
        
        elapsed = (datetime.now(gen_at.tzinfo) - gen_at).total_seconds()
        if elapsed >= 60.0 and msg_count_at_gen < current_message_count:
            should_regenerate = True

    if not should_regenerate:
        logger.info(f"Debounce conditions not met for conversation {convo_uuid}. Skipping summary generation.")
        return

    # Trigger resumption logic: set conversations.ai_mode_active = true
    await db.execute(
        "UPDATE conversations SET ai_mode_active = true WHERE id = $1 AND account_id = $2",
        convo_uuid, account_uuid
    )

    # 3. Generate summary
    # Fetch account summary_schema
    account_row = await db.fetchrow(
        "SELECT settings FROM accounts WHERE id = $1",
        account_uuid
    )
    settings = {}
    if account_row and account_row["settings"]:
        try:
            settings = json.loads(account_row["settings"])
        except Exception:
            pass

    summary_schema = settings.get("summary_schema")
    if not summary_schema:
        summary_schema = [
            {"key": "customer_wants", "label": "Customer Wants", "description": "What the customer is looking for"},
            {"key": "preferred_timeframe", "label": "Preferred Timeframe", "description": "When the customer wants it"},
            {"key": "objections", "label": "Objections", "description": "Customer doubts or objections"},
            {"key": "next_action", "label": "Next Action", "description": "What needs to be done next"}
        ]

    # Fetch recent messages
    history = await db.fetch(
        """
        SELECT direction, sender_type, content
        FROM messages
        WHERE conversation_id = $1 AND account_id = $2 AND content_type = 'text'
        ORDER BY created_at ASC
        """,
        convo_uuid, account_uuid
    )
    if not history:
        logger.info(f"No messages in conversation {convo_uuid} to summarize.")
        return

    history_list = []
    for h in history:
        try:
            t_body = json.loads(h["content"]).get("text", "")
        except Exception:
            t_body = ""
        history_list.append(f"{h['direction']} ({h['sender_type']}): {t_body}")
    history_text = "\n".join(history_list)

    try:
        # Build dynamic Pydantic model
        fields = {item["key"]: (str, ...) for item in summary_schema}
        DynamicSummarySchema = create_model("DynamicSummarySchema", **fields)

        api_key, base_url, comp_model, embed_model = await get_ai_config(db)
        prompt_msgs = [
            {
                "role": "user",
                "content": (
                    f"Analyze the following conversation history and extract details for each of the requested summary fields.\n"
                    f"Do not invent any details. If a field is not mentioned, use 'N/A' or 'Not discussed'.\n\n"
                    f"Conversation History:\n{history_text}"
                )
            }
        ]
        summary_data = await complete(api_key, base_url, comp_model, prompt_msgs, DynamicSummarySchema)

        # Upsert summary
        await db.execute(
            """
            INSERT INTO conversation_summaries (account_id, conversation_id, summary_fields, generated_at, message_count_at_generation)
            VALUES ($1, $2, $3, NOW(), $4)
            ON CONFLICT (conversation_id)
            DO UPDATE SET
                summary_fields = EXCLUDED.summary_fields,
                generated_at = EXCLUDED.generated_at,
                message_count_at_generation = EXCLUDED.message_count_at_generation
            """,
            account_uuid, convo_uuid, json.dumps(summary_data), current_message_count
        )

        logger.info(f"Successfully generated summary for conversation {convo_uuid}")

        # Publish a lightweight websocket event for "summary updated"
        ws_payload = {
            "account_id": str(account_uuid),
            "conversation_id": str(convo_uuid),
            "summary_fields": summary_data
        }
        await publish_redis_stream(redis_client, "conversation.summary_updated", ws_payload)

    except Exception as e:
        logger.error(f"Failed to generate summary for conversation {convo_uuid}: {e}")

async def consume_stream(redis_client, db_pool, stream_name: str, group_name: str, consumer_name: str, handler):
    # Ensure group exists
    try:
        await redis_client.xgroup_create(stream_name, group_name, id="0", mkstream=True)
        logger.info(f"Created consumer group {group_name} on stream {stream_name}")
    except ResponseError as e:
        if "BUSYGROUP" not in str(e):
            logger.error(f"Failed to create group {group_name}: {e}")
            raise e
        logger.info(f"Consumer group {group_name} already exists on stream {stream_name}")

    while True:
        try:
            streams = await redis_client.xreadgroup(
                groupname=group_name,
                consumername=consumer_name,
                streams={stream_name: ">"},
                count=1,
                block=1000
            )
            for _, messages in streams:
                for msg_id, payload in messages:
                    try:
                        raw_payload = payload[b"payload"]
                        data = json.loads(raw_payload.decode("utf-8"))
                        await handler(data, db_pool, redis_client)
                        await redis_client.xack(stream_name, group_name, msg_id)
                    except Exception as e:
                        logger.exception(f"Error processing message {msg_id} in stream {stream_name}: {e}")
        except ResponseError as e:
            if "NOGROUP" in str(e):
                logger.warning(f"Consumer group missing for stream {stream_name}. Re-creating...")
                try:
                    await redis_client.xgroup_create(stream_name, group_name, id="0", mkstream=True)
                except Exception:
                    pass
            await asyncio.sleep(1)
        except Exception as e:
            logger.error(f"Error in consumer loop for stream {stream_name}: {e}")
            await asyncio.sleep(1)

async def main():
    logger.info("Initializing database connection pool...")
    db_pool = await create_db_pool(config.DATABASE_URL)

    redis_url = f"redis://{config.REDIS_URL}"
    logger.info(f"Connecting to Redis at {redis_url}...")
    redis_client = Redis.from_url(redis_url)

    logger.info("Starting stream consumers...")
    # Run both consumers concurrently
    await asyncio.gather(
        consume_stream(
            redis_client,
            db_pool,
            "conversation.updated",
            "ai-answer-svc-group",
            "ai-answer-svc-consumer",
            process_conversation_updated
        ),
        consume_stream(
            redis_client,
            db_pool,
            "conversation.closed",
            "ai-answer-svc-group",
            "ai-answer-svc-consumer",
            process_conversation_closed
        )
    )

if __name__ == "__main__":
    asyncio.run(main())
