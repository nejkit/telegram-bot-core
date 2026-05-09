# FSM Patterns for Go Telegram Bots

## When to use FSM

Use a Finite State Machine when the bot needs to collect multiple pieces of information across turns (wizard/form), or when the meaning of a message depends on prior context.

## Concepts

The `TelegramStateService` is generic over three type parameters:

- **`Action ~int`** — which handler processes the *next* message/callback from a user. Persisted in `UserActionStorage`. Value `0` = no active flow (default).
- **`Command ~string`** — exact bot command without leading `/`, matched via `update.Message.Command()`.
- **`Callback ~string`** — prefix of `CallbackQuery.Data`, matched after `UnwrapCallbackData`.

**Dispatch priority for messages:** command handler → `/cancel` routes to active action handler if no `CommandHandler("cancel")` → active action handler.

**Dispatch priority for callbacks:** matching `CallbackHandler` → fallback to active action handler.

**Special events** (ChatMember, MyChatMember, ChatJoinRequest, Migration) bypass middleware and dispatch directly to their handlers.

---

## Defining Types

```go
type Action   int
type Command  string
type Callback string

const (
    ActionNone      Action   = 0  // always 0 — means "no active flow"
    ActionAwaitName Action   = 1
    ActionAwaitAge  Action   = 2

    CmdStart  Command = "start"   // no leading slash
    CmdCancel Command = "cancel"

    CbSubmit Callback = "submit"
)
```

---

## Registering Handlers

```go
svc := state.NewTelegramStateService[Action, Command, Callback](cfg, actionStore, msgStore, tgClient, locales).
    RegisterCommandHandler(CmdStart,  startHandler).
    RegisterCommandHandler(CmdCancel, cancelHandler).
    RegisterActionHandler(ActionAwaitName, awaitNameHandler).
    RegisterActionHandler(ActionAwaitAge,  awaitAgeHandler).
    RegisterCallbackHandler(CbSubmit, submitHandler).
    AddNotFlowableAction(ActionAwaitAge) // failed validators won't save messages for this action
```

---

## Multi-Step Form Example

```go
func startHandler(ctx context.Context, u *tgbotapi.Update) error {
    userID := u.SentFrom().ID
    chatID := u.Message.Chat.ID
    if err := actionStore.SaveAction(ctx, userID, int(ActionAwaitName)); err != nil {
        return err
    }
    _, err := tgClient.SendMessage(ctx, chatID, "What is your name?")
    return err
}

func awaitNameHandler(ctx context.Context, u *tgbotapi.Update) error {
    userID := u.SentFrom().ID
    chatID := u.Message.Chat.ID
    name := u.Message.Text
    // persist name in your own DB here
    if err := actionStore.SaveAction(ctx, userID, int(ActionAwaitAge)); err != nil {
        return err
    }
    _, err := tgClient.SendMessage(ctx, chatID, "How old are you, "+name+"?")
    return err
}

func awaitAgeHandler(ctx context.Context, u *tgbotapi.Update) error {
    userID := u.SentFrom().ID
    chatID := u.Message.Chat.ID
    // flow complete — clear state
    if err := actionStore.SaveAction(ctx, userID, int(ActionNone)); err != nil {
        return err
    }
    _, err := tgClient.SendMessage(ctx, chatID, "Registered! Age: "+u.Message.Text)
    return err
}

// cancelHandler is called when the user sends /cancel regardless of active action
func cancelHandler(ctx context.Context, u *tgbotapi.Update) error {
    userID := u.SentFrom().ID
    chatID := u.Message.Chat.ID
    _ = actionStore.SaveAction(ctx, userID, int(ActionNone))
    _, err := tgClient.SendMessage(ctx, chatID, "Cancelled.")
    return err
}
```

Note: if no `CommandHandler("cancel")` is registered, `/cancel` is routed to the currently active action handler — handle reset there instead.

---

## ValidatorFunc

Validators run before the handler. If a validator returns an error, the error string is treated as a **locale key** and looked up via `locales.GetWithCulture(userLang, err.Error())`, then sent to the user. The handler is **not** called.

```go
func validateNonEmpty(u *tgbotapi.Update) error {
    if u.Message == nil || strings.TrimSpace(u.Message.Text) == "" {
        return errors.New("validation.name_required") // must be a key in messages.json
    }
    return nil
}

// Register with validators:
svc.RegisterActionHandler(ActionAwaitName, awaitNameHandler, validateNonEmpty)
```

**`AddNotFlowableAction(action)`** — when set, failed validation does NOT save the invalid message or the error reply in `UserMessageStorage`. Use for actions where you don't track conversation history.

---

## SaveActionWithRollback

Atomically save a new action and get a rollback function that restores the previous one:

```go
rollback, err := actionStore.SaveActionWithRollback(ctx, userID, int(ActionAwaitName))
if err != nil {
    return err
}
if err := doRiskyOperation(ctx); err != nil {
    return rollback(err) // restores previous action, returns the original error
}
return rollback(nil) // keeps the new action, returns nil
```

---

## Storage Backends

### In-memory (single instance, dev or prod)

```go
import "github.com/dgraph-io/ristretto"

cache, _ := ristretto.NewCache(&ristretto.Config{
    NumCounters: 1e7,
    MaxCost:     1 << 30,
    BufferItems: 64,
})
actionStore  := storage.NewInMemoryUserActionStorage[Action](cache)
messageStore := storage.NewInMemoryUserMessageStorage(cache)
inviteStore  := storage.NewInMemoryInvitesStorage(cache)
```

### Redis (multi-instance, production)

```go
import "github.com/redis/go-redis/v9"

rdb := redis.NewClient(&redis.Options{Addr: os.Getenv("REDIS_ADDR")})
actionStore  := storage.NewRedisUserActionStorage[Action]("mybot", rdb)
messageStore := storage.NewRedisUserMessageStorage("mybot", rdb)
inviteStore  := storage.NewRedisInvitesStorage("mybot", rdb)
// "mybot" prefix namespaces all keys — allows multiple bots on one Redis instance
```

---

## Callback Wrapping

```go
// WrapCallbackData produces "{prefix}_{data}", e.g. "submit_42"
data := state.WrapCallbackData(CbSubmit, "42")

// UnwrapCallbackData splits on "_" and expects exactly 2 parts
// IMPORTANT: payload must NOT contain "_" — the function returns ("", "") if there are more than one "_"
cb, payload := state.UnwrapCallbackData[Callback](u.CallbackData())
// cb == CbSubmit, payload == "42"
```

If your payload may contain `_`, encode it first (e.g. base64 or replace `_` with a different separator before wrapping).

---

## Tips

- `actionStore.SaveAction(ctx, userID, int(ActionNone))` is the "reset" — always call it when a flow completes or is cancelled.
- Per-chat serialization is built-in: the same chat never processes two updates concurrently regardless of `WORKERS_COUNT`.
- Validator error strings must be valid locale keys in your `messages.json`. The state service looks them up and sends the localized text; if the key is missing the raw error string is sent.
- Set a Redis TTL on action keys to clean up abandoned flows automatically (configure at the Redis client level).
