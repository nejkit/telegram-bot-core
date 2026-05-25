package client

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/nejkit/telegram-bot-core/config"
	"github.com/nejkit/telegram-bot-core/limiter"
	"github.com/sirupsen/logrus"
	"golang.org/x/time/rate"
)

const updatesBufferSize = 1024

type TelegramClient struct {
	api            *bot.Bot
	allowedUpdates []string

	chatLimiter   *limiter.UserLimiter
	globalLimiter *rate.Limiter

	updates   chan *models.Update
	startOnce sync.Once
}

type MessageOptions func(msgCfg *bot.SendMessageParams)
type EditMessageOptions func(msgCfg *bot.EditMessageTextParams)

func WithSendInlineKeyboard(keyboard models.InlineKeyboardMarkup) MessageOptions {
	return func(msgCfg *bot.SendMessageParams) {
		msgCfg.ReplyMarkup = keyboard
	}
}

func WithSendReplyKeyboard(keyboard models.ReplyKeyboardMarkup) MessageOptions {
	return func(msgCfg *bot.SendMessageParams) {
		msgCfg.ReplyMarkup = keyboard
	}
}

func WithRemoveReplyKeyboard() MessageOptions {
	return func(msgCfg *bot.SendMessageParams) {
		msgCfg.ReplyMarkup = models.ReplyKeyboardRemove{RemoveKeyboard: true}
	}
}

func WithEditInlineKeyboard(keyboard models.InlineKeyboardMarkup) EditMessageOptions {
	return func(msgCfg *bot.EditMessageTextParams) {
		msgCfg.ReplyMarkup = keyboard
	}
}

func WithEditMessageText(text string) EditMessageOptions {
	return func(msgCfg *bot.EditMessageTextParams) {
		msgCfg.Text = text
	}
}

type DownloadFileInfo struct {
	FileName string
	MimoType string
	FileData []byte
}

func NewTelegramClient(cfg *config.TelegramConfig) *TelegramClient {
	transport := &RetryTransport{
		Base:    http.DefaultTransport,
		Retries: 3,
		Wait:    time.Second,
	}

	httpClient := &http.Client{Transport: transport}

	t := &TelegramClient{
		allowedUpdates: cfg.AllowedUpdates,
		globalLimiter:  rate.NewLimiter(25, 25),
		chatLimiter:    limiter.NewUserLimiter(rate.Every(time.Second), 2),
		updates:        make(chan *models.Update, updatesBufferSize),
	}

	opts := []bot.Option{
		bot.WithHTTPClient(time.Minute, httpClient),
		bot.WithDefaultHandler(t.bridgeHandler),
	}

	if cfg.TelegramApiUrl != "" {
		opts = append(opts, bot.WithServerURL(cfg.TelegramApiUrl))
	}

	if len(cfg.AllowedUpdates) > 0 {
		opts = append(opts, bot.WithAllowedUpdates(bot.AllowedUpdates(cfg.AllowedUpdates)))
	}

	botApi, err := bot.New(cfg.Token, opts...)

	if err != nil {
		logrus.WithError(err).Fatal("Error creating telegram client")
	}

	t.api = botApi

	return t
}

func (t *TelegramClient) bridgeHandler(ctx context.Context, _ *bot.Bot, update *models.Update) {
	select {
	case <-ctx.Done():
		return
	case t.updates <- update:
	}
}

func (t *TelegramClient) ValidateWebAppInitData(initData string) (int64, error) {
	values, err := url.ParseQuery(initData)

	if err != nil {
		return 0, err
	}

	user, ok := bot.ValidateWebappRequest(values, t.api.Token())

	if !ok {
		return 0, fmt.Errorf("invalid telegram init data")
	}

	return user.ID, nil
}

func (t *TelegramClient) RunChatRatesCleanup(ctx context.Context) {
	go t.chatLimiter.Run(ctx)
}

func (t *TelegramClient) SendMessage(ctx context.Context, recipientChatID int64, messageText string, options ...MessageOptions) (int, error) {
	cfg := &bot.SendMessageParams{
		ChatID: recipientChatID,
		Text:   messageText,
	}

	for _, opt := range options {
		opt(cfg)
	}

	if err := t.globalLimiter.Wait(ctx); err != nil {
		return 0, err
	}

	t.chatLimiter.Wait(ctx, recipientChatID)

	response, err := t.api.SendMessage(ctx, cfg)

	if err != nil {
		return 0, t.handleError(err)
	}

	return response.ID, nil
}

func (t *TelegramClient) EditMessage(ctx context.Context, recipientChatID int64, messageID int, options ...EditMessageOptions) error {
	cfg := &bot.EditMessageTextParams{
		ChatID:    recipientChatID,
		MessageID: messageID,
	}

	for _, opt := range options {
		opt(cfg)
	}

	if err := t.globalLimiter.Wait(ctx); err != nil {
		return err
	}

	t.chatLimiter.Wait(ctx, recipientChatID)

	_, err := t.api.EditMessageText(ctx, cfg)

	return t.handleError(err)
}

func (t *TelegramClient) EditMessageKeyboard(ctx context.Context, recipientChatID int64, messageID int, keyboard *models.InlineKeyboardMarkup) error {
	cfg := &bot.EditMessageReplyMarkupParams{
		ChatID:    recipientChatID,
		MessageID: messageID,
	}

	if keyboard != nil {
		cfg.ReplyMarkup = *keyboard
	}

	if err := t.globalLimiter.Wait(ctx); err != nil {
		return err
	}

	t.chatLimiter.Wait(ctx, recipientChatID)

	_, err := t.api.EditMessageReplyMarkup(ctx, cfg)

	return t.handleError(err)
}

func (t *TelegramClient) DeleteMessage(ctx context.Context, recipientChatID int64, messageID int) error {
	if err := t.globalLimiter.Wait(ctx); err != nil {
		return err
	}

	t.chatLimiter.Wait(ctx, recipientChatID)

	_, err := t.api.DeleteMessage(ctx, &bot.DeleteMessageParams{
		ChatID:    recipientChatID,
		MessageID: messageID,
	})

	return t.handleError(err)
}

func (t *TelegramClient) UploadFile(ctx context.Context, recipientChatID int64, fileName string, fileContent []byte) (int, string, error) {
	if err := t.globalLimiter.Wait(ctx); err != nil {
		return 0, "", err
	}

	t.chatLimiter.Wait(ctx, recipientChatID)

	response, err := t.api.SendDocument(ctx, &bot.SendDocumentParams{
		ChatID: recipientChatID,
		Document: &models.InputFileUpload{
			Filename: fileName,
			Data:     strings.NewReader(string(fileContent)),
		},
	})

	if err != nil {
		return 0, "", t.handleError(err)
	}

	var fileID string

	if response.Document != nil {
		fileID = response.Document.FileID
	}

	return response.ID, fileID, nil
}

func (t *TelegramClient) SendFileByID(ctx context.Context, recipientChatID int64, fileID string) (int, error) {
	if err := t.globalLimiter.Wait(ctx); err != nil {
		return 0, err
	}

	t.chatLimiter.Wait(ctx, recipientChatID)

	response, err := t.api.SendDocument(ctx, &bot.SendDocumentParams{
		ChatID:   recipientChatID,
		Document: &models.InputFileString{Data: fileID},
	})

	if err != nil {
		return 0, t.handleError(err)
	}

	return response.ID, nil
}

func (t *TelegramClient) CopyMessage(ctx context.Context, fromChatID, toChatID int64, messageID int) (int, error) {
	if err := t.globalLimiter.Wait(ctx); err != nil {
		return 0, err
	}

	t.chatLimiter.Wait(ctx, toChatID)

	response, err := t.api.CopyMessage(ctx, &bot.CopyMessageParams{
		ChatID:     toChatID,
		FromChatID: fromChatID,
		MessageID:  messageID,
	})

	if err != nil {
		return 0, t.handleError(err)
	}

	return response.ID, nil
}

func (t *TelegramClient) DownloadFile(ctx context.Context, fileID string) (*DownloadFileInfo, error) {
	if err := t.globalLimiter.Wait(ctx); err != nil {
		return nil, err
	}

	file, err := t.api.GetFile(ctx, &bot.GetFileParams{FileID: fileID})

	if err != nil {
		return nil, t.handleError(err)
	}

	httpResp, err := http.Get(t.api.FileDownloadLink(file))

	if err != nil {
		return nil, err
	}

	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			logrus.WithError(err).Error("failed to close file reader")
		}
	}(httpResp.Body)

	body, err := io.ReadAll(httpResp.Body)

	if err != nil {
		return nil, err
	}

	mimoType := http.DetectContentType(body)
	fileName := filepath.Base(file.FilePath)

	return &DownloadFileInfo{
		FileName: fileName,
		MimoType: mimoType,
		FileData: body,
	}, nil
}

func (t *TelegramClient) AnswerCallback(ctx context.Context, callbackID, messageText string) error {
	_, err := t.api.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
		CallbackQueryID: callbackID,
		Text:            messageText,
	})

	return t.handleError(err)
}

func (t *TelegramClient) GetInviteLink(ctx context.Context, secret string) (string, error) {
	if err := t.globalLimiter.Wait(ctx); err != nil {
		return "", err
	}

	me, err := t.api.GetMe(ctx)

	if err != nil {
		return "", t.handleError(err)
	}

	return fmt.Sprintf("https://telegram.me/%s?start=%s", me.Username, secret), nil
}

func (t *TelegramClient) AnswerCallbackQuery(ctx context.Context, callbackID string) error {
	if err := t.globalLimiter.Wait(ctx); err != nil {
		return err
	}

	_, err := t.api.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
		CallbackQueryID: callbackID,
	})

	return t.handleError(err)
}

func (t *TelegramClient) ProcessChatJoinRequest(ctx context.Context, chatID int64, userID int64, accept bool) error {
	if err := t.globalLimiter.Wait(ctx); err != nil {
		return err
	}

	t.chatLimiter.Wait(ctx, chatID)

	if accept {
		_, err := t.api.ApproveChatJoinRequest(ctx, &bot.ApproveChatJoinRequestParams{
			ChatID: chatID,
			UserID: userID,
		})

		return t.handleError(err)
	}

	_, err := t.api.DeclineChatJoinRequest(ctx, &bot.DeclineChatJoinRequestParams{
		ChatID: chatID,
		UserID: userID,
	})

	return t.handleError(err)
}

func (t *TelegramClient) GetBotCommands(ctx context.Context, fromChatID int64) ([]models.BotCommand, error) {
	if err := t.globalLimiter.Wait(ctx); err != nil {
		return nil, err
	}

	commands, err := t.api.GetMyCommands(ctx, &bot.GetMyCommandsParams{
		Scope: &models.BotCommandScopeChat{ChatID: fromChatID},
	})

	if err != nil {
		return nil, t.handleError(err)
	}

	return commands, nil
}

func (t *TelegramClient) SetBotCommands(ctx context.Context, toChatID int64, commands []models.BotCommand) error {
	if err := t.globalLimiter.Wait(ctx); err != nil {
		return err
	}

	_, err := t.api.SetMyCommands(ctx, &bot.SetMyCommandsParams{
		Commands: commands,
		Scope:    &models.BotCommandScopeChat{ChatID: toChatID},
	})

	return t.handleError(err)
}

func (t *TelegramClient) KickUserFromChat(ctx context.Context, fromChatID, userID int64, withBan bool) error {
	if err := t.globalLimiter.Wait(ctx); err != nil {
		return err
	}

	_, err := t.api.BanChatMember(ctx, &bot.BanChatMemberParams{
		ChatID: fromChatID,
		UserID: userID,
	})

	if err != nil || withBan {
		return t.handleError(err)
	}

	_, err = t.api.UnbanChatMember(ctx, &bot.UnbanChatMemberParams{
		ChatID:       fromChatID,
		UserID:       userID,
		OnlyIfBanned: true,
	})

	return t.handleError(err)
}

func (t *TelegramClient) GetContactInfo(ctx context.Context, userID int64) (*models.ChatFullInfo, error) {
	if err := t.globalLimiter.Wait(ctx); err != nil {
		return nil, err
	}

	contact, err := t.api.GetChat(ctx, &bot.GetChatParams{ChatID: userID})

	if err != nil {
		return nil, t.handleError(err)
	}

	return contact, nil
}

func (t *TelegramClient) CheckUserExistenceInChat(ctx context.Context, chatID, userID int64) (bool, error) {
	if err := t.globalLimiter.Wait(ctx); err != nil {
		return false, err
	}

	member, err := t.api.GetChatMember(ctx, &bot.GetChatMemberParams{
		ChatID: chatID,
		UserID: userID,
	})

	if err != nil {
		return false, t.handleError(err)
	}

	switch member.Type {
	case models.ChatMemberTypeLeft, models.ChatMemberTypeBanned:
		return false, nil
	}

	return true, nil
}

func (t *TelegramClient) GetUpdates(ctx context.Context) <-chan *models.Update {
	t.startOnce.Do(func() {
		go t.api.Start(ctx)
	})

	return t.updates
}

func (t *TelegramClient) handleError(err error) error {
	if err == nil {
		return nil
	}

	if errors.Is(err, bot.ErrorForbidden) {
		msg := err.Error()
		if strings.Contains(msg, "bot was blocked by the user") ||
			strings.Contains(msg, "user is deactivated") {
			return nil
		}
	}

	return err
}
