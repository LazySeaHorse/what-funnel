# Conversation service

Owns provider-neutral contacts, conversations, messages, reactions, delivery
state, media metadata/cache, leads, and the transactional command outbox.
Provider adapters own credentials and native sessions; this service communicates
with them through the shared `packages/go-common/messaging` contract and an
authenticated internal control API.

It registers both the WhatsApp and Telegram adapters. Inbound media is fetched
lazily into the shared size-based cache, while outbound files are read by the
selected adapter through an authenticated internal endpoint. No provider SDK or
native provider type belongs in this service.

Important environment variables are `WHATSAPP_ADAPTER_URL`,
`TELEGRAM_ADAPTER_URL`, `ADAPTER_SHARED_SECRET`, and `MEDIA_CACHE_PATH`.

## Conversation summaries

`GET /conversations/{id}/summary` returns `{"summary": null}` or
`{"summary": {"fields": [{"key","label","value"}], "generated_at", "message_count_at_generation", "stale"}}`.
Fields follow the account's `summary_schema` setting (labels from the schema; stored keys the schema no longer lists
come last, labelled with their key). `stale` is true when the conversation has more messages than the summary was
generated from.

`POST /conversations/{id}/summary` asks for a new summary. It answers `202 {"status":"queued","summary":...}` after
publishing `conversation.summary_requested` (`account_id`, `conversation_id`, `requested_by`), or
`200 {"status":"up_to_date","summary":...}` without publishing when the stored summary is not stale. This service never
calls an LLM: ai-answer-svc consumes the stream (it owns provider credentials, locking, cooldown and validation) and
the result returns as the `conversation.summary_updated` / `conversation.summary_failed` websocket events. Both
endpoints use the same visibility rule as the rest of the conversation API (unseen conversations are 404).
