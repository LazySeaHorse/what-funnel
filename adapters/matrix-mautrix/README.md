# Matrix/mautrix Channel Adapter for WhatsApp

This adapter connects What Funnel to a Matrix homeserver (Synapse) and listens to events from the `mautrix-whatsapp` bridge. It normalizes events (such as text, images, video, audio, files, locations, reactions, and contact cards) into standard format messages and publishes them to Redis Streams. It also handles outbound sending.

## Synapse & Bridge Configuration

Setting up Synapse and the `mautrix-whatsapp` bridge locally involves the following steps:

### 1. Synapse Configuration
Synapse serves as the Matrix homeserver. Ensure your `homeserver.yaml` has application services enabled:

```yaml
# homeserver.yaml
app_service_config_files:
  - "/data/whatsapp-registration.yaml"
```

### 2. Generate Bridge Registration
The `mautrix-whatsapp` bridge needs to register as an application service. Run the bridge once with the `-g` flag to generate the registration file:

```bash
docker run --rm -v $(pwd)/bridge:/data dock.mau.dev/mautrix/whatsapp:latest -g
```

This generates `whatsapp-registration.yaml` in your bridge directory. It will contain fields like:
```yaml
id: whatsapp
url: http://mautrix-whatsapp:29318
as_token: <generated-application-service-token>
hs_token: <generated-homeserver-token>
sender_localpart: whatsapp
namespaces:
  users:
    - exclusive: true
      regex: '@whatsapp_.*'
```

Copy this registration file to your Synapse config directory so that Synapse knows about the application service.

### 3. Bridge Configuration
Update the bridge config file `config.yaml` to connect to Synapse:

```yaml
# config.yaml
homeserver:
  address: http://synapse:8008
  domain: localhost

appservice:
  address: http://mautrix-whatsapp:29318
  database: postgres://postgres:password@postgres:5432/mautrix_whatsapp?sslmode=disable

whatsapp:
  # Connection settings for multi-device WhatsApp protocol
```

Start the containers (Synapse, Postgres, Redis, and `mautrix-whatsapp`).

### 4. QR Code Login Flow
To connect a WhatsApp account:
1. Open a Matrix client (e.g., Element) and log in to your account.
2. Start a direct message chat with `@whatsapp:localhost` (the bridge user).
3. Send the command `login` to the bridge user.
4. The bridge will respond with a QR code in terminal logs or in the chat itself.
5. Open WhatsApp on your phone, go to **Linked Devices**, select **Link a Device**, and scan the QR code.
6. Once scanned, the bridge will connect and begin bridging all your chats into Matrix rooms.

### 5. What Funnel Connection
Register the channel in What Funnel using the admin endpoints.
- Channel type: `matrix_whatsapp`
- `bridge_identity`: `@whatsapp:localhost`
- `bridge_credentials`:
  ```json
  {
    "homeserver_url": "http://localhost:8008",
    "user_id": "@admin:localhost",
    "access_token": "syt_your_access_token_here"
  }
  ```
The adapter will automatically sync with Synapse, listen for new events, and normalize them into What Funnel conversations.
