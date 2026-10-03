# WhatFunnel

**WhatFunnel** is an open-source, privacy-first conversational CRM, chatbot automation platform, and unified messaging workspace. It brings multi-channel customer conversations into a single responsive inbox while delegating provider protocols to isolated adapter microservices.

Core application services handle domain logic, contact state, and AI orchestration without ever handling raw provider session tokens or credentials.

---

## Table of Contents

- [Supported Channels](#supported-channels)
- [System Architecture](#system-architecture)
- [Core Services & Repository Layout](#core-services--repository-layout)
- [Key Capabilities](#key-capabilities)
  - [Unified Omnichannel Inbox](#unified-omnichannel-inbox)
  - [AI Answer Engine & RAG Cascade](#ai-answer-engine--rag-cascade)
  - [Knowledge Base Compiler & Conversation Mining](#knowledge-base-compiler--conversation-mining)
  - [Transactional Outbox & Delivery Guarantees](#transactional-outbox--delivery-guarantees)
  - [Media Pipeline & Retention](#media-pipeline--retention)
- [Local Development](#local-development)
  - [Prerequisites](#prerequisites)
  - [Starting the Development Stack](#starting-the-development-stack)
  - [Connecting Messaging Channels](#connecting-messaging-channels)
  - [Frontend Development](#frontend-development)
- [Testing & Quality Assurance](#testing--quality-assurance)
  - [Unit & Integration Tests](#unit--integration-tests)
  - [Playwright E2E & UI Monkey Fuzzing](#playwright-e2e--ui-monkey-fuzzing)
  - [Database Migrations & Code Generation](#database-migrations--code-generation)
- [Production Deployment](#production-deployment)
  - [Hardened Stack](#hardened-stack)
  - [Automated Deployment Script](#automated-deployment-script)
  - [Security Checklist & Secrets](#security-checklist--secrets)
- [Configuration Reference](#configuration-reference)
- [Specification](#specification)

---

## Supported Channels

- **WhatsApp**: Fully implemented via [`whatsmeow`](https://github.com/tulir/whatsmeow) in an isolated adapter with private SQLite session persistence. Supports multi-device QR code pairing.
- **Telegram**: Fully implemented for private one-to-one bot chats using the official Telegram Bot API via long polling. Bot tokens are AES-256-GCM encrypted at rest.
- **Instagram & Facebook Messenger**: Official Graph API integration planned (marked coming soon).
- *Explicitly excluded*: Matrix, Synapse, Beeper, and mautrix are not part of this architecture.

---

## System Architecture

```text
  +------------------+         +--------------------+
  | WhatsApp Network |         |  Telegram Bot API  |
  +--------+---------+         +---------+----------+
           |                             |
           v                             v
+-----------------------+     +-----------------------+
|   whatsapp-adapter    |     |   telegram-adapter    |
| (whatsmeow + SQLite)  |     | (long poll + SQLite)  |
+-----------+-----------+     +-----------+-----------+
            |                             |
            +--------------+--------------+
                           |
                           | adapter.events / adapter.commands (Redis Streams)
                           v
            +------------------------------+
            |       conversation-svc       | <---+
            |  - Transactional Outbox      |     |
            |  - Media Cache (Disk/MinIO)  |     |
            |  - Normalized Contacts & Msg |     |
            +-------+--------------+-------+     |
                    |              |             |
   conversation.    |              |             | ai.reply_ready
   updated (Stream) |              v             | (Stream)
                    |     +------------------+   |
                    |     | PostgreSQL 16    |   |
                    |     | (+ pgvector)     |   |
                    |     +------------------+   |
                    v                            |
            +------------------------------+     |
            |        ai-answer-svc         +-----+
            |  - Debouncing Burst Messages |
            |  - LLM + RAG Grounding       |
            |  - Confidence Gating         |
            +--------------+---------------+
                           ^
                           | Semantic Search
                           v
            +------------------------------+
            |        ai-kb-compiler        |
            |  - Raw Ingestion & Chunking  |
            |  - pgvector Vector Embeddings|
            |  - Dormant Conv Mining       |
            +------------------------------+

            +------------------------------+     +-------------------+
            |       identity-svc           |     |   workspace-svc   |
            |  - Auth, Sessions, RBAC      |     |  - Workspaces     |
            |  - Audit Logging             |     |  - Team & Invites |
            +--------------+---------------+     +---------+---------+
                           |                               |
                           +---------------+---------------+
                                           |
                                           v
                              +--------------------------+
                              |       api-gateway        |
                              |  - Auth Verification     |
                              |  - Reverse Proxy         |
                              +-------------+------------+
                                            |
                         +------------------+------------------+
                         |                                     |
                         v                                     v
            +--------------------------+          +--------------------------+
            |     notification-svc     |          |       apps/web (SPA)     |
            |  - WebSocket Push Engine |          |  - Svelte 5 + Tailwind 4 |
            +-------------+------------+          |  - Optimistic Inbox UI   |
                          |                       +--------------------------+
                          +====================================> Realtime WS
```

### Communication & Boundaries

1. **Normalized Wire Contract**: Located in [`packages/go-common/messaging`](packages/go-common/messaging). All inbound provider events become normalized `messaging.Event` envelopes, and outbound actions are normalized `messaging.Command` envelopes.
2. **Provider Isolation**: Adapters manage their own network lifecycles and store credentials in isolated private SQLite volumes (`/data/*.db`). The domain services never access provider session keys.
3. **Internal Authentication**: Internal HTTP communication requires `ADAPTER_SHARED_SECRET` and `INTERNAL_SERVICE_TOKEN`.

---

## Core Services & Repository Layout

| Directory | Service / Component | Technology | Description |
| --- | --- | --- | --- |
| [`services/api-gateway`](services/api-gateway) | API Gateway | Go 1.27 | Single ingress point, session verification, routing, rate limiting, and WebSocket proxying. |
| [`services/identity-svc`](services/identity-svc) | Identity Service | Go 1.27 | User accounts, authentication, session tokens, RBAC, and audit logging. |
| [`services/workspace-svc`](services/workspace-svc) | Workspace Service | Go 1.27 | Workspace configuration, member invitations, pipeline stages, and AI provider credentials. |
| [`services/conversation-svc`](services/conversation-svc) | Conversation Service | Go 1.27 | Contact management, message timeline, transactional command outbox, and media handling. |
| [`services/notification-svc`](services/notification-svc) | Notification Service | Go 1.27 | Real-time WebSocket connection manager and event fanout to client browsers. |
| [`services/ai-answer-svc`](services/ai-answer-svc) | AI Answer Engine | Python 3.11 | LLM-based answering engine with RAG retrieval, debouncing, and automated confidence gating. |
| [`services/ai-kb-compiler`](services/ai-kb-compiler) | AI KB Compiler | Python 3.11 | Knowledge base document ingestion, pgvector vectorization, and background conversation mining. |
| [`adapters/whatsapp-whatsmeow`](adapters/whatsapp-whatsmeow) | WhatsApp Adapter | Go 1.27 | WhatsApp multi-device client powered by `whatsmeow` with local SQLite outbox and session storage. |
| [`adapters/telegram-botapi`](adapters/telegram-botapi) | Telegram Adapter | Go 1.27 | Long-polling Telegram Bot API adapter with encrypted token store and durable outbox. |
| [`apps/web`](apps/web) | Web Application | Svelte 5, Vite, Tailwind CSS | Modern reactive web frontend with real-time WebSocket synchronization and optimistic updates. |
| [`packages/go-common`](packages/go-common) | Shared Go Package | Go 1.27 | Shared contracts (`messaging`), db migrations (Goose), crypto, middleware, and pubsub primitives. |
| [`tests/`](tests/) | Test Suites | Go / Playwright | Integration, failover/chaos, multi-agent scale fuzzing, and end-to-end browser tests. |

---

## Key Capabilities

### Unified Omnichannel Inbox

- Supports direct (1:1) customer chats across WhatsApp and Telegram.
- Rich content support: text, images, videos, audio/voice notes, documents, captions, emoji reactions, edits, and deletions.
- Group and channel traffic is deliberately ignored to preserve 1:1 conversation boundaries.
- Unsupported provider payloads degrade gracefully into visible, actionable `notice` timeline events prompting the user to view the native app.

### AI Answer Engine & RAG Cascade

- Located in [`services/ai-answer-svc`](services/ai-answer-svc).
- **Inbound Debouncing**: Intelligently aggregates rapid successive customer messages into a coherent prompt before answering.
- **RAG & Vector Grounding**: Queries pgvector embeddings compiled by `ai-kb-compiler` for semantic relevance.
- **Confidence Gating**: Evaluates response quality and confidence. High-confidence answers can be dispatched automatically, while marginal answers are presented as suggestions or handed over to human agents.

### Knowledge Base Compiler & Conversation Mining

- Located in [`services/ai-kb-compiler`](services/ai-kb-compiler).
- **Document & Paste Ingestion**: Ingests raw text, documentation, and FAQs, generating semantic chunks and vector embeddings.
- **Conversation Mining**: Periodically analyzes unresolved or answered conversation histories to extract candidate FAQ items and propose knowledge additions.

### Transactional Outbox & Delivery Guarantees

- **PostgreSQL Transactional Outbox**: Creating an outbound message and its command-outbox row is atomic. A background dispatcher claims ready rows with backoff and publishes them to Redis Streams.
- **Idempotency**: Both adapter event processing and outbox command consumption enforce idempotency keys, preventing duplicates across network disconnects or container restarts.
- **Crash Recovery**: Outbox row claims recover automatically if a dispatcher is killed (`SIGKILL`).

### Media Pipeline & Retention

- Hard upload and download limit of **20 MiB**.
- Lazy fetch: Inbound provider media is downloaded only upon first inspection.
- Storage backends: Local disk directory or S3-compatible object storage (e.g. MinIO).
- Automated retention policy enforced by background cleanup:

| Media Size | Retention Duration |
| --- | --- |
| Up to 1 MiB | 7 days |
| > 1 MiB to 10 MiB | 24 hours |
| > 10 MiB to 20 MiB | 1 hour |
| Over 20 MiB | Rejected |

Expired provider media is fetched again on demand when the remote provider link remains valid.

---

## Local Development

### Prerequisites

- [Docker](https://docs.docker.com/get-docker/) and [Docker Compose](https://docs.docker.com/compose/)
- [Go](https://go.dev/) 1.27+ (or system Go via `source scripts/codex-env.sh`)
- [Node.js](https://nodejs.org/) 20+ and npm

### Starting the Development Stack

1. **Initialize configuration**:
   ```bash
   cp .env.example .env
   ```

2. **Start the local Docker Compose stack**:
   ```bash
   make up
   # or: docker compose up -d --build
   ```
   *Note: Database migrations run automatically via the `migrate` container before services accept requests.*

3. **Check logs**:
   ```bash
   make logs
   ```

4. **Stop the stack**:
   ```bash
   make down
   ```

The local development stack binds the following host ports:
- Web Gateway: `http://localhost:18080` (API Gateway)
- PostgreSQL: `localhost:5432`
- Redis: `localhost:6379`
- MinIO API: `http://localhost:9000` (Console: `http://localhost:9001`)

### Connecting Messaging Channels

Navigate to **Settings → Channels** in the web application:

- **WhatsApp**: Click *Add Channel* → *WhatsApp*. A QR code will be generated. Scan it in WhatsApp under *Linked Devices*. Once paired, the status changes to `connected`.
- **Telegram**: Create a bot using [`@BotFather`](https://t.me/BotFather) on Telegram. Enter the bot token in WhatFunnel. The token is validated, AES-256 encrypted in the adapter's private database, and scrubbed from the browser. Send a message to your bot in Telegram to initiate the chat.

### Frontend Development

To run the SvelteKit development server with hot-module reloading:

```bash
cd apps/web
npm install
npm run dev
```

Visit `http://localhost:5173` to access the development UI.

---

## Testing & Quality Assurance

Before running tests, always initialize the repository test environment:

```bash
source scripts/codex-env.sh
```

### Unit & Integration Tests

```bash
# Fast unit tests (no database or external services required)
source scripts/codex-env.sh && make test-short

# Full test suite (requires dev stack running via `make up`)
source scripts/codex-env.sh && make test

# Verbose test run
source scripts/codex-env.sh && make test-verbose

# Chaos / failover tests (SIGKILL takeover, Redis pause/resume, Postgres restart)
source scripts/codex-env.sh && make test-destructive

# Multi-agent scale fuzz test
source scripts/codex-env.sh && make test-scale-fuzz
```

### Playwright E2E & UI Monkey Fuzzing

The frontend includes standard Playwright end-to-end tests and deterministic monkey fuzz suites:

```bash
# Standard Playwright E2E suite
source scripts/codex-env.sh && make pw

# Interactive Playwright UI runner
source scripts/codex-env.sh && make pw-ui

# Deterministic UI monkey fuzz test (fast mock mode)
make pw-fuzz

# UI monkey testing under network fault injection (500 errors, latency, packet drops)
make pw-fuzz-chaos

# Run all fast UI monkey fuzz suites
make pw-fuzz-all

# Live UI monkey fuzzing against an isolated ephemeral Docker pod
make pw-fuzz-live
```

### Database Migrations & Code Generation

Database schema migrations are written for [Goose](https://github.com/pressly/goose) in [`packages/go-common/migrations`](packages/go-common/migrations), and type-safe Go queries are generated via [sqlc](https://sqlc.dev/):

```bash
# Apply migrations manually
make migrate

# Check migration status or roll back
make migrate-status
make migrate-down

# Validate SQL queries and compile schema
source scripts/codex-env.sh && make sqlc-compile

# Regenerate Go database code
source scripts/codex-env.sh && make sqlc-gen
```

---

## Production Deployment

### Hardened Stack

WhatFunnel provides a hardened, multi-network production configuration in [`docker-compose.prod.yml`](docker-compose.prod.yml):

- **Network Segmentation**: Separates traffic into `public_net`, `backend_net`, `data_net`, and `adapter_net`. Sensitive databases, Redis, and internal microservices are never exposed to public networks.
- **Nginx Ingress**: Static frontend asset serving and reverse-proxying with gzip/brotli compression and security headers.
- **Resource Constraints**: Container limits, non-root users, and healthcheck policies.

To run the production stack:

```bash
cp .env.example .env
# Edit .env with production secrets
make prod-up
```

### Automated Deployment Script

The repository includes an automated VPS deployment script [`deploy.sh`](deploy.sh) that builds binaries, packages dependencies, configures the host, and provisions Cloudflare Zero Trust tunnels:

```bash
# Basic deployment
./deploy.sh

# Deployment with database reset and fresh migrations
./deploy.sh --reset-db

# Ingress via Cloudflare Zero Trust tunnel
./deploy.sh --tunnel-token <CLOUDFLARE_TUNNEL_TOKEN>
```

### Security Checklist & Secrets

Ensure the following variables in `.env` are set to high-entropy random secrets:

1. `SESSION_SECRET`: Minimum 32-character string for signing session cookies (`openssl rand -base64 32`).
2. `ENCRYPTION_KEY`: 32-byte / 64-character hex key for AES-256-GCM data encryption at rest (`openssl rand -hex 32`).
3. `ADAPTER_SHARED_SECRET`: Shared token for internal adapter control endpoints.
4. `INTERNAL_SERVICE_TOKEN`: Minimum 32-character token for inter-service communication.
5. `COOKIE_SECURE=true`: Enforces HTTPS cookies.
6. `ENABLE_SIMULATION_ROUTES=false`: Disables mock/simulation routes in production.

Never commit `.env` or adapter SQLite databases to source control.

---

## Configuration Reference

Key environment variables configured in [`.env.example`](.env.example):

| Variable | Description | Default / Example |
| --- | --- | --- |
| `APP_ENV` | Application environment mode (`development`, `production`, `test`) | `production` |
| `PORT` | Public HTTP port for API gateway / web ingress | `8080` (prod: `80`) |
| `DATABASE_URL` | PostgreSQL connection string | `postgres://whatfunnel:...@postgres:5432/whatfunnel?sslmode=disable` |
| `REDIS_URL` | Redis Streams and Pub/Sub connection URL | `redis://redis:6379` |
| `SESSION_SECRET` | Secret key used to sign HTTP session cookies | (Required, ≥ 32 chars) |
| `ENCRYPTION_KEY` | 64-hex-char AES-256 key for stored provider credentials | (Required, 64 hex chars) |
| `ADAPTER_SHARED_SECRET` | Secret shared between conversation-svc and adapters | (Required) |
| `INTERNAL_SERVICE_TOKEN`| Secret shared across internal backend microservices | (Required, ≥ 32 chars) |
| `MEDIA_STORAGE_BACKEND` | Media storage driver (`disk` or `s3`) | `disk` |
| `MEDIA_CACHE_PATH` | Local disk mount for media files | `/tmp/whatfunnel-media` |
| `MEDIA_S3_ENDPOINT` | MinIO or S3 endpoint | `minio:9000` |
| `GOOGLE_AI_KEY` | API key for Gemini / Google AI models | (Optional if AI features enabled) |
| `MINING_INTERVAL_HOURS` | Interval for automatic conversation knowledge mining | `6` |

---

## Specification

For deeper architectural guarantees, message state lifecycle, adapter idempotency mechanisms, and edge-case handling, see the comprehensive [WhatFunnel Messaging Specification](spec.md).

For development environment setup and agent guidelines, refer to [AGENTS.md](AGENTS.md).
