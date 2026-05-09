---
name: telegram-bot-go
description: >
  Engineer Telegram bots using the Go (Golang) programming language. Use this skill any time
  the user wants to build, scaffold, extend, or debug a Telegram bot in Go — even if they just say
  "telegram bot", "tgbot", "write a bot", or mention a library like telebot, go-telegram/bot,
  telegram-bot-api, telego, or nejkit/telegram-bot-core. Covers project setup, command routing,
  inline keyboards, callbacks, state machines, middleware, deployment, and best practices.
  Also use when the user wants to port a Python/Node bot to Go, add a new feature to an existing
  Go bot, or troubleshoot bot update handling or rate limiting.
---

# Telegram Bot Engineering in Go

This skill uses `github.com/nejkit/telegram-bot-core` as the standard library. It wraps `github.com/go-telegram-bot-api/telegram-bot-api/v5` with built-in rate limiting, retry transport, a worker-pool state dispatcher, localization, and pluggable storage backends (Redis or in-memory).

---

## Project Bootstrap

### Minimal working bot

```go
package main

import (
    "context"
    "os"
    "os/signal"

    "github.com/caarlos0/env/v11"
    "github.com/dgraph-io/ristretto"
    tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
    "github.com/nejkit/telegram-bot-core/client"
    "github.com/nejkit/telegram-bot-core/config"
    "github.com/nejkit/telegram-bot-core/state"
    "github.com/nejkit/telegram-bot-core/storage"
)

// Step 1: Define domain types — Action ~int, Command ~string, Callback ~string
type Action   int
type Command  string
type Callback string

const (
    ActionNone       Action   = 0        // 0 = no active flow
    ActionAwaitEmail Action   = 1
    CmdStart         Command  = "start"  // no leading slash
    CmdHelp          Command  = "help"
    CbConfirm        Callback = "confirm"
)

var tgClient *client.TelegramClient

func main() {
    ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
    defer cancel()

    // Step 2: Parse config from environment
    cfg := config.TelegramConfig{}
    if err := env.Parse(&cfg); err != nil {
        panic(err)
    }

    // Step 3: Storage — in-memory for dev/single-instance, Redis for production
    cache, _ := ristretto.NewCache(&ristretto.Config{
        NumCounters: 1e7,
        MaxCost:     1 << 30,
        BufferItems: 64,
    })
    actionStore  := storage.NewInMemoryUserActionStorage[Action](cache)
    messageStore := storage.NewInMemoryUserMessageStorage(cache)

    // Step 4: Client
    tgClient = client.NewTelegramClient(&cfg)

    // Step 5: State service + handler registration (chainable)
    state.NewTelegramStateService[Action, Command, Callback](
        cfg, actionStore, messageStore, tgClient, nil,
    ).
        RegisterCommandHandler(CmdStart, handleStart).
        RegisterCommandHandler(CmdHelp, handleHelp).
        RegisterActionHandler(ActionAwaitEmail, handleAwaitEmail).
        RegisterCallbackHandler(CbConfirm, handleConfirm).
        RegisterMiddlewareHandler(authMiddleware).
        Run(ctx) // blocks; starts worker pool and cleanup goroutines
}

func handleStart(ctx context.Context, u *tgbotapi.Update) error {
    _, err := tgClient.SendMessage(ctx, u.Message.Chat.ID, "Hello! Send me your email.")
    return err
}

func handleHelp(ctx context.Context, u *tgbotapi.Update) error {
    _, err := tgClient.SendMessage(ctx, u.Message.Chat.ID, "Commands:\n/start – begin\n/help – this message")
    return err
}
```

### go.mod setup

```bash
go mod init github.com/you/mybot
go get github.com/nejkit/telegram-bot-core@latest
go get github.com/caarlos0/env/v11
go get github.com/dgraph-io/ristretto
```

---

## Update Handling

This library uses **long polling exclusively**. `Run(ctx)` calls `GetUpdates()` internally and automatically reconnects if the updates channel closes. There is no webhook mode.

---

## Dispatch Order

For each incoming update, the service dispatches in this order:

1. **Special events** (ChatMember, MyChatMember, ChatJoinRequest, group→supergroup migration) → their dedicated handlers. **Middleware is NOT called** for these.
2. **Middleware** (if registered) — runs for all message and callback updates; can enrich `ctx` or abort by returning `false`.
3. **Messages**:
   - If `update.Message.Command()` matches a registered `CommandHandler` → call it
   - If the command is `/cancel` and no `CommandHandler("cancel")` is registered → route to the active action handler
   - Otherwise → load the user's active `Action` from storage and call its `ActionHandler`
4. **Callbacks**:
   - Parse `CallbackData` prefix via `UnwrapCallbackData` → call the matching `CallbackHandler`
   - If no `CallbackHandler` matches → fall back to the user's active `ActionHandler`
5. **Callback queries are auto-answered** — the service calls `AnswerCallbackQuery` after every callback handler. Do not call it manually.

```go
// Register all handler types:
svc.RegisterCommandHandler(CmdStart, handleStart)
svc.RegisterActionHandler(ActionAwaitEmail, handleEmail)
svc.RegisterCallbackHandler(CbConfirm, handleConfirm)
svc.RegisterMyChatMemberHandler(handleBotStatusChange)   // bot added/removed
svc.RegisterChatMemberHandler(handleMemberUpdate)        // user joined/left
svc.RegisterChatJoinRequestHandler(handleJoinRequest)    // join approval
svc.RegisterMigrationHandler(handleMigration)            // group→supergroup
svc.RegisterLimiterHandler(handleRateLimitExceeded)      // per-user rate limit hit
```

---

## Keyboards and Callbacks

```go
import (
    "github.com/nejkit/telegram-bot-core/client"
    "github.com/nejkit/telegram-bot-core/state"
    "github.com/nejkit/telegram-bot-core/utils"
    tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Simple inline keyboard
kb := tgbotapi.NewInlineKeyboardMarkup(
    tgbotapi.NewInlineKeyboardRow(
        tgbotapi.NewInlineKeyboardButtonData(
            "Confirm",
            state.WrapCallbackData(CbConfirm, "42"), // produces "confirm_42"
        ),
    ),
)
msgID, _ := tgClient.SendMessage(ctx, chatID, "Confirm action?",
    client.WithSendInlineKeyboard(kb))

// Paginated keyboard (automatically adds Назад/Вперед navigation buttons)
keys := []string{"Alice", "Bob", "Carol", "Dave", "Eve", "Frank"}
data := map[string]string{
    "Alice": state.WrapCallbackData(CbSelect, "alice"),
    "Bob":   state.WrapCallbackData(CbSelect, "bob"),
    // ...
}
kbInfo := utils.BuildInlineDataKeyboard(keys, data, 4) // 4 items per page
msgID, _ = tgClient.SendMessage(ctx, chatID, "Select user:",
    client.WithSendInlineKeyboard(kbInfo.Keyboards[0]))

// Persist kbInfo so built-in pagination callbacks work automatically
messageStore.SaveUserMessage(ctx, chatID, msgID, true) // withKeyboard=true
messageStore.SaveKeyboardInfo(ctx, chatID, msgID, kbInfo)
// "set-previous-keyboard" / "set-next-keyboard" callbacks are pre-registered by NewTelegramStateService

// Reply keyboard
replyKb := tgbotapi.NewReplyKeyboard(
    tgbotapi.NewKeyboardButtonRow(
        tgbotapi.NewKeyboardButton("Yes"),
        tgbotapi.NewKeyboardButton("No"),
    ),
)
tgClient.SendMessage(ctx, chatID, "Are you sure?",
    client.WithSendReplyKeyboard(replyKb))

// Remove reply keyboard
tgClient.SendMessage(ctx, chatID, "Keyboard removed.",
    client.WithRemoveReplyKeyboard())
```

---

## Middleware

```go
// MiddlewareFunc — called only for message/callback updates (not ChatMember etc.)
// Return (enrichedCtx, true) to continue; (ctx, false) to abort the update
func authMiddleware(ctx context.Context, u *tgbotapi.Update) (context.Context, bool) {
    if u.SentFrom() == nil || !isAllowed(u.SentFrom().ID) {
        return ctx, false
    }
    // Inject user language for localization
    lang := getUserLang(u.SentFrom().ID)
    return context.WithValue(ctx, state.LangCtxKey{}, lang), true
}
```

Only one middleware can be registered; chain multiple concerns inside it.

---

## Conversation State (FSM)

Call `actionStorage.SaveAction(ctx, userID, int(NextAction))` to transition a user to a new state; the next message from that user routes to `RegisterActionHandler(NextAction, ...)`. Reset with `SaveAction(ctx, userID, int(ActionNone))` (value `0`).

See [references/fsm.md](references/fsm.md) for the full pattern: multi-step forms, validators, `SaveActionWithRollback`, and storage backend setup.

---

## Sending Rich Content

```go
// Edit message text
tgClient.EditMessage(ctx, chatID, msgID,
    client.WithEditMessageText("Updated text"))

// Edit keyboard only
tgClient.EditMessageKeyboard(ctx, chatID, msgID, &newKeyboard)

// Edit both text and keyboard
tgClient.EditMessage(ctx, chatID, msgID,
    client.WithEditMessageText("New text"),
    client.WithEditInlineKeyboard(newKeyboard))

// Upload a file; returns (messageID, fileID, error)
msgID, fileID, err := tgClient.UploadFile(ctx, chatID, "report.pdf", fileBytes)

// Send previously uploaded file by Telegram file_id (no re-upload)
tgClient.SendFileByID(ctx, chatID, fileID)

// Copy a message between chats
tgClient.CopyMessage(ctx, fromChatID, toChatID, msgID)
```

---

## Error Handling

The client silently returns `nil` for "bot was blocked by the user" and "user is deactivated" — callers never need to filter these. HTTP-level errors (502, 503, 429) are automatically retried up to 3 times with 1-second waits.

Domain errors (check with `errors.Is`):

```go
import "github.com/nejkit/telegram-bot-core/domain"

domain.ErrorMessageNotFound   // message not in storage
domain.ErrorInviteIsExpired   // invite TTL elapsed or not found
domain.ErrorCallerNotFilled   // SentFrom() returned nil
domain.ErrorChatNotFilled     // FromChat() returned nil
```

---

## Project Structure (recommended)

```
mybot/
├── cmd/bot/main.go              # wiring only: config → storage → client → state service
├── internal/
│   ├── handlers/                # one file per feature; each func is state.HandlerFunc
│   ├── validators/              # ValidatorFunc implementations
│   ├── middleware/              # single MiddlewareFunc (auth, lang injection)
│   └── app/                    # optional DI container / struct
├── locales/
│   └── messages.json            # LocalizationFileInfo schema
├── Dockerfile
└── go.mod
```

---

## Deployment Checklist

- [ ] `BOT_TOKEN` in env var, never hardcoded
- [ ] `WORKERS_COUNT` set to CPU count for concurrent chat processing
- [ ] `ALLOWED_UPDATES` set to only what the bot needs (e.g. `message,callback_query`)
- [ ] `MESSAGE_PER_SECOND` = `-1` unless per-user rate limiting is desired
- [ ] `LOCALIZATION_FILE_PATH` set if using multi-language support
- [ ] Use Redis storage implementations for multi-instance deployments
- [ ] Graceful shutdown via `signal.NotifyContext`
- [ ] Structured logging with `log/slog` (Go 1.21+)
- [ ] No webhook setup needed (long polling only)

### Minimal Dockerfile

```dockerfile
FROM golang:1.23-alpine AS build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o bot ./cmd/bot

FROM alpine:3.20
RUN apk --no-cache add ca-certificates tzdata
WORKDIR /app
COPY --from=build /app/bot .
COPY locales/ locales/
CMD ["./bot"]
```

---

## Reference Files

- [references/fsm.md](references/fsm.md) — Full FSM patterns: Action/Command/Callback types, multi-step forms, validators, rollback, storage backends
- [references/core-library.md](references/core-library.md) — Full API reference: TelegramClient methods, storage interfaces, callback wrapping, localization, keyboard builder
- [references/deployment.md](references/deployment.md) — Docker, Fly.io, Railway, nginx/systemd, env var reference

Read a reference file when the user's task goes deep into that area.
