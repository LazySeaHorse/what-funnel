# Build Prompt 2: Channel Ingestion

**Read `docs/WHATFUNNEL_SPEC.md` in full before writing any code.** Also read `BUILD_PROMPT_1_FOUNDATION.md` and confirm Build Prompt 1's definition-of-done is fully satisfied in the current repo state before starting this one — this prompt builds directly on the tenant isolation helper, RBAC middleware, and audit logging from that layer. Reuse them; do not reimplement.

This prompt covers: the normalized message model, the `ChannelAdapter` contract, a working Matrix/mautrix adapter for **WhatsApp only** (via `mautrix-whatsapp`), the Redis Streams ingestion pipeline, and the Conversation Service's persistence layer (contacts, conversations, messages). It does not cover the inbox UI, leads, AI processing, or any bridge type beyond WhatsApp.

---

## 0. Non-goals for this prompt

- No frontend UI (Build Prompt 3).
- No lead management (Build Prompt 4).
- No AI processing of inbound messages — `ai-answer-svc` stays a stub. A message can be persisted and sit there unanswered; that's correct behavior for this prompt.
- No Instagram/Messenger/Telegram bridges yet. Build the adapter interface generically, but only wire up WhatsApp. Adding another `mautrix-*` bridge later should mean writing a new adapter implementation against the same interface, not touching this prompt's code.
- No webchat adapter (deferred per spec §3).
- No official (non-bridge) API integrations.
- No contact merge UI/logic beyond the schema column existing — merge is a manual future action, not built here.

---

## 1. Why WhatsApp only, and why that's fine

The `ChannelAdapter` interface (spec §5.1) is the seam that isolates the rest of the system from Matrix specifics. Proving that seam works with one real bridge is more valuable right now than half-wiring four. Get WhatsApp solid — real inbound messages, real outbound sends, correct content-type normalization — and the pattern for adding Instagram/Messenger/Telegram later is "implement the same interface again," not an architecture change.

---

## 2. Data model for this prompt

Implement exactly these tables, per spec §4:

```sql
channels (
  id uuid primary key default gen_random_uuid(),
  account_id uuid not null references accounts(id),
  type text not null check (type in ('matrix_whatsapp', 'matrix_instagram', 'matrix_messenger', 'matrix_telegram', 'webchat')),
  bridge_identity text,                  -- e.g. the Matrix user ID this bridge instance operates as
  bridge_credentials jsonb,               -- encrypted at rest, see §3
  status text not null default 'disconnected' check (status in ('connected', 'disconnected', 'error')),
  status_detail text,                     -- human-readable error/status message, nullable
  created_at timestamptz not null default now()
)

contacts (
  id uuid primary key default gen_random_uuid(),
  account_id uuid not null references accounts(id),
  channel_id uuid not null references channels(id),
  external_identity text not null,        -- e.g. WhatsApp JID
  display_name text,
  avatar_url text,
  merged_into_contact_id uuid references contacts(id),
  created_at timestamptz not null default now(),
  unique (channel_id, external_identity)
)

conversations (
  id uuid primary key default gen_random_uuid(),
  account_id uuid not null references accounts(id),
  contact_id uuid not null references contacts(id),
  channel_id uuid not null references channels(id),
  status text not null default 'open' check (status in ('open', 'closed')),
  assigned_user_ids uuid[] not null default '{}',
  last_message_at timestamptz,
  ai_mode_active boolean not null default true,
  created_at timestamptz not null default now(),
  unique (contact_id, channel_id)
)

messages (
  id uuid primary key default gen_random_uuid(),
  account_id uuid not null references accounts(id),
  conversation_id uuid not null references conversations(id),
  direction text not null check (direction in ('inbound', 'outbound')),
  sender_type text not null check (sender_type in ('contact', 'human', 'ai')),
  sender_user_id uuid references users(id),
  content_type text not null check (content_type in ('text', 'image', 'video', 'audio', 'document', 'reaction', 'location', 'contact')),
  content jsonb not null,
  external_message_id text,
  created_at timestamptz not null default now()
)
```

Index `account_id` on all four (consistent with Build Prompt 1's pattern). Also index `messages(conversation_id, created_at)` for chronological reads, and `conversations(account_id, last_message_at)` for inbox sorting later.

---

## 3. Bridge credential storage

`channels.bridge_credentials` holds whatever `mautrix-whatsapp` needs to keep a session alive (this will include things like the WhatsApp multi-device session data). Encrypt it using the same AES-GCM helper built in Build Prompt 1 for `ai_provider_config` — reuse the primitive, don't duplicate it. If the existing helper is account-scoped in a way that doesn't fit channel-scoped secrets cleanly, generalize it rather than forking it.

---

## 4. `ChannelAdapter` contract

Implement exactly the interface from spec §5.1 in `packages/go-common`:

```go
type NormalizedMessage struct {
  ContentType       string // text|image|video|audio|document|reaction|location|contact
  Text              string
  MediaURL          string
  ReplyToExternalID string
}

type ContactRef struct {
  ExternalIdentity string
  DisplayName      string
  AvatarURL        string
}

type InboundEvent struct {
  ChannelID        string
  ExternalThreadID string
  Contact          ContactRef
  Message          NormalizedMessage
  Timestamp        time.Time
}

type ChannelStatus struct {
  Status string // connected|disconnected|error
  Detail string
}

type ChannelAdapter interface {
  Start(ctx context.Context, publish func(InboundEvent)) error
  SendMessage(ctx context.Context, channelID, externalThreadID string, msg NormalizedMessage) error
  Status(channelID string) ChannelStatus
}
```

Build two implementations:

1. **`adapters/matrix-mautrix`** — the real thing, talks to a Matrix homeserver over the Matrix client-server API (Synapse locally), listens for events in bridged rooms, maps them to `NormalizedMessage`, calls `publish`. `SendMessage` posts an `m.room.message` (or appropriate event type for non-text content) into the corresponding Matrix room.
2. **`adapters/fake`** — a test-only adapter that lets tests call a method like `SimulateInbound(event InboundEvent)` to push synthetic events through the exact same pipeline, and records everything passed to `SendMessage` so tests can assert on outbound behavior. **This is what CI uses.** Real `mautrix-whatsapp` requires a live QR-code login per WhatsApp account and cannot run unattended in CI — document this clearly in the README, and make sure nothing in the automated test suite depends on a live bridge connection.

### Content-type mapping (Matrix → normalized)

Map at minimum: `m.text` → `text`, `m.image` → `image`, `m.video` → `video`, `m.audio` → `audio`, `m.file` → `document`, `m.location` → `location`, reactions (`m.reaction` relation) → `reaction`, shared contact cards (bridge-specific event type from `mautrix-whatsapp`) → `contact`. If `mautrix-whatsapp` doesn't cleanly expose one of these (e.g. contact cards may need bridge-specific handling), fall back to `document` with a note in the code rather than silently dropping the message — every inbound event must produce a persisted message, never a silent no-op.

---

## 5. Redis Streams wiring

- Stream `messages.inbound`: adapters publish `InboundEvent` (JSON) here after normalization.
- Stream `conversation.updated`: Conversation Service publishes here after persisting, for Notification Service / AI service consumption in later prompts.
- Build a thin Go wrapper in `go-common` for stream publish/consume with consumer groups, since every service touching Redis Streams will need this. Use consumer groups from the start (not raw `XREAD`) so multiple consumers of the same stream don't duplicate work — this matters once notification-svc and ai-answer-svc both need `conversation.updated` later.

---

## 6. Conversation Service: ingestion consumer

Build the consumer in `services/conversation-svc` that:

1. Reads `messages.inbound`.
2. Upserts the `contacts` row (`channel_id` + `external_identity`, unique constraint per §2 — on conflict, update `display_name`/`avatar_url` if changed, don't create a duplicate).
3. Upserts the `conversations` row (`contact_id` + `channel_id`, unique constraint — on conflict, update `last_message_at`).
4. Inserts the `messages` row (`direction=inbound`, `sender_type=contact`).
5. Publishes to `conversation.updated`.

All of this happens inside a single DB transaction per inbound event — a partially-applied event (contact created but message missing, etc.) is worse than a retried event, so wrap accordingly and make the consumer safe to reprocess (idempotent on `external_message_id` where the bridge provides one — dedupe on that if present).

---

## 7. Outbound send path

Expose an internal API endpoint (used by later prompts — the inbox UI and eventually the AI service) on `conversation-svc`:

```
POST /internal/conversations/{id}/send
{ "content_type": "text", "text": "...", "sender_type": "human"|"ai", "sender_user_id": "..." (if human) }
```

This endpoint: looks up the conversation's channel and external thread ID, calls `ChannelAdapter.SendMessage`, and on success persists an outbound `messages` row and publishes `conversation.updated`. RBAC-gate this behind `RequireAuthenticated()` at minimum (finer-grained assignment-based access control comes with the inbox UI in Build Prompt 3 — don't over-build authorization logic here for a UI that doesn't exist yet). Every send through this path writes an audit log entry (reuse the Build Prompt 1 helper).

---

## 8. Channel connection lifecycle

Admin-only endpoints (reuse `RequireRole(admin)` from Build Prompt 1) to:
- Register a new channel (`POST /channels` — creates the row with `status=disconnected`, kicks off the bridge connection flow).
- Check channel status (`GET /channels/{id}` — reflects live `Status()` from the adapter, not just the DB row, since bridge state can drift).
- Disconnect a channel.

For the real WhatsApp adapter, "connecting" means surfacing whatever `mautrix-whatsapp` needs for QR-code login back to the caller (this will likely mean returning a QR code payload/image the admin scans with their phone). Build this as a real flow, but note in the README that end-to-end testing of the actual QR login is manual/local, not part of the CI suite (per §4's fake-adapter note).

---

## 9. Docker Compose additions

Add to `docker-compose.yml`:
- Synapse (Matrix homeserver), minimal config for local dev.
- `mautrix-whatsapp` bridge container, registered as an application service against Synapse.
- Real Redis wiring (was a placeholder in Build Prompt 1 — now actually used).

Document the local setup steps (homeserver config, appservice registration file, bridge config) in a `adapters/matrix-mautrix/README.md` — this is fiddly to get running and whoever picks this repo up next will need the steps written down, not rediscovered.

---

## 10. Build stages — test and commit at each gate

**Stage 0 — Schema**
Migrations for `channels`, `contacts`, `conversations`, `messages`. Test: migrate up/down cleanly. Commit: `feat(db): channel ingestion schema`.

**Stage 1 — Adapter contract + fake adapter**
`ChannelAdapter` interface and types in `go-common`, plus `adapters/fake` with `SimulateInbound` and send-capture. Tests: fake adapter round-trips an event through the interface correctly. Commit: `feat(common): ChannelAdapter interface and fake test adapter`.

**Stage 2 — Redis Streams wrapper**
Publish/consume helper with consumer groups in `go-common`. Tests against a real Redis (docker-compose). Commit: `feat(common): redis streams pub/sub wrapper`.

**Stage 3 — Conversation Service ingestion consumer**
Consumer logic from §6, using the fake adapter's simulated events as the test input (not live WhatsApp). Tests cover: new contact + new conversation created correctly, existing contact reuses the same conversation on a second message, all content types normalize into a valid `messages` row, idempotency on repeated `external_message_id`. Commit: `feat(conversation-svc): ingestion consumer`.

**Stage 4 — Outbound send path**
Endpoint from §7, tested against the fake adapter (assert `SendMessage` was called with correct params, and that the outbound message persists). Commit: `feat(conversation-svc): outbound send endpoint`.

**Stage 5 — Channel lifecycle endpoints**
Register/status/disconnect from §8, RBAC-gated, encrypted credential storage reusing the Build Prompt 1 primitive. Commit: `feat(conversation-svc): channel connection lifecycle`.

**Stage 6 — Real Matrix/mautrix adapter**
`adapters/matrix-mautrix` implementation per §4, docker-compose additions per §9, setup README. This is the one stage where full automated test coverage isn't realistic (live bridge/QR dependency) — write what can be tested automatically (Matrix event → `NormalizedMessage` mapping, given a captured sample event payload, doesn't need a live connection), and clearly document what requires manual verification. Commit: `feat(adapters): matrix-mautrix WhatsApp adapter`.

**Stage 7 — Integration pass**
End-to-end test using the fake adapter: simulate an inbound WhatsApp-shaped event → confirm contact/conversation/message all persisted correctly → send an outbound reply through the API → confirm it reaches the fake adapter and persists. Commit: `test: channel ingestion end-to-end pass`.

---

## 11. Definition of done

- [ ] `make test` passes fully using only the fake adapter — no test depends on a live bridge connection
- [ ] Content-type normalization covers all seven types from the schema, each with a test
- [ ] Repeated inbound events with the same `external_message_id` don't create duplicate messages
- [ ] Outbound sends persist correctly and are audit-logged
- [ ] Channel credentials are encrypted at rest (reusing, not duplicating, the Build Prompt 1 primitive)
- [ ] `adapters/matrix-mautrix/README.md` documents real local setup (Synapse config, appservice registration, bridge config, QR login flow) clearly enough that a new contributor could get a real WhatsApp account connected
- [ ] Every stage has its own commit
- [ ] Adding a second bridge type later would mean a new file in `adapters/`, not changes to `conversation-svc` or `go-common` — confirm this by checking that nothing in those two places references WhatsApp/Matrix specifics directly

Do not start Build Prompt 3 (Unified Inbox) until every box above is checked.
