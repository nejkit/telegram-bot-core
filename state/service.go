package state

import (
	"context"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/nejkit/telegram-bot-core/config"
	"github.com/nejkit/telegram-bot-core/storage"
	"github.com/nejkit/telegram-bot-core/wrapper"
	"golang.org/x/time/rate"
)

type HandlerFunc func(ctx context.Context, update *tgbotapi.Update) bool

type ValidatorFunc func(update *tgbotapi.Update) error

type HandlerInfo struct {
	Handler           HandlerFunc
	MessageValidators []ValidatorFunc
}

type TelegramStateService[Action storage.UserAction, Command string, Callback string] struct {
	chatRequestChannels map[int64]chan tgbotapi.Update
	processingQueueChan chan int64

	commandHandler  map[Command]HandlerInfo
	actionHandler   map[Action]HandlerInfo
	callbackHandler map[Callback]HandlerInfo

	chatMemberHandler   HandlerFunc
	myChatMemberHandler HandlerFunc

	limiterMessageHandler HandlerFunc

	actionStorage  *storage.UserActionStorage[Action]
	workersCount   int
	limiter        *UserLimiter
	middlewareFunc HandlerFunc
}

func NewTelegramStateService[Action storage.UserAction, Command string, Callback string](
	cfg config.TelegramConfig,
	actionStorage *storage.UserActionStorage[Action],
) *TelegramStateService[Action, Command, Callback] {
	return &TelegramStateService[Action, Command, Callback]{
		chatRequestChannels: make(map[int64]chan tgbotapi.Update),
		processingQueueChan: make(chan int64, cfg.WorkersCount),
		commandHandler:      make(map[Command]HandlerInfo),
		actionHandler:       make(map[Action]HandlerInfo),
		callbackHandler:     make(map[Callback]HandlerInfo),

		actionStorage: actionStorage,
		workersCount:  cfg.WorkersCount,
		limiter:       NewUserLimiter(rate.Limit(cfg.MessagePerSecond), 0),
	}
}

func (t *TelegramStateService[Action, Command, Callback]) RegisterActionHandler(action Action, handler HandlerInfo) *TelegramStateService[Action, Command, Callback] {
	t.actionHandler[action] = handler

	return t
}

func (t *TelegramStateService[Action, Command, Callback]) RegisterCommandHandler(cmd Command, handler HandlerInfo) *TelegramStateService[Action, Command, Callback] {
	t.commandHandler[cmd] = handler

	return t
}

func (t *TelegramStateService[Action, Command, Callback]) RegisterCallbackHandler(callback Callback, handler HandlerInfo) *TelegramStateService[Action, Command, Callback] {
	t.callbackHandler[callback] = handler

	return t
}

func (t *TelegramStateService[Action, Command, Callback]) RegisterMyChatMemberHandler(handler HandlerFunc) *TelegramStateService[Action, Command, Callback] {
	t.myChatMemberHandler = handler

	return t
}

func (t *TelegramStateService[Action, Command, Callback]) RegisterChatMemberHandler(handler HandlerFunc) *TelegramStateService[Action, Command, Callback] {
	t.chatMemberHandler = handler

	return t
}

func (t *TelegramStateService[Action, Command, Callback]) RegisterLimiterHandler(handler HandlerFunc) *TelegramStateService[Action, Command, Callback] {
	t.limiterMessageHandler = handler

	return t
}

func (t *TelegramStateService[Action, Command, Callback]) RegisterMiddlewareHandler(handler HandlerFunc) *TelegramStateService[Action, Command, Callback] {
	t.middlewareFunc = handler

	return t
}

func (t *TelegramStateService[Action, Command, Callback]) Run(ctx context.Context, updatesChan tgbotapi.UpdatesChannel) {
	go t.startConsumeQueueChan(ctx)

	for {
		select {
		case <-ctx.Done():
			return

		case update, ok := <-updatesChan:
			if !ok {
				return
			}

			chatID := update.FromChat().ID
			userID := update.SentFrom().ID
			withRateCheck := true

			if update.ChatMember != nil {
				chatID = update.ChatMember.Chat.ID
				withRateCheck = false
			}

			if update.MyChatMember != nil {
				chatID = update.MyChatMember.Chat.ID
				withRateCheck = false
			}

			if withRateCheck && !t.limiter.Check(userID) {
				if t.limiterMessageHandler != nil {
					t.limiterMessageHandler(ctx, &update)
				}

				return
			}

			if _, ok = t.chatRequestChannels[chatID]; !ok {
				t.chatRequestChannels[chatID] = make(chan tgbotapi.Update, 10)
			}

			t.chatRequestChannels[chatID] <- update
			t.processingQueueChan <- chatID
		}
	}
}

func (t *TelegramStateService[Action, Command, Callback]) startConsumeQueueChan(ctx context.Context) {
	processingChan := make(chan tgbotapi.Update)

	for range t.workersCount {
		go func() {
			for {
				select {
				case <-ctx.Done():
					return

				case update := <-processingChan:
					t.handleUpdate(ctx, &update)
				}
			}
		}()
	}

	for {
		select {
		case <-ctx.Done():
			return

		case chatID, ok := <-t.processingQueueChan:
			if !ok {
				return
			}

			chatRequestChan, ok := t.chatRequestChannels[chatID]

			if !ok {
				continue
			}

			update, ok := <-chatRequestChan

			if !ok {
				continue
			}

			processingChan <- update
		}
	}
}

func (t *TelegramStateService[Action, Command, Callback]) handleUpdate(ctx context.Context, update *tgbotapi.Update) {
	if t.middlewareFunc != nil && !t.middlewareFunc(ctx, update) {
		return
	}

	if update.MyChatMember != nil && t.myChatMemberHandler != nil {
		t.myChatMemberHandler(ctx, update)
	}

	if update.ChatMember != nil && t.chatMemberHandler != nil {
		t.chatMemberHandler(ctx, update)
	}

	if update.Message != nil {
		t.handleMessage(ctx, update)
	}

	if update.CallbackQuery != nil {
		t.handleCallback(ctx, update)
	}
}

func (t *TelegramStateService[Action, Command, Callback]) handleCallback(ctx context.Context, update *tgbotapi.Update) {
	ctx = wrapper.FillCtx(ctx, update.FromChat().ID, update.SentFrom().ID)

	callbackHandler, ok := t.callbackHandler[Callback(update.CallbackData())]

	if ok {
		callbackHandler.Handler(ctx, update)
		return
	}

	action, err := t.actionStorage.GetAction(ctx)

	if err != nil {
		return
	}

	actionHandler, ok := t.actionHandler[action]

	if !ok {
		return
	}

	actionHandler.Handler(ctx, update)
}

func (t *TelegramStateService[Action, Command, Callback]) handleMessage(ctx context.Context, update *tgbotapi.Update) {
	ctx = wrapper.FillCtx(ctx, update.FromChat().ID, update.SentFrom().ID)

	cmdHandler, ok := t.commandHandler[Command(update.Message.Command())]

	if ok {
		cmdHandler.Handler(ctx, update)
		return
	}

	action, err := t.actionStorage.GetAction(ctx)

	if err != nil {
		return
	}

	actionHandler, ok := t.actionHandler[action]

	if !ok {
		return
	}

	isCancel := update.Message.IsCommand() && update.Message.Command() != "cancel"

	if isCancel {
		actionHandler.Handler(ctx, update)
		return
	}

	for _, validator := range actionHandler.MessageValidators {
		if err := validator(update); err != nil {
			return
		}
	}

	actionHandler.Handler(ctx, update)
}
