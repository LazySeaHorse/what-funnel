# WhatFunnel

WhatFunnel is a chatbot automation and lead-management workspace with a unified
one-to-one messaging inbox. Messaging providers are isolated behind small
adapter services; the core application never handles provider session data.

The current provider rollout is:

- WhatsApp through `whatsmeow`: implemented.
- Telegram through the Telegram Bot API: planned.
- Instagram and Facebook Messenger official APIs: shown as coming soon and not enabled.

Matrix, Synapse, Beeper, and mautrix are not part of this architecture.

## Messaging architecture

`packages/go-common/messaging` is the versioned wire contract shared by the
conversation service and provider adapters. An adapter translates native
provider traffic into `messaging.Event` values and consumes
`messaging.Command` values. The domain service sees only that normalized contract.

```text
WhatsApp network
      |
      v
whatsapp-adapter (whatsmeow + private SQLite session store)
      |  adapter.events / adapter.commands (Redis Streams)
      v
conversation-svc (PostgreSQL domain state + transactional command outbox)
      |
      v
api-gateway -> Svelte web inbox
```

This boundary supports multiple WhatsApp accounts per workspace and deliberately
accepts only direct chats. Group events are ignored. Text, images, video, audio,
documents, replies, reactions, edits, deletes, and receipts exist in the shared
contract. Unsupported native events become visible `notice` timeline items that
tell the user to open WhatsApp.

Provider credentials and WhatsApp device/session keys stay in the adapter's
private SQLite volume. PostgreSQL stores only connection state and the remote
account identifier. Internal adapter HTTP calls require `ADAPTER_SHARED_SECRET`.

## Media

Inbound and outbound media is limited to 20 MiB. Files are stored in a private
conversation-service volume and served only through authenticated endpoints.
Inbound WhatsApp media is fetched lazily when first opened. Cache retention is:

| Size | Retention |
| --- | --- |
| Up to 1 MiB | 7 days |
| Over 1 MiB through 10 MiB | 24 hours |
| Over 10 MiB through 20 MiB | 1 hour |
| Over 20 MiB | rejected |

Expired provider media is downloaded again when the provider reference remains
valid. A cleanup loop removes expired cached bytes.

## Local development

```bash
cp .env.example .env
docker compose up -d --build
```

Migrations run automatically. Connect WhatsApp under Settings → Channels: add a
labeled account, scan its QR code in WhatsApp's linked-devices screen, and wait
for the state to become `connected`. More labeled accounts can be connected the
same way.

The production template is `.env.example`. Generate a high-entropy value for:

```dotenv
ADAPTER_SHARED_SECRET=<high-entropy shared secret>
```

Never expose the adapter secret, WhatsApp SQLite database, or media volume to
browser code or a public network.

## Tests

Load the repository test environment before Go or Playwright commands:

```bash
source scripts/codex-env.sh && make test-short
source scripts/codex-env.sh && make test
source scripts/codex-env.sh && make pw
```

`make test-short` includes the separately versioned WhatsApp adapter module.
`make test` requires the Docker services. Real WhatsApp pairing and delivery are
manual acceptance tests; automated tests never require a live account.

See [spec.md](spec.md) for contract and operational decisions.
