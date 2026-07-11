# Build Prompt 1: Foundation

**Read `docs/WHATFUNNEL_SPEC.md` in full before writing any code.** That document is authoritative for data model field names, service boundaries, and tech stack choices. If anything in this prompt seems to contradict it, the spec wins — stop and flag the conflict rather than guessing.

This prompt covers **only** the foundation layer: repo skeleton, Postgres schema for accounts/users/audit/pipelines, auth, and RBAC. It does not cover channels, conversations, messages, leads (beyond the pipeline config table), or anything AI-related. Do not build ahead of scope — later prompts depend on this layer being solid and unambiguous, not on extra features bolted on early.

---

## 0. Non-goals for this prompt (explicitly do not build)

- No `channels`, `contacts`, `conversations`, `messages`, `leads`, `kb_concepts`, `patterns`, or `automation_suggestions` tables.
- No Matrix/mautrix adapter code.
- No AI service code.
- No frontend UI beyond what's needed to manually exercise auth during testing (a bare login/signup form is fine if useful for your own verification; do not polish it).
- No Redis Streams wiring yet — Redis can appear in docker-compose as a placeholder service, but nothing needs to publish/consume from it in this prompt.

---

## 1. Repo skeleton

Create this monorepo structure. Leave stub `README.md` files in directories that later prompts will fill in, so the shape is visible from the start.

```
whatfunnel/
  apps/
    web/                      # stub only — later prompt (SvelteKit)
  services/
    api-gateway/               # Go — build this
    identity-svc/               # Go — build this
    workspace-svc/              # Go — build this
    conversation-svc/          # stub only
    notification-svc/          # stub only
    ai-answer-svc/              # stub only (Python later)
    ai-kb-compiler/             # stub only (Python later)
  adapters/
    matrix-mautrix/             # stub only
  packages/
    go-common/                  # Go — build this (shared middleware/types)
  docs/
    WHATFUNNEL_SPEC.md          # copy the spec in here
  docker-compose.yml
  Makefile
  README.md
```

Set up Go modules per service (or a shared go.work workspace across `services/*` and `packages/go-common` — your call, pick whichever keeps `go build ./...` and `go test ./...` working cleanly across the monorepo, and document the choice in the root README).

---

## 2. Tech choices for this layer (assumptions — flag if you deviate)

- **Migrations**: use `goose` (pressly/goose) for Postgres migrations. Migration files live in `services/workspace-svc/migrations` or a shared `packages/go-common/migrations` if that's cleaner given cross-service tables — pick one, document it.
- **Auth**: `authboss` (volatiletech/authboss) inside `identity-svc`. Session-based auth (not JWT) is fine for v1 — cookies work for a desktop-first web app and keep revocation simple. If you have a strong reason to prefer JWT, state it before implementing, otherwise default to sessions.
- **DB driver**: `pgx` (jackc/pgx).
- **Testing**: unit tests with the standard library + `testify`. For anything touching Postgres, use a real Postgres via docker-compose (a `docker-compose.test.yml` override or a `make test` target that spins up a disposable test DB) — do not mock the database for schema/query-level tests.
- **API style**: plain REST/JSON over HTTP for now (no gRPC), served from `api-gateway`, which proxies to `identity-svc` and `workspace-svc`. Internal service-to-service calls can be plain HTTP for v1 — no need for a service mesh at this stage.

---

## 3. Data model for this prompt

Implement exactly these tables (field names/types per spec §4, expanded here with the practical detail needed to migrate):

```sql
accounts (
  id uuid primary key default gen_random_uuid(),
  name text not null,
  plan text not null default 'self_hosted',
  ai_provider_config jsonb,        -- encrypted at rest, see §5 below
  settings jsonb not null default '{}',
  created_at timestamptz not null default now()
)

users (
  id uuid primary key default gen_random_uuid(),
  account_id uuid not null references accounts(id),
  email text not null,
  password_hash text not null,      -- managed by authboss
  role text not null check (role in ('admin', 'member')),
  created_at timestamptz not null default now(),
  unique (account_id, email)
)

audit_logs (
  id uuid primary key default gen_random_uuid(),
  account_id uuid not null references accounts(id),
  actor_user_id uuid references users(id),
  action text not null,
  target_type text not null,
  target_id uuid,
  metadata jsonb not null default '{}',
  created_at timestamptz not null default now()
)

lead_pipelines (
  id uuid primary key default gen_random_uuid(),
  account_id uuid not null references accounts(id),
  name text not null,
  states jsonb not null,             -- ordered array of {key, label, color}
  created_at timestamptz not null default now()
)
```

Enable the `pgvector` extension in this migration set even though nothing uses it yet (`CREATE EXTENSION IF NOT EXISTS vector;`) — later prompts assume it's already available and you shouldn't need to touch this layer again just to turn it on.

Every table gets an index on `account_id`. This is the tenant-isolation boundary — get it right here, since every later table follows the same pattern.

---

## 4. Application-layer tenant isolation

Per spec §9, isolation is enforced at the application layer, not Postgres RLS, for v1. Concretely:

- Build a query-scoping helper in `packages/go-common` that every service uses — something like a `ScopedDB` wrapper that takes `account_id` once (from the authenticated session/request context) and refuses to run any query against a tenant-scoped table without it.
- Write at least one test that proves a request authenticated as Account A cannot read/write a row belonging to Account B, even if it guesses the row's UUID.
- Document this as a known v1 tradeoff in the root README (Postgres RLS is a flagged fast-follow, not required now).

---

## 5. `ai_provider_config` encryption

This column will hold a BYO OpenAI-compatible API key later (not used in this prompt, but the column exists now). Encrypt it at the application layer before writing (AES-GCM with a key from environment config is sufficient for v1) rather than storing it in plaintext, even though nothing populates it yet. Write a round-trip test (encrypt → store → read → decrypt) even though no feature calls it yet — better to prove the primitive works now than discover it's broken when the AI prompt needs it.

---

## 6. RBAC

Two roles only: `admin`, `member`, per spec §8.

- Signup flow: the first user created for a new account is always `admin`. There is no "invite yourself as member" path for account creation.
- Admin can invite additional users (as `admin` or `member`) via email — for this prompt, a real email send isn't required; generate an invite token, log/return it in the API response, and leave actual email delivery as a documented stub (`// TODO: wire to email provider`).
- Middleware in `packages/go-common` should expose something like `RequireRole(admin)` / `RequireAuthenticated()` that `api-gateway` and services apply to routes. Write tests proving a `member` gets a 403 on admin-only routes (account settings, user management, pipeline config) and an `admin` doesn't.

---

## 7. Audit logging

Every state-changing action in this prompt's scope writes an `audit_logs` row: account creation, user creation/invite, role change, login, logout, pipeline creation/edit. Build this as a small helper (`packages/go-common/audit`) that services call explicitly at the point of mutation — do not try to make it automatic/reflective, explicit calls are easier to reason about and test.

---

## 8. Default pipeline seeding

When an account is created, seed one default `lead_pipelines` row so the account isn't in a broken empty state. Reasonable default states: `New`, `Contacted`, `Follow-up`, `Won`, `Lost`. This is a placeholder — later, admins can edit/replace it; this prompt just needs it to exist so nothing downstream breaks on a null pipeline.

---

## 9. Build stages — test and commit at each gate

Work through these in order. **At the end of each stage: run the full test suite, confirm it passes, then commit with a clear message before moving to the next stage.** Do not batch multiple stages into one commit — if something breaks later, we need to be able to bisect.

**Stage 0 — Skeleton**
Repo structure, go.work/modules, docker-compose.yml (postgres w/ pgvector image, redis placeholder, api-gateway, identity-svc, workspace-svc), Makefile with at least `make up`, `make test`, `make migrate`. Commit: `chore: repo skeleton and local dev environment`.

**Stage 1 — Schema**
Goose migrations for all four tables above, pgvector extension enabled, indexes in place. Test: migrations run cleanly up and down. Commit: `feat(db): foundation schema — accounts, users, audit_logs, lead_pipelines`.

**Stage 2 — Tenant isolation helper**
`ScopedDB` in `go-common`, plus the cross-tenant-leak test from §4. Commit: `feat(common): application-layer tenant isolation`.

**Stage 3 — Identity Service: signup/login/logout**
Authboss wired up, session cookies, signup creates account + admin user + default pipeline (ties in §8) in one transaction, login/logout work, passwords hashed correctly (verify authboss's default is bcrypt or equivalent — don't roll your own). Tests cover: successful signup, duplicate email rejection, login with correct/incorrect password, session persists across requests, logout invalidates session. Commit: `feat(identity): signup, login, logout via authboss`.

**Stage 4 — RBAC middleware**
`RequireRole`/`RequireAuthenticated` in go-common, applied to a couple of representative protected routes (account settings, pipeline config) to prove it works end-to-end through `api-gateway`. Tests per §6. Commit: `feat(common): RBAC middleware, admin/member enforcement`.

**Stage 5 — Workspace Service: users & pipeline management**
Admin can list users, invite a user (token-based, email stubbed), change a user's role, view/edit the account's pipeline. All of these write audit log entries (§7). Tests cover the happy paths plus the RBAC-denial paths (member attempting admin actions). Commit: `feat(workspace): user management and pipeline config`.

**Stage 6 — `ai_provider_config` encryption primitive**
Round-trip encrypt/decrypt test per §5, wired into the account settings update path even though nothing in the UI/API surfaces it as a real feature yet (a raw field accepting a string is fine). Commit: `feat(workspace): encrypted storage primitive for AI provider config`.

**Stage 7 — Integration pass**
One end-to-end test that: creates an account via signup, confirms default pipeline exists, invites a second user as member, logs in as that member, confirms they're denied on an admin route, confirms they can be assigned appropriately once later prompts add conversations (this last part will just be a placeholder assertion for now since conversations don't exist — note it as `// extended in Build Prompt 3`). Commit: `test: foundation end-to-end integration pass`.

---

## 10. Definition of done

- [ ] `make up` brings up postgres, redis, api-gateway, identity-svc, workspace-svc cleanly on a fresh machine
- [ ] `make test` runs the full suite (unit + integration) and passes
- [ ] Signup → login → logout works via HTTP calls against `api-gateway`
- [ ] Admin/member RBAC is enforced and tested
- [ ] Cross-tenant data access is proven impossible by a test, not just by inspection
- [ ] `ai_provider_config` is encrypted at rest, proven by a round-trip test
- [ ] Every state-changing action in scope writes an audit log row
- [ ] Every stage above has its own commit; no squashed mega-commit
- [ ] Root README documents: migration tool choice, session vs JWT choice, RLS-deferred tradeoff, and how to run tests locally

Do not start Build Prompt 2 (Channel Ingestion) until every box above is checked.
