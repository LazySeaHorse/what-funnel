import os
import uuid
import json
import math
import logging
import asyncio
from datetime import datetime, timezone
from typing import List, Optional
from pydantic import BaseModel, Field

from db import ScopedDB
from llm import get_ai_config, embed, complete

logger = logging.getLogger("ai-kb-compiler")

MINING_SIMILARITY_THRESHOLD = 0.85

# Pydantic schema for mining suggestion drafting
class MineClusterDraft(BaseModel):
    canonical_question: str = Field(description="The canonical question representing this cluster of user messages")
    answer_markdown: str = Field(description="Proposed Markdown answer. Ground the answer using the provided KB concepts if relevant. If no concepts match, write a fresh drafted answer.")

# Pure Python vector helpers
def dot_product(v1: List[float], v2: List[float]) -> float:
    return sum(x * y for x, y in zip(v1, v2))

def norm(v: List[float]) -> float:
    return math.sqrt(sum(x * x for x in v))

def cosine_similarity(v1: List[float], v2: List[float]) -> float:
    n1 = norm(v1)
    n2 = norm(v2)
    if n1 == 0.0 or n2 == 0.0:
        return 0.0
    return dot_product(v1, v2) / (n1 * n2)

def mean_vector(vectors: List[List[float]]) -> List[float]:
    if not vectors:
        return []
    dim = len(vectors[0])
    res = [0.0] * dim
    for v in vectors:
        for i in range(dim):
            res[i] += v[i]
    num = len(vectors)
    return [x / num for x in res]

class Cluster:
    def __init__(self, first_msg_id: uuid.UUID, first_text: str, first_emb: List[float]):
        self.message_ids = [first_msg_id]
        self.texts = [first_text]
        self.embeddings = [first_emb]
        self.centroid = first_emb

    def add(self, msg_id: uuid.UUID, text: str, emb: List[float]):
        self.message_ids.append(msg_id)
        self.texts.append(text)
        self.embeddings.append(emb)
        self.centroid = mean_vector(self.embeddings)

def cluster_messages(messages: List[dict], threshold: float) -> List[Cluster]:
    clusters: List[Cluster] = []
    for msg in messages:
        msg_id = msg["id"]
        text = msg["text"]
        emb = msg["embedding"]
        
        best_similarity = -1.0
        best_cluster = None
        
        for c in clusters:
            sim = cosine_similarity(emb, c.centroid)
            if sim > best_similarity:
                best_similarity = sim
                best_cluster = c
                
        if best_cluster and best_similarity >= threshold:
            best_cluster.add(msg_id, text, emb)
        else:
            clusters.append(Cluster(msg_id, text, emb))
            
    return clusters

async def run_mining(db: ScopedDB) -> dict:
    """
    Executes the conversation mining flow for a single account.
    """
    run_at = datetime.now(timezone.utc)

    # 1. Determine window_start
    # Get last run's window_end
    last_run = await db.fetchrow(
        "SELECT window_end FROM kb_mining_runs WHERE account_id = $1 ORDER BY run_at DESC LIMIT 1",
        db.account_id
    )

    if last_run and last_run["window_end"]:
        window_start = last_run["window_end"]
    else:
        # Fallback to account creation time
        account_row = await db.fetchrow(
            "SELECT created_at FROM accounts WHERE id = $1",
            db.account_id
        )
        if account_row and account_row["created_at"]:
            window_start = account_row["created_at"]
        else:
            window_start = run_at

    window_end = run_at

    # 2. Pull inbound contact text messages in the window
    # Content structure: content->>'text' extracts the text payload
    message_rows = await db.fetch(
        """
        SELECT id, content->>'text' as text
        FROM messages
        WHERE account_id = $1
          AND direction = 'inbound'
          AND sender_type = 'contact'
          AND content_type = 'text'
          AND created_at >= $2
          AND created_at <= $3
        """,
        db.account_id,
        window_start,
        window_end
    )

    # 3. Minimum scanned messages cutoff
    if len(message_rows) < 5:
        logger.info(f"Account {db.account_id}: Only {len(message_rows)} messages found. Skipping mining run.")
        # Log empty run
        await db.execute(
            """
            INSERT INTO kb_mining_runs (account_id, run_at, window_start, window_end, messages_scanned, clusters_found, suggestions_created)
            VALUES ($1, $2, $3, $4, $5, 0, 0)
            """,
            db.account_id,
            run_at,
            window_start,
            window_end,
            len(message_rows)
        )
        return {
            "messages_scanned": len(message_rows),
            "clusters_found": 0,
            "suggestions_created": 0
        }

    # 4. Generate embeddings in parallel
    api_key, base_url, completion_model, embedding_model = await get_ai_config(db)

    async def embed_msg(row) -> Optional[dict]:
        text = row["text"]
        if not text or not text.strip():
            return None
        try:
            emb = await embed(api_key, base_url, embedding_model, text)
            return {
                "id": row["id"],
                "text": text,
                "embedding": emb
            }
        except Exception as e:
            logger.error(f"Failed to embed message {row['id']}: {e}")
            return None

    tasks = [embed_msg(r) for r in message_rows]
    embedded_results = await asyncio.gather(*tasks)
    valid_messages = [r for r in embedded_results if r is not None]

    if not valid_messages:
        # Log run with 0 scanned
        await db.execute(
            """
            INSERT INTO kb_mining_runs (account_id, run_at, window_start, window_end, messages_scanned, clusters_found, suggestions_created)
            VALUES ($1, $2, $3, $4, $5, 0, 0)
            """,
            db.account_id,
            run_at,
            window_start,
            window_end,
            len(message_rows)
        )
        return {
            "messages_scanned": len(message_rows),
            "clusters_found": 0,
            "suggestions_created": 0
        }

    # 5. Cluster messages using greedy clustering
    clusters = cluster_messages(valid_messages, MINING_SIMILARITY_THRESHOLD)

    # 6. Discard clusters smaller than 3
    qualifying_clusters = [c for c in clusters if len(c.message_ids) >= 3]

    suggestions_created = 0

    # 7. Process qualifying clusters
    for cluster in qualifying_clusters:
        # Retrieve top 3 similar concepts for grounding
        centroid_str = f"[{','.join(map(str, cluster.centroid))}]"
        concept_rows = await db.fetch(
            """
            SELECT title, body_markdown
            FROM kb_concepts
            WHERE account_id = $1 AND embedding IS NOT NULL
            ORDER BY embedding <=> $2::vector
            LIMIT 3
            """,
            db.account_id,
            centroid_str
        )

        kb_context = ""
        if concept_rows:
            kb_context = "\n".join(
                f"Concept Title: {r['title']}\nContent: {r['body_markdown']}\n---"
                for r in concept_rows
            )

        # Build prompt
        msg_list_str = "\n".join(f"- {txt}" for txt in cluster.texts)
        prompt = (
            f"We have clustered a group of similar user questions:\n"
            f"{msg_list_str}\n\n"
        )
        if kb_context:
            prompt += (
                f"Here are the most relevant existing knowledge base concepts for context:\n"
                f"{kb_context}\n\n"
            )
        prompt += (
            "Draft a single canonical question that summarizes this cluster, "
            "and a proposed Markdown answer. Ground the answer in the provided concepts if they are relevant."
        )

        try:
            draft_result = await complete(
                api_key,
                base_url,
                completion_model,
                prompt,
                MineClusterDraft
            )
        except Exception as e:
            logger.error(f"Failed to draft pattern suggestion for cluster: {e}")
            continue

        # Compute confidence: min(1.0, cluster_size / 10) * avg_similarity
        total_sim = sum(cosine_similarity(emb, cluster.centroid) for emb in cluster.embeddings)
        avg_similarity = total_sim / len(cluster.embeddings)
        cluster_size = len(cluster.message_ids)
        confidence = min(1.0, cluster_size / 10.0) * avg_similarity

        # Write automation suggestion
        sugg_id = uuid.uuid4()
        proposed_payload = {
            "canonical_question": draft_result["canonical_question"],
            "answer_markdown": draft_result["answer_markdown"],
            "trigger_phrases": cluster.texts
        }

        await db.execute(
            """
            INSERT INTO automation_suggestions (id, account_id, type, source_message_ids, proposed_payload, confidence, status)
            VALUES ($1, $2, 'new_pattern', $3, $4, $5, 'pending')
            """,
            sugg_id,
            db.account_id,
            cluster.message_ids,
            json.dumps(proposed_payload),
            confidence
        )
        suggestions_created += 1

        # Audit log suggestion creation
        await db.execute(
            """
            INSERT INTO audit_logs (account_id, action, target_type, target_id, metadata)
            VALUES ($1, 'automation_suggestion.created', 'automation_suggestion', $2, $3)
            """,
            db.account_id,
            sugg_id,
            json.dumps({"type": "new_pattern", "canonical_question": draft_result["canonical_question"]})
        )

    # 8. Log run
    await db.execute(
        """
        INSERT INTO kb_mining_runs (account_id, run_at, window_start, window_end, messages_scanned, clusters_found, suggestions_created)
        VALUES ($1, $2, $3, $4, $5, $6, $7)
        """,
        db.account_id,
        run_at,
        window_start,
        window_end,
        len(message_rows),
        len(clusters),
        suggestions_created
    )

    return {
        "messages_scanned": len(message_rows),
        "clusters_found": len(clusters),
        "suggestions_created": suggestions_created
    }
