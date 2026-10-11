import uuid
import json
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

MAX_EXAMPLE_PHRASES = 6
MAX_EXAMPLE_WORDS = 15


def example_phrases(texts: Sequence[str]) -> List[str]:
    """Pick up to six short, distinct customer messages as router examples (shortest first).

    Raw cluster text can be long, multi-clause and contain personal data, so messages over
    15 words are dropped. PII masking is a known follow-up; the owner reviews every example
    before the FAQ is approved.
    """
    seen: set[str] = set()
    candidates: List[str] = []
    for text in texts:
        cleaned = " ".join(str(text).split())
        key = cleaned.lower()
        if cleaned and key not in seen and len(cleaned.split()) <= MAX_EXAMPLE_WORDS:
            seen.add(key)
            candidates.append(cleaned)
    candidates.sort(key=lambda t: (len(t), t))
    return candidates[:MAX_EXAMPLE_PHRASES]


class Cluster:
    """A greedy cluster; its centroid is the running mean of the members' embeddings."""

    def __init__(self, first_msg_id: uuid.UUID, first_text: str, first_emb: Sequence[float]):
        self.message_ids = [first_msg_id]
        self.texts = [first_text]
        self.embeddings = [np.asarray(first_emb, dtype=np.float64)]
        self.centroid = self.embeddings[0].copy()

    def add(self, msg_id: uuid.UUID, text: str, emb: Sequence[float]):
        emb_arr = np.asarray(emb, dtype=np.float64)
        self.message_ids.append(msg_id)
        self.texts.append(text)
        self.embeddings.append(emb_arr)
        self.centroid += (emb_arr - self.centroid) / len(self.embeddings)


def cluster_messages(messages: List[dict], threshold: float) -> List[Cluster]:
    """Assign each message to the most similar cluster centroid (cosine >= threshold) or start a new one."""
    clusters: List[Cluster] = []
    for msg in messages:
        emb = np.asarray(msg["embedding"], dtype=np.float64)
        emb_norm = float(np.linalg.norm(emb))
        best_idx, best_similarity = -1, -1.0
        if clusters and emb_norm > 0.0:
            centroids = np.stack([c.centroid for c in clusters])  # shape (k, dim)
            c_norms = np.linalg.norm(centroids, axis=1)
            sims = np.full(len(clusters), -1.0, dtype=np.float64)
            valid = c_norms > 0.0
            sims[valid] = centroids[valid] @ emb / (c_norms[valid] * emb_norm)
            best_idx = int(np.argmax(sims))
            best_similarity = float(sims[best_idx])

        if best_idx >= 0 and best_similarity >= threshold:
            clusters[best_idx].add(msg["id"], msg["text"], emb)
        else:
            clusters.append(Cluster(msg["id"], msg["text"], emb))
    return clusters


async def _record_run(db: ScopedDB, run_at, window_start, window_end, scanned: int, clusters: int = 0, created: int = 0) -> dict:
    await db.execute(
        """
        INSERT INTO kb_mining_runs (account_id, run_at, window_start, window_end, messages_scanned, clusters_found, suggestions_created)
        VALUES ($1, $2, $3, $4, $5, $6, $7)
        """,
        db.account_id, run_at, window_start, window_end, scanned, clusters, created,
    )
    return {"messages_scanned": scanned, "clusters_found": clusters, "suggestions_created": created}


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
        return await _record_run(db, run_at, window_start, window_end, len(message_rows))

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
        return await _record_run(db, run_at, window_start, window_end, len(message_rows))

    # 5. Cluster messages using greedy clustering
    clusters = cluster_messages(valid_messages, MINING_SIMILARITY_THRESHOLD)

    # 6. Discard clusters smaller than 3
    qualifying_clusters = [c for c in clusters if len(c.message_ids) >= 3]

    suggestions_created = 0

    # 7. Process qualifying clusters
    for cluster in qualifying_clusters:
        # Retrieve top 3 similar concepts for grounding
        centroid_str = str(cluster.centroid.tolist())
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
        cluster_embs = np.stack(cluster.embeddings)
        centroid_vec = cluster.centroid
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
            "trigger_phrases": example_phrases(cluster.texts),
            "not_for": "",
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

    return await _record_run(db, run_at, window_start, window_end, len(message_rows), len(clusters), suggestions_created)
