# services/ai-answer-svc — AI Answer Service

> **Stub** — built in Build Prompt 4 (AI Cascade).

Python. Consumes `conversation.updated` from Redis Streams, then runs the AI cascade for newly persisted inbound text:
rapidfuzz → pgvector → LLM-grounded answer → gate.
Publishes to `ai.reply_ready`.
