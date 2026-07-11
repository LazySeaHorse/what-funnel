# services/ai-answer-svc — AI Answer Service

> **Stub** — built in Build Prompt 4 (AI Cascade).

Python. Consumes `messages.inbound` from Redis Streams, runs the AI cascade:
rapidfuzz → pgvector → LLM-grounded answer → gate.
Publishes to `ai.reply_ready`.
