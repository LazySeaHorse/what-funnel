# Telegram Bot API adapter

This container translates official Telegram Bot API updates and methods to the
provider-neutral messaging contract. It supports multiple bot accounts, private
one-to-one chats only, and does not import history.

## Operation

The adapter uses long polling. This fits Docker Compose without a public webhook
endpoint, DNS, or TLS setup. A first-time connection calls `deleteWebhook` with
pending-update removal; a restart resumes from the encrypted session database's
stored update offset. Each accepted update is committed with its durable event
outbox record before the offset advances.

Required configuration:

- `ADAPTER_SHARED_SECRET`: authenticates internal control and media requests.
- `ENCRYPTION_KEY`: the same 64-character hex AES-256 key used by the stack.
- `REDIS_URL`: Redis Streams transport.
- `TELEGRAM_DATABASE_PATH`: private SQLite database, normally `/data/telegram.db`.
- `CONVERSATION_INTERNAL_URL`: internal conversation-service URL for outbound media.

`TELEGRAM_BOT_API_URL` defaults to the official API and is overridden only by
automated tests. Bot tokens are supplied through the authenticated connection
API, encrypted before storage, never returned, and removed when disconnected.

## Troubleshooting

- `Needs attention` immediately after connect usually means the BotFather token
  is invalid or revoked. Retry with a replacement token.
- No inbound conversation: confirm the user opened a private chat and sent the
  bot a message. Bots cannot initiate contact; groups and channels are ignored.
- Media notice: Telegram's Bot API download and WhatFunnel both enforce a 20 MiB
  limit. Open Telegram for unsupported or oversized content.
- Repeated reconnects: inspect container health, outbound HTTPS/DNS access to
  `api.telegram.org`, Redis health, and the writable `/data` volume. Logs identify
  channel IDs but redact bot tokens.
- An outbound message can rarely duplicate if the adapter stops after Telegram
  accepts it but before local completion is committed. This is the documented
  at-least-once boundary of the Bot API.

Never publish port 8087 or copy the SQLite volume into an untrusted location.
