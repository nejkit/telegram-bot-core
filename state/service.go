package state

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/go-telegram/bot/models"
	"github.com/nejkit/telegram-bot-core/client"
	"github.com/nejkit/telegram-bot-core/config"
	"github.com/nejkit/telegram-bot-core/limiter"
	"github.com/nejkit/telegram-bot-core/locale"
	"github.com/nejkit/telegram-bot-core/storage"
	"github.com/sirupsen/logrus"
	"golang.org/x/time/rate"
)

type BotCommand interface {
	~string
}

type HandlerFunc func(ctx context.Context, update *models.Update) error

type MiddlewareFunc func(ctx context.Context, update *models.Update) (context.Context, bool)

type ValidatorFunc func(update *models.Update) error

type HandlerInfo struct {
	Handler           HandlerFunc
	MessageValidators []ValidatorFunc
}

type TelegramStateService[Action storage.UserAction, Command BotCommand, Callback CallbackPrefix] struct {
	chatRequestChannels map[int64]chan *models.Update
	processingQueueChan chan struct{}

	telegramClient *client.TelegramClient

	commandHandler  map[Command]HandlerInfo
	actionHandler   map[Action]HandlerInfo
	callbackHandler map[Callback]HandlerInfo

	chatMemberHandler      HandlerFunc
	myChatMemberHandler    HandlerFunc
	limiterMessageHandler  HandlerFunc
	chatMigrationHandler   HandlerFunc
	chatJoinRequestHandler HandlerFunc

	actionStorage      storage.UserActionStorage
	messageStorage     storage.UserMessageStorage
	workersCount       int
	limiter            *limiter.UserLimiter
	middlewareFunc     MiddlewareFunc
	processor          *MessageProcessor
	locales            *locale.LocalizationProvider
	notFlowableActions []Action
}

func NewTelegramStateService[Action storage.UserAction, Command BotCommand, Callback CallbackPrefix](
	cfg config.TelegramConfig,
	actionStorage storage.UserActionStorage,
	messageStorage storage.UserMessageStorage,
	client *client.TelegramClient,
	locales *locale.LocalizationProvider,
) *TelegramStateService[Action, Command, Callback] {
	handler := &TelegramStateService[Action, Command, Callback]{
		chatRequestChannels: make(map[int64]chan *models.Update),
		processingQueueChan: make(chan struct{}, cfg.WorkersCount),
		commandHandler:      make(map[Command]HandlerInfo),
		actionHandler:       make(map[Action]HandlerInfo),
		callbackHandler:     make(map[Callback]HandlerInfo),
		telegramClient:      client,

		actionStorage:      actionStorage,
		messageStorage:     messageStorage,
		workersCount:       cfg.WorkersCount,
		limiter:            limiter.NewUserLimiter(rate.Limit(cfg.MessagePerSecond), 1),
		processor:          NewMessageProcessor(),
		locales:            locales,
		notFlowableActions: make([]Action, 0),
	}

	handler.callbackHandler["set-previous-keyboard"] = HandlerInfo{
		Handler: handler.handleSetPreviousKeyboardPage,
	}
	handler.callbackHandler["set-next-keyboard"] = HandlerInfo{
		Handler: handler.handleSetNextKeyboardPage,
	}

	return handler
}

func (t *TelegramStateService[Action, Command, Callback]) handleSetPreviousKeyboardPage(ctx context.Context, update *models.Update) (result error) {
	result = nil
	chat := UpdateChat(update)
	if chat == nil {
		return
	}
	userID := chat.ID
	cbMessageID := callbackMessageID(update)

	if cbMessageID == 0 {
		return nil
	}

	messageInfos, err := t.messageStorage.GetUserMessages(ctx, userID)

	if err != nil {
		logrus.WithError(err).Error("Error getting messages")
		return
	}

	for _, messageInfo := range messageInfos {
		if messageInfo.MessageID == cbMessageID {
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

			err = t.telegramClient.EditMessage(ctx, userID, messageInfo.MessageID, client.WithEditInlineKeyboard(newKeyboard))

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

func (t *TelegramStateService[Action, Command, Callback]) handleSetNextKeyboardPage(ctx context.Context, update *models.Update) (result error) {
	result = nil
	chat := UpdateChat(update)
	if chat == nil {
		return
	}
	userID := chat.ID
	cbMessageID := callbackMessageID(update)

	if cbMessageID == 0 {
		return nil
	}

	messageInfos, err := t.messageStorage.GetUserMessages(ctx, userID)

	if err != nil {
		logrus.WithError(err).Error("Error getting messages")
		return
	}

	for _, messageInfo := range messageInfos {
		if messageInfo.MessageID == cbMessageID {
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

			err = t.telegramClient.EditMessage(ctx, userID, messageInfo.MessageID, client.WithEditInlineKeyboard(newKeyboard))

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

func (t *TelegramStateService[Action, Command, Callback]) AddNotFlowableAction(action Action) *TelegramStateService[Action, Command, Callback] {
	t.notFlowableActions = append(t.notFlowableActions, action)

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

func (t *TelegramStateService[Action, Command, Callback]) RegisterMigrationHandler(handler HandlerFunc) *TelegramStateService[Action, Command, Callback] {
	t.chatMigrationHandler = handler

	return t
}

func (t *TelegramStateService[Action, Command, Callback]) RegisterChatJoinRequestHandler(handler HandlerFunc) *TelegramStateService[Action, Command, Callback] {
	t.chatJoinRequestHandler = handler

	return t
}

func (t *TelegramStateService[Action, Command, Callback]) Run(ctx context.Context) {
	updatesChan := t.telegramClient.GetUpdates(ctx)
	logrus.Info("start telegram updates handler service")
	go t.startConsumeQueueChan(ctx)
	t.telegramClient.RunChatRatesCleanup(ctx)
	go t.limiter.Run(ctx)

	for {
		select {
		case <-ctx.Done():
			return

		case update, ok := <-updatesChan:
			if !ok {
				logrus.Warning("updates chan was closed, polling stopped")
				return
			}

			chat := UpdateChat(update)
			user := UpdateUser(update)

			var chatID, userID int64
			if chat != nil {
				chatID = chat.ID
			}
			if user != nil {
				userID = user.ID
			}

			withRateCheck := true

			if update.ChatMember != nil || update.MyChatMember != nil || update.ChatJoinRequest != nil {
				withRateCheck = false
			}

			log := logrus.WithFields(logrus.Fields{
				"updateID": update.ID,
				"userID":   userID,
				"chatID":   chatID,
			})

			log.Debug("received update from telegram bot")

			if withRateCheck && !t.limiter.Check(userID) {
				log.Debug("rate limit exceeded, skip update")
				if t.limiterMessageHandler != nil {
					if err := t.limiterMessageHandler(ctx, update); err != nil {
						log.WithError(err).Error("failed execute limiter message handler")
					}
				}

				continue
			}

			log.Debug("success check rates by this user")

			if _, ok = t.chatRequestChannels[chatID]; !ok {
				t.chatRequestChannels[chatID] = make(chan *models.Update, 10)
			}

			t.chatRequestChannels[chatID] <- update
			t.processor.PutChat(chatID)
			t.processingQueueChan <- struct{}{}

			log.Debug("update successfully queued for processing")
		}
	}
}

func (t *TelegramStateService[Action, Command, Callback]) startConsumeQueueChan(ctx context.Context) {
	processingChan := make(chan *models.Update, t.workersCount)
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
					t.handleUpdate(ctx, update)
					logrus.WithField("workerID", workerId).Debug("finished processing update")

					chat := UpdateChat(update)
					if chat == nil {
						continue
					}

					omitChatIdsChan <- chat.ID
				}
			}
		}(i)
	}

	ticker := time.NewTicker(time.Millisecond * 100)

	for {
		select {
		case <-ctx.Done():
			return

		case <-t.processingQueueChan:
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
				ticker.Reset(time.Millisecond * 100)
				continue
			}

			update, ok := <-chatRequestChan

			if !ok {
				ticker.Reset(time.Millisecond * 100)
				continue
			}

			log.WithField("updateID", update.ID).Debug("add update to worker processing queue")

			processingChan <- update
			ticker.Reset(time.Millisecond * 100)

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
				ticker.Reset(time.Millisecond * 100)
				continue
			}

			update, ok := <-chatRequestChan

			if !ok {
				ticker.Reset(time.Millisecond * 100)
				continue
			}

			log.WithField("updateID", update.ID).Debug("add update to worker processing queue")

			processingChan <- update
			ticker.Reset(time.Millisecond * 100)
		}
	}
}

func (t *TelegramStateService[Action, Command, Callback]) handleUpdate(ctx context.Context, update *models.Update) {
	log := logrus.WithFields(logrus.Fields{
		"updateID": update.ID,
	})

	if update.Message != nil && update.Message.MigrateToChatID != 0 {
		if t.chatMigrationHandler == nil {
			return
		}

		log.Debug("handle chat migration event")
		if err := t.chatMigrationHandler(ctx, update); err != nil {
			log.WithError(err).Error("failed execute chat migration handler")
		}
		return
	}

	if update.MyChatMember != nil {
		if t.myChatMemberHandler == nil {
			return
		}
		log.Debug("handle my chat member event")
		if err := t.myChatMemberHandler(ctx, update); err != nil {
			log.WithError(err).Error("failed handle my chat member event")
		}
		return
	}

	if update.ChatMember != nil {
		if t.chatMemberHandler == nil {
			return
		}
		log.Debug("handle chat member event")
		if err := t.chatMemberHandler(ctx, update); err != nil {
			log.WithError(err).Error("failed handle chat member event")
		}
		return
	}

	if update.ChatJoinRequest != nil {
		if t.chatJoinRequestHandler == nil {
			return
		}
		log.Debug("handle chat member event")
		if err := t.chatJoinRequestHandler(ctx, update); err != nil {
			log.WithError(err).Error("failed handle chat member event")
		}
		return
	}

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

	if update.Message != nil {
		log.Debug("handle message event")
		t.handleMessage(ctx, update)
		return
	}

	if update.CallbackQuery != nil {
		log.Debug("handle callback event")
		t.handleCallback(ctx, update)

		if err := t.telegramClient.AnswerCallbackQuery(ctx, update.CallbackQuery.ID); err != nil {
			log.WithError(err).Error("failed answer callback query")
		}
		return
	}
}

func (t *TelegramStateService[Action, Command, Callback]) handleCallback(ctx context.Context, update *models.Update) {
	user := UpdateUser(update)
	chat := UpdateChat(update)
	var userID, chatID int64
	if user != nil {
		userID = user.ID
	}
	if chat != nil {
		chatID = chat.ID
	}
	log := logrus.WithFields(logrus.Fields{
		"updateID": update.ID,
		"userID":   userID,
		"chatID":   chatID,
	})

	log.Debug("check is event contains callback data")
	cbData := callbackData(update)
	log.Debug("callback entry: " + cbData)

	callback, _ := UnwrapCallbackData[Callback](cbData)

	log.Debug("parsed callback: " + string(callback))

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

	actionHandler, ok := t.actionHandler[Action(action)]

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

func (t *TelegramStateService[Action, Command, Callback]) handleMessage(ctx context.Context, update *models.Update) {
	user := UpdateUser(update)
	chat := UpdateChat(update)
	var userID, chatID int64
	if user != nil {
		userID = user.ID
	}
	if chat != nil {
		chatID = chat.ID
	}
	log := logrus.WithFields(logrus.Fields{
		"updateID": update.ID,
		"userID":   userID,
		"chatID":   chatID,
	})

	log.Debug("check is event is bot command")

	cmd := MessageCommand(update.Message)

	cmdHandler, ok := t.commandHandler[Command(cmd)]

	if ok {
		log.WithField("command", cmd).Debug("try process validations before call handler")

		if err := t.processValidation(ctx, chatID, update, cmdHandler.MessageValidators, log, 0); err != nil {
			return
		}

		log.WithField("command", cmd).Debug("validations processed, call handler")

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

	actionHandler, ok := t.actionHandler[Action(action)]

	if !ok {
		log.Error("action handler not found")
		return
	}

	isCancel := MessageIsCommand(update.Message) && MessageCommand(update.Message) == "cancel"

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

	if err = t.processValidation(ctx, chatID, update, actionHandler.MessageValidators, log, Action(action)); err != nil {
		return
	}

	log.WithField("action", action).Debug("validations processed, call handler")

	err = actionHandler.Handler(ctx, update)

	if err != nil {
		log.WithError(err).Error("failed handle event")
	}
}

func (t *TelegramStateService[Action, Command, Callback]) processValidation(ctx context.Context, chatID int64, update *models.Update, validators []ValidatorFunc, log *logrus.Entry, action Action) error {
	updateMessageID := 0

	if update.Message != nil {
		updateMessageID = update.Message.ID
	}

	if update.CallbackQuery != nil {
		updateMessageID = callbackMessageID(update)
	}

	for _, validator := range validators {
		if err := validator(update); err != nil {
			log.WithError(err).Error("failed validate update")
			userLang := getLangFromContext(ctx)
			messageID, inErr := t.telegramClient.SendMessage(ctx, chatID, t.locales.GetWithCulture(userLang, err.Error()))

			if inErr != nil {
				log.WithError(err).Error("failed send message to telegram")
				return err
			}

			if slices.Contains(t.notFlowableActions, action) {
				return err
			}

			if inErr = t.messageStorage.SaveUserMessage(ctx, chatID, messageID, false); inErr != nil {
				log.WithError(inErr).Error("failed save message to storage")
			}

			if inErr = t.messageStorage.SaveUserMessage(ctx, chatID, updateMessageID, false); inErr != nil {
				log.WithError(inErr).Error("failed save message to storage")
			}

			return err
		}
	}

	return nil
}

type LangCtxKey struct{}

func getLangFromContext(ctx context.Context) string {
	lang := ctx.Value(LangCtxKey{})

	if lang == nil {
		return ""
	}

	return lang.(string)
}

// UpdateChat extracts the chat associated with the update across all known carriers
// (message, callback query, chat member events, join requests, etc.).
func UpdateChat(u *models.Update) *models.Chat {
	switch {
	case u.Message != nil:
		return &u.Message.Chat
	case u.EditedMessage != nil:
		return &u.EditedMessage.Chat
	case u.ChannelPost != nil:
		return &u.ChannelPost.Chat
	case u.EditedChannelPost != nil:
		return &u.EditedChannelPost.Chat
	case u.CallbackQuery != nil:
		if u.CallbackQuery.Message.Type == models.MaybeInaccessibleMessageTypeMessage && u.CallbackQuery.Message.Message != nil {
			return &u.CallbackQuery.Message.Message.Chat
		}
		if u.CallbackQuery.Message.InaccessibleMessage != nil {
			return &u.CallbackQuery.Message.InaccessibleMessage.Chat
		}
	case u.MyChatMember != nil:
		return &u.MyChatMember.Chat
	case u.ChatMember != nil:
		return &u.ChatMember.Chat
	case u.ChatJoinRequest != nil:
		return &u.ChatJoinRequest.Chat
	}
	return nil
}

// UpdateUser extracts the user associated with the update.
func UpdateUser(u *models.Update) *models.User {
	switch {
	case u.Message != nil:
		return u.Message.From
	case u.EditedMessage != nil:
		return u.EditedMessage.From
	case u.InlineQuery != nil:
		return u.InlineQuery.From
	case u.ChosenInlineResult != nil:
		return &u.ChosenInlineResult.From
	case u.CallbackQuery != nil:
		return &u.CallbackQuery.From
	case u.ShippingQuery != nil:
		return u.ShippingQuery.From
	case u.PreCheckoutQuery != nil:
		return u.PreCheckoutQuery.From
	case u.MyChatMember != nil:
		return &u.MyChatMember.From
	case u.ChatMember != nil:
		return &u.ChatMember.From
	case u.ChatJoinRequest != nil:
		return &u.ChatJoinRequest.From
	}
	return nil
}

// MessageIsCommand reports whether the first entity of the message is a bot command at offset 0.
func MessageIsCommand(m *models.Message) bool {
	if m == nil || len(m.Entities) == 0 {
		return false
	}
	e := m.Entities[0]
	return e.Type == models.MessageEntityTypeBotCommand && e.Offset == 0
}

// MessageCommand returns the command name (without the leading slash or "@botname" suffix)
// for messages whose first entity is a bot command.
func MessageCommand(m *models.Message) string {
	if !MessageIsCommand(m) {
		return ""
	}
	length := m.Entities[0].Length
	if length <= 1 || length > len(m.Text) {
		return ""
	}
	cmd := m.Text[1:length]
	if idx := strings.Index(cmd, "@"); idx >= 0 {
		cmd = cmd[:idx]
	}
	return cmd
}

func callbackData(u *models.Update) string {
	if u.CallbackQuery == nil {
		return ""
	}
	return u.CallbackQuery.Data
}

func callbackMessageID(u *models.Update) int {
	if u.CallbackQuery == nil {
		return 0
	}
	switch u.CallbackQuery.Message.Type {
	case models.MaybeInaccessibleMessageTypeMessage:
		if u.CallbackQuery.Message.Message != nil {
			return u.CallbackQuery.Message.Message.ID
		}
	case models.MaybeInaccessibleMessageTypeInaccessibleMessage:
		return 0
	}
	return 0
}
