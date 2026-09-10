import os
import uuid
import json
import math
import logging
import asyncio
from datetime import datetime, timezone
from typing import List, Optional, Sequence
import numpy as np
from pydantic import BaseModel, Field, field_validator

from plain_text import normalize_plain_text

from db import ScopedDB
from llm import get_ai_config, provider_client
from redis_client import publish_suggestion_created

logger = logging.getLogger("ai-kb-compiler")

MINING_SIMILARITY_THRESHOLD = 0.85

# Pydantic schema for mining suggestion drafting
class MineClusterDraft(BaseModel):
    canonical_question: str = Field(description="The canonical question representing this cluster of user messages")
    answer_text: str = Field(description="Proposed plain-text answer. Ground it using the provided KB concepts when relevant. Never use Markdown or HTML.")

    @field_validator("canonical_question", "answer_text")
    @classmethod
    def plain_fields(cls, value: str) -> str:
        return normalize_plain_text(value)

# NumPy-accelerated vector helpers
def dot_product(v1: Sequence[float] | np.ndarray, v2: Sequence[float] | np.ndarray) -> float:
    return float(np.dot(np.asarray(v1, dtype=np.float64), np.asarray(v2, dtype=np.float64)))

def norm(v: Sequence[float] | np.ndarray) -> float:
    return float(np.linalg.norm(np.asarray(v, dtype=np.float64)))

def cosine_similarity(v1: Sequence[float] | np.ndarray, v2: Sequence[float] | np.ndarray) -> float:
    a1 = np.asarray(v1, dtype=np.float64)
    a2 = np.asarray(v2, dtype=np.float64)
    n1 = float(np.linalg.norm(a1))
    n2 = float(np.linalg.norm(a2))
    if n1 == 0.0 or n2 == 0.0:
        return 0.0
    return float(np.dot(a1, a2) / (n1 * n2))

def mean_vector(vectors: Sequence[Sequence[float] | np.ndarray]) -> List[float]:
    if not vectors:
        return []
    return np.mean(np.asarray(vectors, dtype=np.float64), axis=0).tolist()

class Cluster:
    def __init__(self, first_msg_id: uuid.UUID, first_text: str, first_emb: List[float]):
        self.message_ids = [first_msg_id]
        self.texts = [first_text]
        self.embeddings = [list(first_emb)]
        self.centroid: List[float] = list(first_emb)

    def add(self, msg_id: uuid.UUID, text: str, emb: List[float]):
        self.message_ids.append(msg_id)
        self.texts.append(text)
        self.embeddings.append(list(emb))
        self.centroid = mean_vector(self.embeddings)

def cluster_messages(messages: List[dict], threshold: float) -> List[Cluster]:
    clusters: List[Cluster] = []
    if not messages:
        return clusters

    centroid_arrays: List[np.ndarray] = []

    for msg in messages:
        msg_id = msg["id"]
        text = msg["text"]
        emb = msg["embedding"]
        emb_arr = np.asarray(emb, dtype=np.float64)
        emb_norm = float(np.linalg.norm(emb_arr))

        if not clusters:
            c = Cluster(msg_id, text, emb)
            clusters.append(c)
            centroid_arrays.append(np.asarray(c.centroid, dtype=np.float64))
            continue

        if emb_norm == 0.0:
            best_similarity = -1.0
            best_idx = -1
        else:
            centroids_mat = np.stack(centroid_arrays)  # shape (k, dim)
            c_norms = np.linalg.norm(centroids_mat, axis=1)  # shape (k,)
            
            valid_mask = c_norms > 0.0
            sims = np.full(len(clusters), -1.0, dtype=np.float64)
            if np.any(valid_mask):
                dots = np.dot(centroids_mat[valid_mask], emb_arr)
                sims[valid_mask] = dots / (c_norms[valid_mask] * emb_norm)

            best_idx = int(np.argmax(sims))
            best_similarity = float(sims[best_idx])

        if best_idx >= 0 and best_similarity >= threshold:
            best_cluster = clusters[best_idx]
            best_cluster.add(msg_id, text, emb)
            centroid_arrays[best_idx] = np.asarray(best_cluster.centroid, dtype=np.float64)
        else:
            c = Cluster(msg_id, text, emb)
            clusters.append(c)
            centroid_arrays.append(np.asarray(c.centroid, dtype=np.float64))

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

    # 2. Pull inbound contact text messages in the window that were flagged/unmatched
    message_rows = await db.fetch(
        """
        SELECT m.id, m.content->>'text' as text
        FROM messages m
        JOIN ai_answer_events a ON m.id = a.message_id AND m.account_id = a.account_id
        WHERE m.account_id = $1
          AND m.direction = 'inbound'
          AND m.sender_type = 'contact'
          AND m.content_type = 'text'
          AND m.created_at >= $2
          AND m.created_at <= $3
          AND (a.action = 'flagged_human' OR a.stage_matched = 'none')
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
    config = await get_ai_config(db)
    client = provider_client(config)

    async def embed_msg(row) -> Optional[dict]:
        text = row["text"]
        if not text or not text.strip():
            return None
        try:
            emb = await client.embed(config.embedding_model, text)
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
            SELECT title, body_text
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
                f"Concept Title: {r['title']}\nContent: {r['body_text']}\n---"
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
            "and a proposed plain-text answer. Never use Markdown or HTML. Ground the answer in the provided concepts if they are relevant."
        )

        try:
            draft_result = await client.complete(
                config.analysis_model,
                [{"role": "user", "content": prompt}],
                MineClusterDraft,
            )
        except Exception as e:
            logger.error(f"Failed to draft pattern suggestion for cluster: {e}")
            continue

        # Compute confidence: min(1.0, cluster_size / 10) * avg_similarity
        cluster_embs = np.asarray(cluster.embeddings, dtype=np.float64)
        centroid_vec = np.asarray(cluster.centroid, dtype=np.float64)
        c_norm = float(np.linalg.norm(centroid_vec))
        e_norms = np.linalg.norm(cluster_embs, axis=1)
        denom = e_norms * c_norm
        valid = denom > 0.0
        sims = np.zeros(len(cluster.embeddings), dtype=np.float64)
        if np.any(valid):
            sims[valid] = np.dot(cluster_embs[valid], centroid_vec) / denom[valid]
        avg_similarity = float(np.mean(sims))
        cluster_size = len(cluster.message_ids)
        confidence = min(1.0, cluster_size / 10.0) * avg_similarity

        # Write automation suggestion
        sugg_id = uuid.uuid4()
        proposed_payload = {
            "canonical_question": draft_result["canonical_question"],
            "answer_text": draft_result["answer_text"],
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
        await publish_suggestion_created(db.account_id, sugg_id, 'new_pattern', proposed_payload)
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
