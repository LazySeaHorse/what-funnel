# WhatFunnel

A chatbot automation layer + lead management workspace that connects to existing messaging channels via mautrix bridges. See [docs/WHATFUNNEL_SPEC.md](docs/WHATFUNNEL_SPEC.md) for the full product specification.

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
* **SvelteKit Desktop Web App:** Bento-style inbox UI featuring list search, cursor-pagination, 7 inline message types, reaction bubble parsing, QR code WhatsApp setup, and user role administration.

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
