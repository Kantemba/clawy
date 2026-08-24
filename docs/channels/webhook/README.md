# Custom Webhook

Generic inbound HTTP webhook channel served from Clawy's shared gateway HTTP
server. External systems — home automation, CI pipelines, IoT devices, cron
wrappers, scripts — POST JSON or plain-text payloads and Clawy processes them
like any other chat message.

## Configuration

```json
{
  "channels": {
    "hooks": {
      "enabled": true,
      "type": "webhook",
      "settings": {
        "token": "your-shared-secret",
        "path": "/webhook/hooks",
        "reply_url": "http://localhost:8123/api/clawy-reply"
      }
    }
  }
}
```

| Field | Required | Description |
|-------|----------|-------------|
| `token` | No | Shared secret. When set, callers must present it via `?token=`, the `X-Clawy-Token` header, or `Authorization: Bearer`. Strongly recommended whenever the gateway is reachable beyond localhost. |
| `path` | No | HTTP mount path. Defaults to `/webhook/<channel name>` so multiple webhook instances never collide. |
| `reply_url` | No | Endpoint that receives agent replies as JSON POSTs (`{"chat_id", "content", "session_key"}`). When omitted, replies are dropped (fire-and-forget triggers). HTTPS is enforced for non-loopback hosts; plain `http://` is allowed only for `localhost` / `127.0.0.1`. |

## Sending a message

JSON (with optional routing fields):

```bash
curl -X POST http://127.0.0.1:18790/webhook/hooks \
  -H "Content-Type: application/json" \
  -H "X-Clawy-Token: your-shared-secret" \
  -d '{"message": "What is on my calendar today?", "chat_id": "morning-brief", "sender_id": "scheduler"}'
```

Plain text also works:

```bash
curl -X POST http://127.0.0.1:18790/webhook/hooks \
  -H "Content-Type: text/plain" \
  -H "Authorization: Bearer your-shared-secret" \
  -d "deploy finished"
```

The endpoint answers `{"ok":true,"chat_id":"..."}` once the message is queued.
Each `chat_id` gets its own agent session, so distinct integrations can keep
separate conversation threads. Access is additionally governed by the shared
`allow_from` list on the channel entry.

## Security notes

- Set `token` in production. It is compared in constant time.
- Request bodies are capped at 1 MiB.
- `reply_url` must use HTTPS unless it points at loopback.

## See also

- [Chat Apps](../../guides/chat-apps.md) for all channels
- [Configuration](../../guides/configuration.md) for gateway host/port settings
