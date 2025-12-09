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

type HandlerFunc func(ctx context.Context, update *tgbotapi.Update) bool

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
	middlewareFunc HandlerFunc
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

func (t *TelegramStateService[Action, Command, Callback]) handleSetPreviousKeyboardPage(ctx context.Context, update *tgbotapi.Update) (result bool) {
	result = true
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

func (t *TelegramStateService[Action, Command, Callback]) handleSetNextKeyboardPage(ctx context.Context, update *tgbotapi.Update) (result bool) {
	result = true
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
					t.limiterMessageHandler(ctx, &update)
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

	log.Debug("call middleware")

	if t.middlewareFunc != nil && !t.middlewareFunc(ctx, update) {
		log.Debug("failed call middleware")
		return
	}

	log.Debug("middleware called successfully")

	if update.MyChatMember != nil && t.myChatMemberHandler != nil {
		log.Debug("handle my chat member event")
		t.myChatMemberHandler(ctx, update)
	}

	if update.ChatMember != nil && t.chatMemberHandler != nil {
		log.Debug("handle chat member event")
		t.chatMemberHandler(ctx, update)
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
		callbackHandler.Handler(ctx, update)
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

	actionHandler.Handler(ctx, update)
}

func (t *TelegramStateService[Action, Command, Callback]) handleMessage(ctx context.Context, update *tgbotapi.Update) {
	userID := update.SentFrom().ID
	cmdHandler, ok := t.commandHandler[Command(update.Message.Command())]

	if ok {
		cmdHandler.Handler(ctx, update)
		return
	}

	action, err := t.actionStorage.GetAction(ctx, userID)

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
