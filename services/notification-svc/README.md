# services/notification-svc — Notification Service

> **Stub** — built in Build Prompt 4 (Realtime).

WebSocket push service. Pushes `message.received`, `conversation.assigned`, `lead.state_changed`, etc. to connected browser clients.

Summary events: `conversation.summary_updated` (`conversation_id`, `summary_fields`, `generated_at`,
`message_count_at_generation`) goes to every connected user who can see the conversation. `conversation.summary_failed`
(`conversation_id`, `error_code`, `message`) goes only to the user who requested the summary.
