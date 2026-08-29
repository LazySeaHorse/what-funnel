# WhatFunnel

A chatbot automation layer + lead management workspace that connects to existing messaging channels via mautrix bridges. See [spec.md](spec.md) for the full product specification.

---

## Screenshots

| Inbox | Sign In | Onboarding |
|:---:|:---:|:---:|
| [![inbox.webp](https://i.postimg.cc/MTnSTDbk/inbox.webp)](https://postimg.cc/47RM2pSw) | [![sign-in.webp](https://i.postimg.cc/W1SLwKXK/sign-in.webp)](https://postimg.cc/SnXvyZ07) | [![onboarding-(6).webp](https://i.postimg.cc/76GFDLwL/onboarding-(6).webp)](https://postimg.cc/62Kjft7s) |

---

## Architecture Overview

```
api-gateway (Go)  →  identity-svc (Go)      — auth, RBAC, audit
                  →  workspace-svc (Go)     — accounts, users, pipeline config
                  →  conversation-svc (Go)  — contacts, conversations, messages, channels
                  →  notification-svc (Go)  — WebSocket push gateway (/ws proxying)

redis (Streams)   — inbound message ingestion stream & live event broadcast bus
postgres (pg)     — shared tenant database
apps/web (Svelte) — premium SvelteKit SPA desktop dashboard
```

---

## Quick Start

Two commands from a fresh clone give you a fully functional stack:

```bash
# 1. Start everything (Postgres, Redis, all Go services, run migrations)
docker compose up -d

# 2. Start the SvelteKit dev server
cd apps/web && npm install && npm run dev
```

Open [http://localhost:5173/](http://localhost:5173/).  
Sign up at `/signup` — the onboarding wizard walks you through the rest.

> **Migrations are automatic.** The `migrate` service in `docker-compose.yml` runs
> `goose up` before any application service starts. No manual migration step is needed.

### Prerequisites

* Docker + Docker Compose v2
* Node.js v20+ & npm

---

## Features Implemented

* **Multi-Tenant Workspace & RBAC:** Complete tenant partitioning at the database level. Admin/Member roles enforced via RBAC middleware on secure server-side sessions.
* **Inbound Ingestion Pipeline:** Redis Stream consumer ingests raw inbound events from Matrix bridges, normalises metadata, creates Contacts & Conversations, and stores Messages.
* **WebSocket Gateway Proxying:** Gateway TCP-hijacking reverse proxy routes `/ws` WebSocket requests to `notification-svc` for secure real-time push.
* **Real-time Event Broadcasting:** WebSocket events are dispatched dynamically based on tenant identity and conversation visibility settings.
* **Invited User Signup:** Dynamic invite token creation by Admins and full token-redemption member registration during signup.
* **Dynamic Adapters & Mocking:** Dynamic channel creation and decryption of credentials. Integrated mock homeserver bypass for robust testing.
* **Guided Bridge Connections:** WhatsApp, Telegram, Instagram, and Messenger have explicit bridge setup states instead of simulated connections (see [Bridge Connections & Operator Deployment](#bridge-connections--operator-deployment)).
* **SvelteKit Desktop Web App:** Bento-style inbox UI featuring list search, cursor-pagination, 7 inline message types, reaction bubble parsing, QR code WhatsApp setup, and user role administration.

---

## Bridge Connections & Operator Deployment

WhatFunnel treats a mautrix login as a connection lifecycle, not as a boolean channel record. The Conversation Service creates one non-admin Matrix user for each channel, opens a direct management room with the configured mautrix bot, and stores the resulting Matrix access token encrypted at rest.

### Supported Guided Flows

| Platform | Bridge command | User action |
| --- | --- | --- |
| WhatsApp | `login qr` | Scan the QR from WhatsApp Linked devices. |
| Telegram | `login qr` | Scan the QR from Telegram Devices. If prompted, enter the verification code or 2FA password. |
| Instagram | `login` | Provide an authenticated Instagram web-session request to mautrix. |
| Messenger | `login` | Provide an authenticated Messenger web-session request to mautrix. |

The UI polls the bridge management room for a QR, success, verification prompt, or error. It does not mark a channel as connected merely because a user clicked Connect.

### Required Operator Configuration

The connection service is disabled unless all of the following are supplied to `conversation-svc`:

```dotenv
MATRIX_HOMESERVER_URL=http://synapse:8008
MATRIX_SERVER_NAME=example.com
MATRIX_REGISTRATION_SHARED_SECRET=<synapse registration_shared_secret>
MATRIX_WHATSAPP_BRIDGE_IDENTITY=@whatsappbot:example.com
MATRIX_TELEGRAM_BRIDGE_IDENTITY=@telegrambot:example.com
MATRIX_INSTAGRAM_BRIDGE_IDENTITY=@instagrambot:example.com
MATRIX_MESSENGER_BRIDGE_IDENTITY=@messengerbot:example.com
```

Each configured bot must be backed by a running, registered bridge on the same homeserver. The base local compose stack provisions WhatsApp; the optional `docker-compose.bridges.yml` deployment adds Telegram, Instagram and Messenger. Bootstrap each one before it is started:

```bash
make bridge-bootstrap BRIDGE=telegram TELEGRAM_API_ID=... TELEGRAM_API_HASH=...
make bridge-bootstrap BRIDGE=messenger
make bridge-bootstrap BRIDGE=instagram
make bridges-up
```

Bootstrap generates separate bridge configs and appservice registrations under `adapters/matrix-mautrix/bridges/`. `matrix-init` copies their registrations into Synapse's `app_service_config_files` before Synapse starts. Those files are intentionally ignored by Git because they contain appservice tokens and persistent bridge state. The optional compose file sets the matching bot IDs: `@telegrambot:localhost`, `@instagrambot:localhost`, and `@messengerbot:localhost`.

Telegram also needs its own `api_id` and `api_hash` from [my.telegram.org/apps](https://my.telegram.org/apps). These API keys enable the bridge client but do not grant it access to user accounts.

### Meta-Session Handling & Security

Instagram and Messenger do not offer a standard OAuth hand-off for the mautrix flow. The bridge uses an authenticated web session, as documented by mautrix. WhatFunnel forwards the submitted cURL payload directly to the bridge and does not put it in its application database or logs. However, that message is part of the Matrix bridge-management workflow. Operators must set an appropriate retention policy and configure end-to-bridge encryption where their mautrix version and client architecture support it.

Use a dedicated bridge-management deployment and a restricted administrator role. Never expose `MATRIX_REGISTRATION_SHARED_SECRET`, Matrix access tokens, or Meta session material to browser code, analytics, error reporting, or general application logs.

---

## Tech Decisions

### Go Workspace vs Per-Service Modules
We use a **single `go.work` workspace** across `services/*` and `packages/go-common`. This allows running `go build ./...` and `go test ./...` from the workspace root. Shared code in `packages/go-common` is referenced as a local module with no publish step needed during development.

### Shared Database Migrations
Migrations live in `packages/go-common/migrations/`. The `migrate` Docker Compose service applies them automatically using the official [goose](https://github.com/pressly/goose) image. All application services use `depends_on: migrate: condition: service_completed_successfully` so they never start against an empty schema.

### Server-Side Sessions
We use `authboss` with server-side sessions and signed cookies. This provides instant revocation via database session deletes and fits the desktop-first nature of the app.

---

## Running Tests

### Go unit + integration tests (requires stack running)
```bash
make test
```

### Playwright E2E tests (requires stack + dev server)
```bash
docker compose up -d          # stack with auto-migrations
cd apps/web
npm install
npm run dev &                 # in background
npx playwright test           # or: make pw
```

The Playwright suite simulates external inbound messages by injecting directly into
the `messages.inbound` Redis Stream — no real WhatsApp bridge required.

---

## Makefile Targets

| Target | Description |
|---|---|
| `make up` | Start full stack; wait for migrations to finish |
| `make down` | Stop all services |
| `make migrate` | Re-run goose migrations manually (normally automatic) |
| `make test` | Run Go unit + integration tests |
| `make pw` | Run Playwright E2E tests |
| `make pw-ui` | Open Playwright interactive runner |
| `make build` | Build all Go service binaries |
| `make lint` | Run golangci-lint |

---

## Simulating Inbound Messages (Demo Mode)

You can inject a fake WhatsApp message directly into the system without a bridge:

```bash
redis-cli xadd messages.inbound '*' payload '{
  "ChannelID": "<channel-uuid-from-settings>",
  "ExternalThreadID": "thread-demo-1",
  "Contact": { "ExternalIdentity": "15551234567@s.whatsapp.net", "DisplayName": "Demo Customer" },
  "Message": { "ContentType": "text", "Text": "Do you offer house calls?", "ExternalMessageID": "msg-1" },
  "Timestamp": "2026-01-01T10:00:00Z"
}'
```

The conversation will appear in the inbox within seconds.

---

## Backlog / Fast-Follows

- Postgres RLS (row-level security) as a secondary database-level isolation layer.
- JWT auth support for mobile clients or external webhooks.
- AI Cascade Engine (Build Prompt 4): Suggested replies, auto-answers, and knowledge base compilation.
- Production build compilation and static asset serving.
