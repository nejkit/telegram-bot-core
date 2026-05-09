# telegram-bot-core Library Reference

Module: `github.com/nejkit/telegram-bot-core`  
Underlying API: `github.com/go-telegram-bot-api/telegram-bot-api/v5`

---

## TelegramClient — Construction

```go
import "github.com/nejkit/telegram-bot-core/client"

tgClient := client.NewTelegramClient(&cfg)
```

Built-in on creation:
- **Retry transport**: retries on 5xx + 429 up to 3 times with 1-second waits
- **Global limiter**: 25 messages/second (burst 25)
- **Per-chat limiter**: 1 message/second per chat (burst 2)
- `RunChatRatesCleanup(ctx)` starts a background goroutine that prunes inactive per-chat limiters every 30 minutes. `Run()` on the state service calls this automatically.

---

## Sending Messages

```go
// Signature
SendMessage(ctx context.Context, chatID int64, text string, options ...MessageOptions) (int, error)
```

### MessageOptions

| Function | Parameter | Effect |
|---|---|---|
| `client.WithSendInlineKeyboard` | `tgbotapi.InlineKeyboardMarkup` | Attach inline keyboard |
| `client.WithSendReplyKeyboard` | `tgbotapi.ReplyKeyboardMarkup` | Attach reply keyboard |
| `client.WithRemoveReplyKeyboard` | — | Remove current reply keyboard |

```go
// Plain text
msgID, err := tgClient.SendMessage(ctx, chatID, "Hello!")

// With inline keyboard
msgID, err = tgClient.SendMessage(ctx, chatID, "Choose:",
    client.WithSendInlineKeyboard(kb))

// With reply keyboard
msgID, err = tgClient.SendMessage(ctx, chatID, "Yes or No?",
    client.WithSendReplyKeyboard(replyKb))

// Remove reply keyboard
msgID, err = tgClient.SendMessage(ctx, chatID, "Keyboard removed.",
    client.WithRemoveReplyKeyboard())
```

---

## Editing Messages

```go
// Edit text and/or keyboard
EditMessage(ctx context.Context, chatID int64, msgID int, options ...EditMessageOptions) error

// Edit keyboard only (shortcut — does not touch text)
EditMessageKeyboard(ctx context.Context, chatID int64, msgID int, keyboard *tgbotapi.InlineKeyboardMarkup) error
```

### EditMessageOptions

| Function | Parameter | Effect |
|---|---|---|
| `client.WithEditMessageText` | `string` | Replace message text |
| `client.WithEditInlineKeyboard` | `tgbotapi.InlineKeyboardMarkup` | Replace inline keyboard |

```go
// Edit text only
tgClient.EditMessage(ctx, chatID, msgID,
    client.WithEditMessageText("Updated text"))

// Edit keyboard only (direct pointer)
tgClient.EditMessageKeyboard(ctx, chatID, msgID, &newKeyboard)

// Edit both
tgClient.EditMessage(ctx, chatID, msgID,
    client.WithEditMessageText("New text"),
    client.WithEditInlineKeyboard(newKeyboard))
```

---

## File Operations

```go
// Upload bytes as a document; returns (messageID, fileID, error)
msgID, fileID, err := tgClient.UploadFile(ctx, chatID, "report.pdf", fileBytes)

// Send a previously uploaded file by Telegram file_id (no re-upload bandwidth)
msgID, err = tgClient.SendFileByID(ctx, chatID, fileID)

// Download a file by file_id
info, err := tgClient.DownloadFile(ctx, fileID)
// info.FileName string
// info.MimoType string  (detected via http.DetectContentType)
// info.FileData []byte

// Copy a message from one chat to another
newMsgID, err := tgClient.CopyMessage(ctx, fromChatID, toChatID, msgID)
```

---

## Chat Management

```go
// Delete a message
tgClient.DeleteMessage(ctx, chatID, msgID)

// Kick (and optionally ban) a user
// withBan=false: kicks and immediately unbans (user can rejoin)
// withBan=true:  permanent ban
tgClient.KickUserFromChat(ctx, chatID, userID, withBan)

// Approve or decline a chat join request
tgClient.ProcessChatJoinRequest(ctx, chatID, userID, accept)

// Get/set bot command menu scoped to a specific chat
cmds, err := tgClient.GetBotCommands(ctx, chatID)
err = tgClient.SetBotCommands(ctx, chatID, []tgbotapi.BotCommand{
    {Command: "start", Description: "Start"},
    {Command: "help",  Description: "Help"},
})

// Get full chat/user info
chat, err := tgClient.GetContactInfo(ctx, userID)
// returns *tgbotapi.Chat
```

---

## Callbacks and Invites

```go
// Answer a callback query with a toast notification (shown to user briefly)
tgClient.AnswerCallback(callbackID, "Action confirmed!")

// Answer a callback query silently (no toast)
// Note: the state service calls this automatically after every callback handler
tgClient.AnswerCallbackQuery(ctx, callbackID)

// Generate a deep-link invite URL: https://telegram.me/<botname>?start=<secret>
link, err := tgClient.GetInviteLink(ctx, secret)
```

---

## Web App

```go
// Validate Telegram Web App initData and extract the user ID
userID, err := tgClient.ValidateWebAppInitData(initData)
```

---

## Error Handling

`handleError` (called internally on every API response) silently returns `nil` for:
- `"Forbidden: bot was blocked by the user"`
- `"Forbidden: user is deactivated"`

All other Telegram API errors are surfaced as `tgbotapi.Error`. Callers never need to filter blocked/deactivated users.

---

## Storage Interfaces

### UserActionStorage

Stores the current `Action` (as `int`) per user:

```go
type UserActionStorage interface {
    SaveAction(ctx context.Context, userID int64, action int) error
    GetAction(ctx context.Context, userID int64) (int, error)
    SaveActionWithRollback(ctx context.Context, userID int64, action int) (func(error) error, error)
}
```

Implementations: `storage.NewRedisUserActionStorage[T]("prefix", rdb)`, `storage.NewInMemoryUserActionStorage[T](cache)`

### UserMessageStorage

Tracks messages sent in a conversation for cleanup and keyboard pagination:

```go
type UserMessageStorage interface {
    SaveUserMessage(ctx context.Context, chatID int64, messageID int, withKeyboard bool) error
    GetUserMessages(ctx context.Context, chatID int64) ([]MessageInfo, error)
    DeleteUserMessage(ctx context.Context, chatID int64) error

    SaveCallbackMessage(ctx context.Context, callbackID string, chatID int64, messageID int) error
    GetCallbackMessage(ctx context.Context, callbackID string) (*MessageInfo, error)
    DeleteCallbackMessage(ctx context.Context, callbackID string) error

    SaveKeyboardInfo(ctx context.Context, chatID int64, messageID int, keyboard *KeyboardInfo) error
    GetKeyboardInfo(ctx context.Context, chatID int64, messageID int) (*KeyboardInfo, error)
    DeleteKeyboardInfo(ctx context.Context, chatID int64, messageID int) error
}
```

Implementations: `storage.NewRedisUserMessageStorage("prefix", rdb)`, `storage.NewInMemoryUserMessageStorage(cache)`

### InvitesStorage

Manages deep-link invite tokens with TTL:

```go
type InvitesStorage interface {
    SaveInvite(ctx context.Context, deepLinkSecret string, fromUserID int64, expiration time.Duration) error
    GetInvite(ctx context.Context, deepLinkSecret string) (int64, error) // returns inviter userID; error if expired
    DeleteInvite(ctx context.Context, deepLinkSecret string) error
}
```

Implementations: `storage.NewRedisInvitesStorage("prefix", rdb)`, `storage.NewInMemoryInvitesStorage(cache)`

### Data Structs

```go
type MessageInfo struct {
    MessageID      int   `json:"message_id"`
    ChatID         int64 `json:"chat_id"`
    InlineKeyboard bool  `json:"inline_keyboard,omitempty"` // has pagination keyboard
}

type KeyboardInfo struct {
    Keyboards       []tgbotapi.InlineKeyboardMarkup
    CurrentPosition int
}
```

---

## Keyboard Builder

```go
import "github.com/nejkit/telegram-bot-core/utils"

// Paginated callback buttons
// sortedKeys controls display order; data maps key → callback data string
keys := []string{"Alice", "Bob", "Carol", "Dave", "Eve"}
cbData := map[string]string{
    "Alice": state.WrapCallbackData(CbSelect, "alice"),
    "Bob":   state.WrapCallbackData(CbSelect, "bob"),
    // ...
}
kbInfo := utils.BuildInlineDataKeyboard(keys, cbData, 3) // 3 items per page

// Paginated URL buttons
urlData := map[string]string{
    "Docs":   "https://docs.example.com",
    "GitHub": "https://github.com/example",
}
kbInfo = utils.BuildInlineURLKeyboard([]string{"Docs", "GitHub"}, urlData, 10)
```

When there are multiple pages, `BuildInlineDataKeyboard` automatically appends "Назад" and "Вперед" navigation rows using the pre-registered `set-previous-keyboard` / `set-next-keyboard` callbacks.

**For pagination to work**, persist the keyboard after sending:

```go
msgID, _ := tgClient.SendMessage(ctx, chatID, "Select:",
    client.WithSendInlineKeyboard(kbInfo.Keyboards[0]))

messageStore.SaveUserMessage(ctx, chatID, msgID, true)   // withKeyboard=true
messageStore.SaveKeyboardInfo(ctx, chatID, msgID, kbInfo)
```

---

## Callback Wrapping

```go
import "github.com/nejkit/telegram-bot-core/state"

// WrapCallbackData produces "{prefix}_{data}", e.g. "select_alice"
data := state.WrapCallbackData(CbSelect, "alice")

// UnwrapCallbackData splits on "_" — expects exactly 2 parts
// IMPORTANT: payload must NOT contain "_"
cb, payload := state.UnwrapCallbackData[Callback](u.CallbackData())
```

If the payload may contain `_`, encode it before wrapping (e.g. base64).

---

## Localization

```go
import "github.com/nejkit/telegram-bot-core/locale"

locales := locale.NewLocalizationProvider("/app/locales/messages.json")

// Default culture (from defaultCulture in JSON)
text := locales.GetDefaultLocalization("welcome.message", userName)

// Specific culture (falls back to defaultCulture if key missing)
text = locales.GetWithCulture("ru", "welcome.message", userName)
```

### JSON file format (`LocalizationFileInfo`)

```json
{
  "defaultCulture": "en",
  "localizedContent": {
    "welcome.message": {
      "en": "Welcome, %s!",
      "ru": "Добро пожаловать, %s!"
    },
    "validation.name_required": {
      "en": "Please enter your name.",
      "ru": "Пожалуйста, введите ваше имя."
    }
  }
}
```

Inject the user's language into context via middleware so validators' locale keys are resolved correctly:

```go
func langMiddleware(ctx context.Context, u *tgbotapi.Update) (context.Context, bool) {
    lang := getUserLangFromDB(u.SentFrom().ID)
    return context.WithValue(ctx, state.LangCtxKey{}, lang), true
}
```

---

## Per-User Rate Limiter

The state service uses `MESSAGE_PER_SECOND` from `TelegramConfig` to build an internal `UserLimiter`. For direct use:

```go
import (
    "golang.org/x/time/rate"
    "github.com/nejkit/telegram-bot-core/limiter"
)

ul := limiter.NewUserLimiter(rate.Limit(5), 10) // 5/sec, burst 10
go ul.Run(ctx)                                   // start background cleanup (every 30 min)

ul.Wait(ctx, userID)         // block until token available
ok := ul.Check(userID)       // non-blocking; returns false if rate exceeded
```

Pass `rate.Limit(-1)` to disable — all calls become no-ops.

---

## Domain Errors

```go
import "github.com/nejkit/telegram-bot-core/domain"

domain.ErrorCallerNotFilled  // SentFrom() returned nil
domain.ErrorChatNotFilled    // FromChat() returned nil
domain.ErrorMessageNotFound  // message not found in UserMessageStorage
domain.ErrorInviteIsExpired  // invite TTL elapsed or secret not found
```

Use with `errors.Is(err, domain.ErrorMessageNotFound)`.
