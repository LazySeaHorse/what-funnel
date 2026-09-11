# WhatFunnel messaging specification

## Scope

The unified inbox supports one-to-one conversations and multiple accounts per
provider in each workspace. The first production adapter is WhatsApp via
`whatsmeow`. Telegram Bot API is next. Instagram and Facebook Messenger are
disabled and labeled coming soon until their official-API designs are implemented.

There is no chat-history import and no compatibility layer for the previous
messaging architecture.

## Service boundaries

The provider adapter owns authentication, persistent provider sessions, native
event decoding, media transfer, and native sends. It runs in its own Docker
container. The conversation service owns authorization, contacts, conversations,
messages, reactions, delivery state, cached-media metadata, and the transactional
outbound outbox. It must not import provider SDKs or store provider credentials.

The only asynchronous contract is the versioned Go package at
`packages/go-common/messaging`:

- adapters publish `messaging.Event` envelopes to `adapter.events`;
- the conversation service publishes `messaging.Command` envelopes to
  `adapter.commands` through a PostgreSQL transactional outbox;
- every envelope carries a schema version, unique ID, provider, channel ID, and timestamp;
- consumers are idempotent and stale Redis pending messages are reclaimed.

The control plane is an authenticated internal HTTP API used to create, inspect,
and unlink provider sessions. It is separate from message traffic.

## Unified message model

Supported content types are `text`, `image`, `video`, `audio`, `document`, and
`notice`. Messages preserve provider IDs and timestamps, direction, external
direct-chat identity, sender, optional caption, media metadata, and reply target.

Event kinds are message created/edited/deleted, reaction changed, receipt
changed, and channel status changed. Command kinds are send/edit/delete and
reaction change. Unsupported provider content becomes a `notice` instructing the
user to open the provider app. Provider-specific payloads must not leak into
domain or UI APIs. Typing indicators are out of scope.

## WhatsApp adapter

Each connection maps to an independent whatsmeow device in a private SQLite
store. The browser receives QR image bytes, never session secrets. Restarts
restore sessions. Only direct JIDs are accepted; group events are ignored.
Adapter events use a durable SQLite outbox so Redis interruption cannot discard
traffic. Processed commands are recorded for idempotency.

Automated tests normalize synthetic events and use fake publishers. Connection
and delivery against real WhatsApp servers are manual acceptance tests.

## Delivery guarantees

Creating an outbound message and its command-outbox row is one PostgreSQL
transaction. A dispatcher claims ready rows, publishes commands, retries with
backoff, and records permanent failure. A successful adapter send emits a
correlated message-created event that records the provider message ID.

Adapter event processing and its idempotency marker commit in one transaction,
so Redis redelivery cannot duplicate a domain message or reaction.

## Media policy

The maximum inbound or outbound media size is 20 MiB. Retention is seven days at
or below 1 MiB, 24 hours above 1 MiB through 10 MiB, and one hour above 10 MiB
through 20 MiB. Provider media is downloaded lazily. Expired bytes are removed
and fetched again where the provider reference remains usable.

Cached bytes live in a private Docker volume owned by `conversation-svc`.
Browser reads/uploads require a workspace session; adapter reads use the shared
secret. Files use owner-only permissions and are served with sniffing disabled.

## Deployment

Docker Compose is the supported orchestrator. Kubernetes is out of scope. The
messaging deployment adds `whatsapp-adapter`, its private SQLite volume, and the
conversation media volume. PostgreSQL is the system of record and Redis Streams
is the event transport.

Production requirements:

- generate a high-entropy `ADAPTER_SHARED_SECRET`;
- do not publish adapter, database, Redis, or internal service ports;
- back up the adapter volume if preserving paired sessions is desired;
- monitor connection state, outbox retries/failures, Redis pending entries, and cache disk use;
- roll out schema, adapter, conversation service, gateway, then web UI.

## Acceptance criteria

1. A workspace can connect, inspect, and unlink multiple labeled WhatsApp accounts.
2. Direct inbound text and supported media create one normalized timeline item.
3. Outbound text and media expose queued/sent/delivered/read/failed state.
4. Duplicate commands and events do not duplicate sends or messages.
5. Unsupported content creates an actionable notice; group traffic is ignored.
6. Media limits, auth, lazy fetch, expiry, and cleanup behave as specified.
7. Restarts restore sessions and unfinished PostgreSQL/Redis/SQLite work.
