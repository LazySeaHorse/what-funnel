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
