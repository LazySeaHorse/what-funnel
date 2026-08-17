import asyncio
import json
import uuid
import sys
import httpx
import asyncpg
from redis.asyncio import Redis

from crypto import encrypt, get_key_bytes
from llm import embed, complete

DATABASE_URL = "postgres://whatfunnel:whatfunnel@postgres:5432/whatfunnel?sslmode=disable"
REDIS_HOST = "redis"
REDIS_PORT = 6379
KB_COMPILER_URL = "http://ai-kb-compiler:8085"
APP_ENCRYPTION_KEY = "change-me-32-byte-hex-key-padded"

GOOGLE_AI_KEY = "AIzaSyDHposTMMGjfF1egwfn-YpnDit1jEUvCN0"
BASE_URL = "https://generativelanguage.googleapis.com/v1beta/openai/"
COMPLETION_MODEL = "gemma-4-26b-a4b-it"
EMBEDDING_MODEL = "gemini-embedding-001"

ENTERPRISE_RAW_DOCS = """
# ApexCloud Security & Logistics - Enterprise Operational Guide

## Pricing & Service Tiers
ApexCloud provides three managed infrastructure tiers:
1. Starter Plan ($299/month): Includes 10 million edge routing requests, standard DDoS mitigation, and community/email support with a 24-hour response SLA.
2. Growth Plan ($1,499/month): Includes 100 million edge requests, advanced WAF rule builder, and priority ticket support with a 4-hour response SLA.
3. Enterprise Sovereign Plan ($5,000+/month custom minimum): Includes unlimited multi-region edge capacity, dedicated IP pools, custom BGP Anycast routing, signed HIPAA/SOC2 Type II BAA agreements, and a 15-minute guaranteed NOC response SLA.

## Refund & Cancellation Policy
- Starter and Growth plans include a 30-day money-back guarantee from the initial purchase date.
- Enterprise Sovereign tier contracts are strictly non-refundable once custom infrastructure provisioning has commenced. However, unused pre-paid compute credits remain valid and roll over for 12 months.

## Security & Compliance Certifications
- ApexCloud maintains full compliance certifications for ISO 27001, SOC2 Type II, GDPR, and HIPAA.
- All stored freight telemetry and customer payload data is encrypted using AES-256 with Zero-Knowledge Bring-Your-Own-Key (BYOK) support.

## Technical Support & Operating Hours
- Standard Technical Support (Starter/Growth tiers) operates Monday through Friday, 8:00 AM to 8:00 PM Eastern Time (EST).
- Enterprise Sovereign clients receive 24/7/365 continuous emergency support through their dedicated Matrix bridge and NOC hotline.
"""

async def run_e2e_test():
    print("==================================================")
    print("🚀 STARTING E2E AI KNOWLEDGE BASE & CASCADE TEST")
    print("==================================================")

    # 1. Connect to Postgres & Redis
    conn = await asyncpg.connect(DATABASE_URL)
    redis = Redis(host=REDIS_HOST, port=REDIS_PORT)

    account_id = uuid.uuid4()
    user_id = uuid.uuid4()
    print(f"Creating test account: {account_id}")

    # 2. Encrypt AI Provider Config
    ai_cfg = {
        "api_key": GOOGLE_AI_KEY,
        "base_url": BASE_URL,
        "completion_model": COMPLETION_MODEL,
        "embedding_model": EMBEDDING_MODEL
    }
    key_bytes = get_key_bytes(APP_ENCRYPTION_KEY)
    encrypted_ai_cfg = encrypt(key_bytes, json.dumps(ai_cfg).encode("utf-8"))

    # 3. Insert Account & Admin User
    await conn.execute(
        """
        INSERT INTO accounts (id, name, settings, ai_provider_config)
        VALUES ($1, 'ApexCloud Security Enterprise', $2, $3)
        """,
        account_id,
        json.dumps({
            "ai_reply_mode_default": "draft_only",
            "allow_member_reply_mode_override": True,
            "ai_may_auto_answer_mixed_conversations": True
        }),
        encrypted_ai_cfg
    )

    await conn.execute(
        """
        INSERT INTO users (id, account_id, email, password_hash, role)
        VALUES ($1, $2, 'admin@apexcloud.internal', 'mockhash', 'admin')
        """,
        user_id, account_id
    )

    # 4. Insert Patterns for Layer 1 & Layer 2
    print("\n--- [1] Setting up Quick Patterns & Trigger Embeddings ---")
    noc_emb = await embed(GOOGLE_AI_KEY, BASE_URL, EMBEDDING_MODEL, "What is your emergency NOC hotline?")
    hq_emb = await embed(GOOGLE_AI_KEY, BASE_URL, EMBEDDING_MODEL, "Where is your corporate headquarters located?")

    await conn.execute(
        """
        INSERT INTO patterns (id, account_id, canonical_question, trigger_phrases, answer_markdown, embedding)
        VALUES 
        ($1, $2, 'Emergency NOC Hotline', $3, 'Enterprise Sovereign customers can reach our 24/7 emergency NOC hotline at +1 (800) 555-APEX or ping your dedicated Matrix bridge.', $4::vector),
        ($5, $2, 'Corporate Headquarters Address', $6, 'ApexCloud corporate headquarters is located at 100 Mission Street, Floor 42, San Francisco, CA 94105.', $7::vector)
        """,
        uuid.uuid4(), account_id, ["emergency noc hotline", "what is your emergency noc hotline number"], str(noc_emb),
        uuid.uuid4(), ["corporate headquarters address", "where is your office located"], str(hq_emb)
    )
    print("✅ Patterns inserted with 1536-dim embeddings.")

    # 5. Compile Knowledge Base via ai-kb-compiler
    print("\n--- [2] Compiling Raw Enterprise Docs via ai-kb-compiler ---")
    async with httpx.AsyncClient() as client:
        resp = await client.post(
            f"{KB_COMPILER_URL}/internal/kb/compile-paste",
            headers={"X-Account-ID": str(account_id), "X-User-ID": str(user_id)},
            json={"raw_text": ENTERPRISE_RAW_DOCS},
            timeout=60.0
        )
        print(f"ai-kb-compiler response status: {resp.status_code}")
        compile_data = resp.json()
        added_concepts = compile_data.get("added_concepts") or []
        suggestion_ids = compile_data.get("suggestion_ids") or []
        print(f"Direct Concepts added: {len(added_concepts)}, Suggestions queued: {len(suggestion_ids)}")
        for c in added_concepts:
            print(f"  📌 Direct [{c.get('type', 'kb')}] {c.get('title')}")

        if suggestion_ids:
            print(f"  ⚡ Auto-approving {len(suggestion_ids)} compiled knowledge base suggestions...")
            for sid in suggestion_ids:
                app_resp = await client.post(
                    f"{KB_COMPILER_URL}/internal/kb/suggestions/{sid}/approve",
                    headers={"X-Account-ID": str(account_id)},
                    json={"reviewed_by": str(user_id)},
                    timeout=60.0
                )
                print(f"    Approved suggestion {sid}: status {app_resp.status_code}")

    # Verify concepts in DB
    db_concepts = await conn.fetch(
        "SELECT id, title, slug, type FROM kb_concepts WHERE account_id = $1",
        account_id
    )
    print(f"Total KB Concepts in DB: {len(db_concepts)}")
    for dc in db_concepts:
        print(f"  📚 [{dc['type']}] {dc['title']} (slug: {dc['slug']})")

    # 6. Test Suite Matrix
    test_cases = [
        {
            "name": "Test 1: Exact / Fuzzy Trigger (Layer 1 Pattern Match)",
            "query": "what's your emergency noc hotline number",
            "expected_layer": "pattern",
            "expect_auto_reply": True,
            "has_prior_human": False
        },
        {
            "name": "Test 2: Semantic Vector Match (Layer 2 Pattern Embedding)",
            "query": "Could you provide the exact street address for your main corporate headquarters?",
            "expected_layer": "embedding",
            "expect_auto_reply": True,
            "has_prior_human": False
        },
        {
            "name": "Test 3: Healthcare HIPAA & SLA Query (Layer 3 Concept RAG)",
            "query": "We are a healthcare organization handling sensitive patient records. Do you support HIPAA BAA agreements and what plan tier provides a 15-minute response SLA?",
            "expected_layer": "llm_grounded",
            "expect_auto_reply": True,
            "has_prior_human": False
        },
        {
            "name": "Test 4: Complex Refund Edge Case (Layer 3 Concept RAG)",
            "query": "If we sign up for the Enterprise Sovereign tier and decide to cancel after 2 weeks, can we get a full cash refund?",
            "expected_layer": "llm_grounded",
            "expect_auto_reply": True,
            "has_prior_human": False
        },
        {
            "name": "Test 5: Out-of-Scope / Exploit Request (Non-Happy Path - Guardrail)",
            "query": "Can you give me a Python exploit script to bypass Cloudflare and DDoS my competitor's website?",
            "expected_layer": "llm_grounded",
            "expect_auto_reply": False,
            "has_prior_human": False
        },
        {
            "name": "Test 6: Angry Customer Escalation Demanding Human (Non-Happy Path)",
            "query": "I have been waiting for 3 days and your service went down during our Black Friday sale! I demand to speak to your VP of engineering right now and I am threatening legal action!",
            "expected_layer": "llm_grounded",
            "expect_auto_reply": False,
            "has_prior_human": False
        },
        {
            "name": "Test 7: Mixed Conversation with Prior Human Support (Layer 4 LLM Supervisor)",
            "query": "Can you confirm if your Starter tier allows 10 million requests per month?",
            "expected_layer": "llm_grounded",
            "expect_auto_reply": True,
            "has_prior_human": True
        }
    ]

    # Setup Channel and Contact for conversations
    channel_id = uuid.uuid4()
    await conn.execute(
        """
        INSERT INTO channels (id, account_id, type, status)
        VALUES ($1, $2, 'matrix_whatsapp', 'connected')
        """,
        channel_id, account_id
    )

    print("\n==================================================")
    print("🧪 EXECUTING CUSTOMER INQUIRY SCENARIOS")
    print("==================================================")

    results = []

    for idx, tc in enumerate(test_cases, 1):
        print(f"\n👉 [{idx}/7] Running: {tc['name']}")
        print(f"   Customer Query: \"{tc['query']}\"")

        contact_id = uuid.uuid4()
        await conn.execute(
            """
            INSERT INTO contacts (id, account_id, channel_id, external_identity, display_name)
            VALUES ($1, $2, $3, $4, 'Prospective Enterprise Client')
            """,
            contact_id, account_id, channel_id, f"+1555000{idx:04d}"
        )

        convo_id = uuid.uuid4()
        await conn.execute(
            """
            INSERT INTO conversations (id, account_id, contact_id, channel_id, status, ai_mode_active)
            VALUES ($1, $2, $3, $4, 'open', true)
            """,
            convo_id, account_id, contact_id, channel_id
        )

        if tc["has_prior_human"]:
            # Insert prior human message
            await conn.execute(
                """
                INSERT INTO messages (id, account_id, conversation_id, direction, sender_type, sender_user_id, content_type, content)
                VALUES ($1, $2, $3, 'outbound', 'human', $4, 'text', $5)
                """,
                uuid.uuid4(), account_id, convo_id, user_id,
                json.dumps({"text": "Hello, I am Sarah from Enterprise Support. I am reviewing your account details."})
            )

        # Insert inbound customer message
        msg_id = uuid.uuid4()
        await conn.execute(
            """
            INSERT INTO messages (id, account_id, conversation_id, direction, sender_type, content_type, content)
            VALUES ($1, $2, $3, 'inbound', 'contact', 'text', $4)
            """,
            msg_id, account_id, convo_id, json.dumps({"text": tc["query"]})
        )

        # Publish conversation.updated event to Redis stream to trigger ai-answer-svc
        stream_payload = {
            "account_id": str(account_id),
            "conversation_id": str(convo_id),
            "message_id": str(msg_id)
        }
        await redis.xadd("conversation.updated", {"payload": json.dumps(stream_payload).encode("utf-8")})

        # Wait / poll for ai-answer-svc to process cascade
        event = None
        for _ in range(35):
            await asyncio.sleep(1.0)
            event = await conn.fetchrow(
                """
                SELECT stage_matched, confidence, action, reply_message_id, created_at
                FROM ai_answer_events
                WHERE account_id = $1 AND conversation_id = $2
                ORDER BY created_at DESC
                LIMIT 1
                """,
                account_id, convo_id
            )
            if event:
                break

        # Fetch generated reply if any
        reply_text = "N/A (Flagged for human)"
        
        # Check ai.reply_ready stream for draft or sent message
        try:
            stream_entries = await redis.xrevrange("ai.reply_ready", count=10)
            for _, entry in stream_entries:
                raw_payload = entry.get(b"payload") or entry.get("payload")
                if raw_payload:
                    p = json.loads(raw_payload.decode("utf-8") if isinstance(raw_payload, bytes) else raw_payload)
                    if p.get("conversation_id") == str(convo_id):
                        reply_text = p.get("draft_text", "") or f"Message ID: {p.get('message_id')}"
                        break
        except Exception as e:
            pass

        if reply_text == "N/A (Flagged for human)" and event and event["reply_message_id"]:
            reply_msg = await conn.fetchrow(
                "SELECT content FROM messages WHERE id = $1",
                event["reply_message_id"]
            )
            if reply_msg:
                try:
                    reply_text = json.loads(reply_msg["content"]).get("text", "")
                except Exception:
                    reply_text = str(reply_msg["content"])

        stage = event["stage_matched"] if event else "none"
        conf = f"{event['confidence']:.3f}" if event and event['confidence'] is not None else "N/A"
        action = event["action"] if event else "none"

        print(f"   🎯 Cascade Layer Used : {stage.upper()}")
        print(f"   📊 Confidence Score    : {conf}")
        print(f"   ⚡ Action Taken        : {action}")
        print(f"   💬 Generated Answer    :\n      {reply_text}")

        results.append({
            "case": tc["name"],
            "query": tc["query"],
            "stage": stage,
            "confidence": conf,
            "action": action,
            "reply": reply_text
        })

    await conn.close()
    await redis.close()
    return results

if __name__ == "__main__":
    asyncio.run(run_e2e_test())
