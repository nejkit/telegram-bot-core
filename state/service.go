package state

import (
	"context"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/nejkit/telegram-bot-core/client"
	"github.com/nejkit/telegram-bot-core/config"
	"github.com/nejkit/telegram-bot-core/storage"
	"github.com/sirupsen/logrus"
	"golang.org/x/time/rate"
	"time"
)

type BotCommand interface {
	~string
}

type HandlerFunc func(ctx context.Context, update *tgbotapi.Update) error

type MiddlewareFunc func(ctx context.Context, update *tgbotapi.Update) (context.Context, bool)

type ValidatorFunc func(update *tgbotapi.Update) error

type HandlerInfo struct {
	Handler           HandlerFunc
	MessageValidators []ValidatorFunc
}

type TelegramStateService[Action storage.UserAction, Command BotCommand, Callback CallbackPrefix] struct {
	chatRequestChannels map[int64]chan tgbotapi.Update
	processingQueueChan chan int64

	telegramClient *client.TelegramClient

	commandHandler  map[Command]HandlerInfo
	actionHandler   map[Action]HandlerInfo
	callbackHandler map[Callback]HandlerInfo

	chatMemberHandler   HandlerFunc
	myChatMemberHandler HandlerFunc

	limiterMessageHandler HandlerFunc

	actionStorage  *storage.UserActionStorage[Action]
	messageStorage *storage.UserMessageStorage
	workersCount   int
	limiter        *UserLimiter
	middlewareFunc MiddlewareFunc
	processor      *MessageProcessor
}

func NewTelegramStateService[Action storage.UserAction, Command BotCommand, Callback CallbackPrefix](
	cfg config.TelegramConfig,
	actionStorage *storage.UserActionStorage[Action],
	messageStorage *storage.UserMessageStorage,
	client *client.TelegramClient,
) *TelegramStateService[Action, Command, Callback] {
	handler := &TelegramStateService[Action, Command, Callback]{
		chatRequestChannels: make(map[int64]chan tgbotapi.Update),
		processingQueueChan: make(chan int64, cfg.WorkersCount),
		commandHandler:      make(map[Command]HandlerInfo),
		actionHandler:       make(map[Action]HandlerInfo),
		callbackHandler:     make(map[Callback]HandlerInfo),
		telegramClient:      client,

		actionStorage:  actionStorage,
		messageStorage: messageStorage,
		workersCount:   cfg.WorkersCount,
		limiter:        NewUserLimiter(rate.Limit(cfg.MessagePerSecond), 1),
		processor:      NewMessageProcessor(),
	}

	handler.callbackHandler["set_previous_keyboard"] = HandlerInfo{
		Handler: handler.handleSetPreviousKeyboardPage,
	}
	handler.callbackHandler["set_next_keyboard"] = HandlerInfo{
		Handler: handler.handleSetNextKeyboardPage,
	}

	return handler
}

func (t *TelegramStateService[Action, Command, Callback]) handleSetPreviousKeyboardPage(ctx context.Context, update *tgbotapi.Update) (result error) {
	result = nil
	userID := update.FromChat().ID
	messageInfos, err := t.messageStorage.GetUserMessages(ctx, userID)

	if err != nil {
		logrus.WithError(err).Error("Error getting messages")
		return
	}

	for _, messageInfo := range messageInfos {
		if messageInfo.MessageID == update.CallbackQuery.Message.MessageID {
			if !messageInfo.InlineKeyboard {
				logrus.Error("message not contains inline keyboard")
				return
			}

			keyboardInfo, err := t.messageStorage.GetKeyboardInfo(ctx, userID, messageInfo.MessageID)

			if err != nil {
				logrus.WithError(err).Error("Error getting keyboard")
				return
			}

			keyboardInfo.CurrentPosition--

			if keyboardInfo.CurrentPosition < 0 {
				logrus.Error("invalid keyboard idx")
				return
			}

			newKeyboard := keyboardInfo.Keyboards[keyboardInfo.CurrentPosition]

			err = t.telegramClient.EditMessage(userID, messageInfo.MessageID, client.WithEditInlineKeyboard(newKeyboard))

			if err != nil {
				logrus.WithError(err).Error("Error editing keyboard")
				return
			}

			err = t.messageStorage.SaveKeyboardInfo(ctx, userID, messageInfo.MessageID, keyboardInfo)

			if err != nil {
				logrus.WithError(err).Error("Error saving keyboard")
			}

			return
		}
	}

	logrus.Warn("not found message id")
	return
}

func (t *TelegramStateService[Action, Command, Callback]) handleSetNextKeyboardPage(ctx context.Context, update *tgbotapi.Update) (result error) {
	result = nil
	userID := update.FromChat().ID
	messageInfos, err := t.messageStorage.GetUserMessages(ctx, userID)

	if err != nil {
		logrus.WithError(err).Error("Error getting messages")
		return
	}

	for _, messageInfo := range messageInfos {
		if messageInfo.MessageID == update.CallbackQuery.Message.MessageID {
			if !messageInfo.InlineKeyboard {
				logrus.Error("message not contains inline keyboard")
				return
			}

			keyboardInfo, err := t.messageStorage.GetKeyboardInfo(ctx, userID, messageInfo.MessageID)

			if err != nil {
				logrus.WithError(err).Error("Error getting keyboard")
				return
			}

			keyboardInfo.CurrentPosition++

			if keyboardInfo.CurrentPosition == len(keyboardInfo.Keyboards) {
				logrus.Error("invalid keyboard idx")
				return
			}

			newKeyboard := keyboardInfo.Keyboards[keyboardInfo.CurrentPosition]

			err = t.telegramClient.EditMessage(userID, messageInfo.MessageID, client.WithEditInlineKeyboard(newKeyboard))

			if err != nil {
				logrus.WithError(err).Error("Error editing keyboard")
				return
			}

			err = t.messageStorage.SaveKeyboardInfo(ctx, userID, messageInfo.MessageID, keyboardInfo)

			if err != nil {
				logrus.WithError(err).Error("Error saving keyboard")
			}

			return
		}
	}

	logrus.Warn("not found message id")
	return
}

func (t *TelegramStateService[Action, Command, Callback]) RegisterActionHandler(action Action, handler HandlerFunc, validators ...ValidatorFunc) *TelegramStateService[Action, Command, Callback] {
	t.actionHandler[action] = HandlerInfo{
		Handler:           handler,
		MessageValidators: validators,
	}

	return t
}

func (t *TelegramStateService[Action, Command, Callback]) RegisterCommandHandler(cmd Command, handler HandlerFunc, validators ...ValidatorFunc) *TelegramStateService[Action, Command, Callback] {
	t.commandHandler[cmd] = HandlerInfo{
		Handler:           handler,
		MessageValidators: validators,
	}

	return t
}

func (t *TelegramStateService[Action, Command, Callback]) RegisterCallbackHandler(callback Callback, handler HandlerFunc, validators ...ValidatorFunc) *TelegramStateService[Action, Command, Callback] {
	t.callbackHandler[callback] = HandlerInfo{
		Handler:           handler,
		MessageValidators: validators,
	}

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

func (t *TelegramStateService[Action, Command, Callback]) RegisterMiddlewareHandler(handler MiddlewareFunc) *TelegramStateService[Action, Command, Callback] {
	t.middlewareFunc = handler

	return t
}

func (t *TelegramStateService[Action, Command, Callback]) Run(ctx context.Context, updatesChan tgbotapi.UpdatesChannel) {
	logrus.Info("start telegram updates handler service")
	go t.startConsumeQueueChan(ctx)

	for {
		select {
		case <-ctx.Done():
			return

		case update, ok := <-updatesChan:
			if !ok {
				return
			}

			fromChat := update.FromChat()
			fromUser := update.SentFrom()

			if fromChat == nil {
				fromChat = &tgbotapi.Chat{}
			}

			if fromUser == nil {
				fromUser = &tgbotapi.User{}
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

			log := logrus.WithFields(logrus.Fields{
				"updateID": update.UpdateID,
				"userID":   userID,
				"chatID":   chatID,
			})

			log.Debug("received update from telegram bot")

			if withRateCheck && !t.limiter.Check(userID) {
				log.Debug("rate limit exceeded, skip update")
				if t.limiterMessageHandler != nil {
					if err := t.limiterMessageHandler(ctx, &update); err != nil {
						log.WithError(err).Error("failed execute limiter message handler")
					}
				}

				continue
			}

			log.Debug("success check rates by this user")

			if _, ok = t.chatRequestChannels[chatID]; !ok {
				t.chatRequestChannels[chatID] = make(chan tgbotapi.Update, 10)
			}

			t.chatRequestChannels[chatID] <- update
			t.processor.PutChat(chatID)

			log.Debug("update successfully queued for processing")
		}
	}
}

func (t *TelegramStateService[Action, Command, Callback]) startConsumeQueueChan(ctx context.Context) {
	processingChan := make(chan tgbotapi.Update, t.workersCount)
	omitChatIdsChan := make(chan int64, t.workersCount)

	go t.processor.Run(ctx, omitChatIdsChan)

	for i := range t.workersCount {
		go func(workerId int) {
			for {
				select {
				case <-ctx.Done():
					return

				case update := <-processingChan:
					logrus.WithField("workerID", workerId).Debug("start processing update")
					t.handleUpdate(ctx, &update)
					logrus.WithField("workerID", workerId).Debug("finished processing update")
					chatInfo := update.FromChat()

					if chatInfo == nil {
						if update.ChatMember != nil {
							chatInfo = &tgbotapi.Chat{ID: update.ChatMember.Chat.ID}
						}
						if update.MyChatMember != nil {
							chatInfo = &tgbotapi.Chat{ID: update.MyChatMember.Chat.ID}
						}
					}

					if chatInfo == nil {
						panic("")
					}

					omitChatIdsChan <- chatInfo.ID
				}
			}
		}(i)
	}

	ticker := time.NewTicker(time.Millisecond * 10)

	for {
		select {
		case <-ctx.Done():
			return

		case <-ticker.C:
			ticker.Stop()

			chatID := t.processor.GetChat()

			if chatID == 0 {
				ticker.Reset(time.Millisecond * 100)
				continue
			}

			log := logrus.WithFields(logrus.Fields{
				"chatID": chatID,
			})

			log.Debug("try get update from chat queue")

			chatRequestChan, ok := t.chatRequestChannels[chatID]

			if !ok {
				ticker.Reset(time.Millisecond * 10)
				continue
			}

			update, ok := <-chatRequestChan

			if !ok {
				ticker.Reset(time.Millisecond * 10)
				continue
			}

			log.WithField("updateID", update.UpdateID).Debug("add update to worker processing queue")

			processingChan <- update
			ticker.Reset(time.Millisecond * 10)
		}
	}
}

func (t *TelegramStateService[Action, Command, Callback]) handleUpdate(ctx context.Context, update *tgbotapi.Update) {
	log := logrus.WithFields(logrus.Fields{
		"updateID": update.UpdateID,
	})

	if t.middlewareFunc != nil {
		log.Debug("try call middleware")
		var isSuccess bool

		ctx, isSuccess = t.middlewareFunc(ctx, update)

		if !isSuccess {
			log.Debug("failed call middleware")
			return
		}

		log.Debug("middleware called successfully")
	}

	if update.MyChatMember != nil && t.myChatMemberHandler != nil {
		log.Debug("handle my chat member event")
		if err := t.myChatMemberHandler(ctx, update); err != nil {
			log.WithError(err).Error("failed handle my chat member event")
		}
	}

	if update.ChatMember != nil && t.chatMemberHandler != nil {
		log.Debug("handle chat member event")
		if err := t.chatMemberHandler(ctx, update); err != nil {
			log.WithError(err).Error("failed handle chat member event")
		}
	}

	if update.Message != nil {
		log.Debug("handle message event")
		t.handleMessage(ctx, update)
	}

	if update.CallbackQuery != nil {
		log.Debug("handle callback event")
		t.handleCallback(ctx, update)
	}
}

func (t *TelegramStateService[Action, Command, Callback]) handleCallback(ctx context.Context, update *tgbotapi.Update) {
	userID := update.SentFrom().ID
	log := logrus.WithFields(logrus.Fields{
		"updateID": update.UpdateID,
		"userID":   userID,
		"chatID":   update.FromChat().ID,
	})

	log.Debug("check is event contains callback data")

	callback, _ := UnwrapCallbackData[Callback](update.CallbackData())

	callbackHandler, ok := t.callbackHandler[callback]

	if ok {
		log.WithField("callback", callback).
			Debug("event contains callback data, call handler")
		err := callbackHandler.Handler(ctx, update)

		if err != nil {
			log.WithError(err).Error("failed handle callback event")
		}
		return
	}

	action, err := t.actionStorage.GetAction(ctx, userID)

	if err != nil {
		log.WithError(err).Error("failed to get action by user")
		return
	}

	actionHandler, ok := t.actionHandler[action]

	if !ok {
		log.Error("action handler not found")
		return
	}

	log.WithField("action", action).
		Debug("event contains action data, call handler")

	err = actionHandler.Handler(ctx, update)

	if err != nil {
		log.WithError(err).Error("failed handle action callback event")
	}
}

func (t *TelegramStateService[Action, Command, Callback]) handleMessage(ctx context.Context, update *tgbotapi.Update) {
	userID := update.SentFrom().ID
	log := logrus.WithFields(logrus.Fields{
		"updateID": update.UpdateID,
		"userID":   userID,
		"chatID":   update.FromChat().ID,
	})

	log.Debug("check is event is bot command")

	cmdHandler, ok := t.commandHandler[Command(update.Message.Command())]

	if ok {
		log.WithField("command", update.Message.Command()).
			Debug("event is bot command, call handler")
		err := cmdHandler.Handler(ctx, update)

		if err != nil {
			log.WithError(err).Error("failed handle command event")
		}
		return
	}

	action, err := t.actionStorage.GetAction(ctx, userID)

	if err != nil {
		return
	}

	actionHandler, ok := t.actionHandler[action]

	if !ok {
		log.Error("action handler not found")
		return
	}

	isCancel := update.Message.IsCommand() && update.Message.Command() != "cancel"

	if isCancel {
		log.WithField("action", action).
			Debug("event is cancel command, call handler")

		err = actionHandler.Handler(ctx, update)

		if err != nil {
			log.WithError(err).Error("failed handle cancel command event")
		}

		return
	}

	log.WithField("action", action).Debug("try process validations before call handler")

	for _, validator := range actionHandler.MessageValidators {
		if err := validator(update); err != nil {
			log.WithError(err).Error("failed validate update")
			return
		}
	}

	log.WithField("action", action).Debug("validations processed, call handler")

	err = actionHandler.Handler(ctx, update)

	if err != nil {
		log.WithError(err).Error("failed handle event")
	}
}
