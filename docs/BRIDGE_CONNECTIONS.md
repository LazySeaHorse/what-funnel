# Bridge connections

WhatFunnel treats a mautrix login as a connection lifecycle, not as a boolean
channel record. The Conversation Service creates one non-admin Matrix user for
each channel, opens a direct management room with the configured mautrix bot,
and stores the resulting Matrix access token encrypted at rest.

## Supported guided flows

| Platform | Bridge command | User action |
| --- | --- | --- |
| WhatsApp | `login qr` | Scan the QR from WhatsApp Linked devices. |
| Telegram | `login qr` | Scan the QR from Telegram Devices. If prompted, enter the verification code or 2FA password. |
| Instagram | `login` | Provide an authenticated Instagram web-session request to mautrix. |
| Messenger | `login` | Provide an authenticated Messenger web-session request to mautrix. |

The UI polls the bridge management room for a QR, success, verification prompt,
or error. It does not mark a channel as connected merely because a user clicked
Connect.

## Required operator configuration

The connection service is disabled unless all of the following are supplied to
`conversation-svc`:

```dotenv
MATRIX_HOMESERVER_URL=http://synapse:8008
MATRIX_SERVER_NAME=example.com
MATRIX_REGISTRATION_SHARED_SECRET=<synapse registration_shared_secret>
MATRIX_WHATSAPP_BRIDGE_IDENTITY=@whatsappbot:example.com
MATRIX_TELEGRAM_BRIDGE_IDENTITY=@telegrambot:example.com
MATRIX_INSTAGRAM_BRIDGE_IDENTITY=@instagrambot:example.com
MATRIX_MESSENGER_BRIDGE_IDENTITY=@messengerbot:example.com
```

Each configured bot must be backed by a running, registered bridge on the same
homeserver. The base local compose stack provisions WhatsApp; the optional
`docker-compose.bridges.yml` deployment adds Telegram, Instagram and Messenger.
Bootstrap each one before it is started:

```bash
make bridge-bootstrap BRIDGE=telegram TELEGRAM_API_ID=... TELEGRAM_API_HASH=...
make bridge-bootstrap BRIDGE=messenger
make bridge-bootstrap BRIDGE=instagram
make bridges-up
```

Bootstrap generates separate bridge configs and appservice registrations under
`adapters/matrix-mautrix/bridges/`. `matrix-init` copies their registrations
into Synapse's `app_service_config_files` before Synapse starts. Those files
are intentionally ignored by Git because they contain appservice tokens and
persistent bridge state. The optional compose file sets the matching bot IDs:
`@telegrambot:localhost`, `@instagrambot:localhost`, and
`@messengerbot:localhost`.

Telegram also needs its own `api_id` and `api_hash` from
`my.telegram.org/apps`. These API keys enable the bridge client but do not
grant it access to user accounts.

## Meta-session handling

Instagram and Messenger do not offer a standard OAuth hand-off for the mautrix
flow. The bridge uses an authenticated web session, as documented by mautrix.
WhatFunnel forwards the submitted cURL payload directly to the bridge and does
not put it in its application database or logs. However, that message is part
of the Matrix bridge-management workflow. Operators must set an appropriate
retention policy and configure end-to-bridge encryption where their mautrix
version and client architecture support it.

Use a dedicated bridge-management deployment and a restricted administrator
role. Never expose `MATRIX_REGISTRATION_SHARED_SECRET`, Matrix access tokens,
or Meta session material to browser code, analytics, error reporting, or
general application logs.
