package state

import (
	"context"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/nejkit/telegram-bot-core/storage"
	"github.com/nejkit/telegram-bot-core/wrapper"
)

type HandlerFunc func(ctx context.Context, update *tgbotapi.Update) bool

type ValidatorFunc func(update *tgbotapi.Update) error

type HandlerInfo struct {
	Handler           HandlerFunc
	MessageValidators []ValidatorFunc
}

type TelegramStateService[Action storage.UserAction, Command string, Callback string] struct {
	chatRequestChannels map[int64]chan tgbotapi.Update
	allowedUpdateIDs    map[int64]int
	processingQueueChan chan int64

	commandHandler  map[Command]HandlerInfo
	actionHandler   map[Action]HandlerInfo
	callbackHandler map[Callback]HandlerInfo

	chatMemberHandler   HandlerFunc
	meChatMemberHandler HandlerFunc

	actionStorage *storage.UserActionStorage[Action]
}

func (t *TelegramStateService[Action, Command, Callback]) startConsumeQueueChan(ctx context.Context) {
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

			t.handleUpdate(ctx, &update)
		}
	}
}

func (t *TelegramStateService[Action, Command, Callback]) handleUpdate(ctx context.Context, update *tgbotapi.Update) {
	if update.Message != nil {
		t.handleMessage(ctx, update)
	}

	if update.CallbackQuery != nil {
		t.handleCallback(ctx, update)
	}

}

func (t *TelegramStateService[Action, Command, Callback]) handleCallback(ctx context.Context, update *tgbotapi.Update) {
	ctx = wrapper.FillCtx(ctx, update.FromChat().ID, update.SentFrom().ID)

	if lastUpdate, ok := t.allowedUpdateIDs[update.FromChat().ID]; ok {
		if lastUpdate > update.UpdateID {
			return
		}
	}

	t.allowedUpdateIDs[update.FromChat().ID] = update.UpdateID

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

	if lastUpdate, ok := t.allowedUpdateIDs[update.FromChat().ID]; ok {
		if lastUpdate > update.UpdateID {
			return
		}
	}

	t.allowedUpdateIDs[update.FromChat().ID] = update.UpdateID

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
