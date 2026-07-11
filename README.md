# WhatFunnel

A chatbot automation layer + lead management workspace that connects to existing messaging channels via mautrix bridges. See [docs/WHATFUNNEL_SPEC.md](docs/WHATFUNNEL_SPEC.md) for the full product spec.

---

## Architecture overview

```
api-gateway (Go)  →  identity-svc (Go)   — auth, RBAC, audit
                  →  workspace-svc (Go)  — accounts, users, pipeline config
                  →  conversation-svc    — contacts, conversations, messages, leads (Build Prompt 2+)
                  →  notification-svc    — WebSocket push (Build Prompt 4+)
ai-answer-svc (Python) — AI cascade (Build Prompt 4+)
ai-kb-compiler (Python) — KB compilation (Build Prompt 4+)
adapters/matrix-mautrix — channel bridge (Build Prompt 2+)
```

Frontend: SvelteKit (`apps/web/`, Build Prompt 5+).

---

## Tech decisions (Build Prompt 1)

### Go workspace vs per-service modules

We use a **single `go.work` workspace** across `services/*` and `packages/go-common`. This means:

- `go build ./...` and `go test ./...` work from the repo root.
- Shared code in `packages/go-common` is referenced as a local module — no publish step needed during development.
- Each service still has its own `go.mod` (with its own module path) so it can be built/containerised independently.

### Migrations: goose (pressly/goose)

Migration files live in `packages/go-common/migrations/`. Rationale: all four foundation tables (`accounts`, `users`, `audit_logs`, `lead_pipelines`) are used across `identity-svc` and `workspace-svc`, so a single canonical location avoids the chicken-and-egg problem of "which service owns the shared schema?". Each service reads the same shared migration dir via a `MIGRATIONS_DIR` env var that docker-compose injects. The `make migrate` target runs goose against all migrations.

### Auth: sessions (not JWT)

We use `authboss` (volatiletech/authboss) with server-side sessions and signed cookies. Reasons for sessions over JWT:

1. **Revocation is trivial** — destroying a session row logs the user out immediately. JWTs require a deny-list to revoke before expiry.
2. **Desktop-first app** — no cross-domain credential sharing needed; cookies work cleanly.
3. **authboss is session-native** — fighting its design to emit JWTs would add complexity with no gain at this stage.

JWT is a documented fast-follow if mobile clients or cross-domain integrations are added.

### Multi-tenancy isolation: application layer (not Postgres RLS)

**v1 tradeoff:** Tenant isolation is enforced by `ScopedDB` (see `packages/go-common/db/scoped.go`), which requires an `account_id` on every query against tenant-scoped tables. This is explicit, auditable, and fast to build. Postgres RLS would add a second enforcement layer that makes misconfiguration harder — **it is a documented fast-follow**, prioritised for any public cloud deployment.

To enable RLS later: add `ALTER TABLE <table> ENABLE ROW LEVEL SECURITY;` and a policy per table. The `account_id` column and indexes are already in place.

---

## Running locally

### Prerequisites

- Docker + Docker Compose v2
- Go 1.22+
- `goose` CLI: `go install github.com/pressly/goose/v3/cmd/goose@latest`

### Start services

```bash
make up          # docker-compose up -d (postgres, redis, all Go services)
make migrate     # run goose migrations against local postgres
```

### Run tests

```bash
make test        # runs go test ./... across the whole go.work workspace
```

Tests that hit Postgres require the docker-compose stack to be running (`make up` first). The `make test` target waits for Postgres to be healthy before running.

### Stop services

```bash
make down
```

---

## Commit history (Build Prompt 1 stages)

| Commit | Stage |
|---|---|
| `chore: repo skeleton and local dev environment` | Stage 0 |
| `feat(db): foundation schema — accounts, users, audit_logs, lead_pipelines` | Stage 1 |
| `feat(common): application-layer tenant isolation` | Stage 2 |
| `feat(identity): signup, login, logout via authboss` | Stage 3 |
| `feat(common): RBAC middleware, admin/member enforcement` | Stage 4 |
| `feat(workspace): user management and pipeline config` | Stage 5 |
| `feat(workspace): encrypted storage primitive for AI provider config` | Stage 6 |
| `test: foundation end-to-end integration pass` | Stage 7 |

---

## Backlog / fast-follows

- Postgres RLS (row-level security) as a second isolation layer
- JWT auth for mobile/cross-domain clients
- Real email delivery (invite flow currently logs token and stubs SMTP — search for `// TODO: wire to email provider`)
- Redis Streams wiring (redis container is running but nothing publishes/consumes yet)
- SvelteKit frontend (`apps/web/`)
