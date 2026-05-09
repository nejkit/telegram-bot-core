# Deployment Reference: Go Telegram Bots

> **Note:** `github.com/nejkit/telegram-bot-core` uses long polling exclusively. Webhook deployments are not applicable; the bot connects outbound to Telegram's API.

## Docker Multi-Stage Build

```dockerfile
# Build stage
FROM golang:1.23-alpine AS build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o bot ./cmd/bot

# Runtime stage
FROM alpine:3.20
RUN apk --no-cache add ca-certificates tzdata
WORKDIR /app
COPY --from=build /app/bot .
COPY locales/ locales/
CMD ["./bot"]
```

```yaml
# docker-compose.yml
services:
  bot:
    build: .
    restart: unless-stopped
    environment:
      - BOT_TOKEN=${BOT_TOKEN}
      - ALLOWED_UPDATES=message,callback_query,chat_member,my_chat_member
      - WORKERS_COUNT=4
      - MESSAGE_PER_SECOND=-1
      - LOCALIZATION_FILE_PATH=/app/locales/messages.json
    volumes:
      - ./locales:/app/locales:ro
```

---

## Fly.io (free tier friendly)

```bash
fly launch --name my-telegram-bot
fly secrets set BOT_TOKEN=<token> WORKERS_COUNT=4 ALLOWED_UPDATES=message,callback_query
fly deploy
```

`fly.toml`:
```toml
[build]
  dockerfile = "Dockerfile"
```

No webhook registration needed — the bot polls Telegram outbound.

---

## Railway

1. Push to GitHub, connect repo in Railway dashboard.
2. Set env vars: `BOT_TOKEN`, `WORKERS_COUNT`, `ALLOWED_UPDATES`.
3. Railway auto-detects Go and builds with `go build ./...`.
4. Set the start command in `Procfile`: `worker: ./mybot`

---

## VPS with systemd

Run the bot as a systemd service:

```ini
# /etc/systemd/system/telegrambot.service
[Unit]
Description=Telegram Bot
After=network.target

[Service]
User=botuser
WorkingDirectory=/opt/mybot
ExecStart=/opt/mybot/bot
Restart=on-failure
EnvironmentFile=/opt/mybot/.env

[Install]
WantedBy=multi-user.target
```

```bash
systemctl enable --now telegrambot
```

---

## Long Polling vs Webhook: When to use each

| Scenario | Recommendation |
|---|---|
| Local development | Long polling (no tunnel needed) |
| Low traffic, simple VPS | Long polling |
| Serverless (Cloud Run, Lambda) | Not recommended — long polling requires a persistent process |
| Multiple replicas / horizontal scale | Long polling + Redis state store |
| Highest reliability, single instance | Long polling (no missed updates) |

---

## Environment Configuration

Use `github.com/caarlos0/env/v11` to parse `config.TelegramConfig` from environment:

```go
import (
    "github.com/caarlos0/env/v11"
    "github.com/nejkit/telegram-bot-core/config"
)

cfg := config.TelegramConfig{}
if err := env.Parse(&cfg); err != nil {
    log.Fatalf("config: %v", err)
}
```

### Environment Variable Reference

| Env Var | Default | Notes |
|---|---|---|
| `BOT_TOKEN` | required | Telegram bot token |
| `ALLOWED_UPDATES` | (all types) | Comma-separated, e.g. `message,callback_query,chat_member` |
| `WORKERS_COUNT` | `1` | Parallel chat workers; set to CPU count for production |
| `MESSAGE_PER_SECOND` | `-1` | Per-user rate limit (msgs/sec); `-1` = disabled |
| `LOCALIZATION_FILE_PATH` | `` | Path to `messages.json`; empty = no localization |
| `TELEGRAM_API_URL` | `https://api.telegram.org/bot%s/%s` | Override for a local Bot API server |

---

## Structured Logging (Go 1.21+)

```go
import "log/slog"

logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
slog.SetDefault(logger)

slog.Info("bot started", "workers", cfg.WorkersCount)
slog.Error("handler failed", "chatID", chatID, "err", err)
```

The library uses `logrus` internally; both will write to stdout.
