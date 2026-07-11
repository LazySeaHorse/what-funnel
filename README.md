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

## Features Implemented

* **Multi-Tenant Workspace & RBAC:** Complete tenant partitioning at the database level. Admin/Member roles enforced via RBAC middleware on secure server-side sessions.
* **Inbound Ingestion Pipeline:** Redis Stream consumer ingests raw inbound events from Matrix bridges, Normalizes metadata, creates Contacts & Conversations, and stores Messages.
* **WebSocket Gateway Proxying:** Gateway TCP-hijacking reverse proxy routes `/ws` WebSocket requests to `notification-svc` for secure real-time push.
* **Real-time Event Broadcasting:** WebSocket events are dispatched dynamically based on tenant identity and private unassigned conversation settings (`unassigned_conversations_visible_to_members`).
* **Invited User Signup:** Dynamic invite token creation by Admins and full token-redemption member registration during signup.
* **Dynamic Adapters & Mocking:** Dynamic channel creation and decryption of credentials. Integrated mock homeserver bypass for robust testing.
* **SvelteKit Desktop Web App:** Bento-style inbox UI featuring list search, cursor-pagination, 7 inline message types, reaction bubble parsing, QR code WhatsApp setup, and user roles administration.

---

## Tech Decisions

### Go Workspace vs Per-Service Modules
We use a **single `go.work` workspace** across `services/*` and `packages/go-common`. This allows running `go build ./...` and `go test ./...` from the workspace root. Shared code in `packages/go-common` is referenced as a local module with no publish step needed during development.

### Shared Database Migrations
Migrations live in `packages/go-common/migrations/`. Goose runs against the shared Postgres DB, ensuring that all services access a single canonical database schema.

### Server-Side Sessions
We use `authboss` with server-side sessions and signed cookies. This provides instant revocation via database session deletes and fits the desktop-first nature of the app.

---

## Running Locally

### Prerequisites

* Docker + Docker Compose v2
* Go 1.25
* Node.js v20+ & npm

### 1. Start Services & Database

Spin up Postgres, Redis, and all Go microservices:
```bash
make up          # docker compose up -d
make migrate     # run goose migrations against local postgres
```

### 2. Run the SvelteKit Frontend

Install dependencies and start the Vite dev server:
```bash
cd apps/web
npm install
npm run dev
```
Open [http://localhost:5173/](http://localhost:5173/) in your browser. Development API calls are proxied automatically to `http://localhost:18080` (API Gateway).

### 3. Run Tests

Run the full Go test suite, including database, service, and E2E integration tests:
```bash
make test        # runs go test ./... across the whole workspace
```

To stop all background docker services:
```bash
make down
```

---

## Backlog / Fast-Follows

- Postgres RLS (row-level security) as a secondary database-level isolation layer.
- JWT auth support for mobile clients or external webhooks.
- AI Cascade Engine (Build Prompt 4): Suggested replies, auto-answers, and knowledge base compilation.
- Production build compilation and static asset serving.
