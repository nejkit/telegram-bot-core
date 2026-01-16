package client

import (
	"context"
	"encoding/json"
	"fmt"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/nejkit/telegram-bot-core/config"
	"github.com/nejkit/telegram-bot-core/limiter"
	"github.com/sirupsen/logrus"
	"golang.org/x/time/rate"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"time"
)

type TelegramClient struct {
	api            *tgbotapi.BotAPI
	allowedUpdates []string

	chatLimiter   *limiter.UserLimiter
	globalLimiter *rate.Limiter
}

type MessageOptions func(msgCfg *tgbotapi.MessageConfig)
type EditMessageOptions func(msgCfg *tgbotapi.EditMessageTextConfig)

func WithSendInlineKeyboard(keyboard tgbotapi.InlineKeyboardMarkup) MessageOptions {
	return func(msgCfg *tgbotapi.MessageConfig) {
		msgCfg.ReplyMarkup = keyboard
	}
}

func WithSendReplyKeyboard(keyboard tgbotapi.ReplyKeyboardMarkup) MessageOptions {
	return func(msgCfg *tgbotapi.MessageConfig) {
		msgCfg.ReplyMarkup = keyboard
	}
}

func WithRemoveReplyKeyboard() MessageOptions {
	return func(msgCfg *tgbotapi.MessageConfig) {
		msgCfg.ReplyMarkup = tgbotapi.NewRemoveKeyboard(false)
	}
}

func WithEditInlineKeyboard(keyboard tgbotapi.InlineKeyboardMarkup) EditMessageOptions {
	return func(msgCfg *tgbotapi.EditMessageTextConfig) {
		msgCfg.ReplyMarkup = &keyboard
	}
}

func WithEditMessageText(text string) EditMessageOptions {
	return func(msgCfg *tgbotapi.EditMessageTextConfig) {
		msgCfg.Text = text
	}
}

type DownloadFileInfo struct {
	FileName string
	MimoType string
	FileData []byte
}

func NewTelegramClient(cfg *config.TelegramConfig) *TelegramClient {
	botApi, err := tgbotapi.NewBotAPI(cfg.Token)

	if err != nil {
		logrus.WithError(err).Fatal("Error creating telegram client")
	}

	chatLimiter := limiter.NewUserLimiter(rate.Every(time.Second), 2)

	return &TelegramClient{
		api:            botApi,
		allowedUpdates: cfg.AllowedUpdates,
		globalLimiter:  rate.NewLimiter(25, 25),
		chatLimiter:    chatLimiter,
	}
}

func (t *TelegramClient) ValidateWebAppInitData(initData string) (int64, error) {
	valid, err := tgbotapi.ValidateWebAppData(t.api.Token, initData)

	if err != nil {
		return 0, err
	}

	if !valid {
		return 0, fmt.Errorf("invalid telegram init data")
	}

	params, _ := url.ParseQuery(initData)

	userJson := params.Get("user")

	user := struct {
		ID int64 `json:"id"`
	}{}

	if err := json.Unmarshal([]byte(userJson), &user); err != nil {
		return 0, err
	}

	return user.ID, nil
}

func (t *TelegramClient) RunChatRatesCleanup(ctx context.Context) {
	go t.chatLimiter.Run(ctx)
}

func (t *TelegramClient) SendMessage(ctx context.Context, recipientChatID int64, messageText string, options ...MessageOptions) (int, error) {
	cfg := tgbotapi.NewMessage(recipientChatID, messageText)

	for _, opt := range options {
		opt(&cfg)
	}

	if err := t.globalLimiter.Wait(ctx); err != nil {
		return 0, err
	}

	t.chatLimiter.Wait(ctx, recipientChatID)

	response, err := t.api.Send(cfg)

	if err != nil {
		return 0, err
	}

	return response.MessageID, nil
}

func (t *TelegramClient) EditMessage(ctx context.Context, recipientChatID int64, messageID int, options ...EditMessageOptions) error {
	cfg := &tgbotapi.EditMessageTextConfig{
		BaseEdit: tgbotapi.BaseEdit{
			ChatID:    recipientChatID,
			MessageID: messageID,
		},
	}

	for _, opt := range options {
		opt(cfg)
	}

	if err := t.globalLimiter.Wait(ctx); err != nil {
		return err
	}

	t.chatLimiter.Wait(ctx, recipientChatID)

	_, err := t.api.Send(cfg)

	return err
}

func (t *TelegramClient) DeleteMessage(ctx context.Context, recipientChatID int64, messageID int) error {
	cfg := tgbotapi.NewDeleteMessage(recipientChatID, messageID)

	if err := t.globalLimiter.Wait(ctx); err != nil {
		return err
	}

	t.chatLimiter.Wait(ctx, recipientChatID)

	_, err := t.api.Request(cfg)

	return err
}

func (t *TelegramClient) UploadFile(ctx context.Context, recipientChatID int64, fileName string, fileContent []byte) (int, string, error) {
	cfg := tgbotapi.NewDocument(recipientChatID, tgbotapi.FileBytes{
		Name:  fileName,
		Bytes: fileContent,
	})

	if err := t.globalLimiter.Wait(ctx); err != nil {
		return 0, "", err
	}

	t.chatLimiter.Wait(ctx, recipientChatID)

	response, err := t.api.Send(cfg)

	if err != nil {
		return 0, "", err
	}

	return response.MessageID, response.Document.FileID, nil
}

func (t *TelegramClient) SendFileByID(ctx context.Context, recipientChatID int64, fileID string) (int, error) {
	cfg := tgbotapi.NewDocument(recipientChatID, tgbotapi.FileID(fileID))

	if err := t.globalLimiter.Wait(ctx); err != nil {
		return 0, err
	}

	t.chatLimiter.Wait(ctx, recipientChatID)

	response, err := t.api.Send(cfg)

	if err != nil {
		return 0, err
	}

	return response.MessageID, nil
}

func (t *TelegramClient) CopyMessage(ctx context.Context, fromChatID, toChatID int64, messageID int) (int, error) {
	cfg := tgbotapi.NewCopyMessage(toChatID, fromChatID, messageID)

	if err := t.globalLimiter.Wait(ctx); err != nil {
		return 0, err
	}

	t.chatLimiter.Wait(ctx, toChatID)

	response, err := t.api.Send(cfg)

	if err != nil {
		return 0, err
	}

	return response.MessageID, nil
}

func (t *TelegramClient) DownloadFile(ctx context.Context, fileID string) (*DownloadFileInfo, error) {
	if err := t.globalLimiter.Wait(ctx); err != nil {
		return nil, err
	}

	file, err := t.api.GetFile(tgbotapi.FileConfig{FileID: fileID})

	if err != nil {
		return nil, err
	}

	httpResp, err := http.Get(file.Link(t.api.Token))

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

func (t *TelegramClient) AnswerCallback(callbackID, messageText string) error {
	cfg := tgbotapi.NewCallback(callbackID, messageText)

	_, err := t.api.Request(cfg)

	return err
}

func (t *TelegramClient) GetInviteLink(ctx context.Context, secret string) (string, error) {
	if err := t.globalLimiter.Wait(ctx); err != nil {
		return "", err
	}

	me, err := t.api.GetMe()

	if err != nil {
		return "", err
	}

	return fmt.Sprintf("https://telegram.me/%s?start=%s", me.UserName, secret), nil
}

func (t *TelegramClient) GetBotCommands(ctx context.Context, fromChatID int64) ([]tgbotapi.BotCommand, error) {
	if err := t.globalLimiter.Wait(ctx); err != nil {
		return nil, err
	}

	commands, err := t.api.GetMyCommandsWithConfig(tgbotapi.NewGetMyCommandsWithScope(tgbotapi.NewBotCommandScopeChat(fromChatID)))

	if err != nil {
		return nil, err
	}

	return commands, nil
}

func (t *TelegramClient) SetBotCommands(ctx context.Context, toChatID int64, commands []tgbotapi.BotCommand) error {
	if err := t.globalLimiter.Wait(ctx); err != nil {
		return err
	}

	cfg := tgbotapi.NewSetMyCommandsWithScope(tgbotapi.NewBotCommandScopeChat(toChatID), commands...)

	_, err := t.api.Request(cfg)

	return err
}

func (t *TelegramClient) KickUserFromChat(ctx context.Context, fromChatID, userID int64, withBan bool) error {
	if err := t.globalLimiter.Wait(ctx); err != nil {
		return err
	}

	_, err := t.api.Request(tgbotapi.KickChatMemberConfig{
		ChatMemberConfig: tgbotapi.ChatMemberConfig{
			ChatID: fromChatID,
			UserID: userID,
		},
		RevokeMessages: false,
	})

	if err != nil || withBan {
		return err
	}

	_, err = t.api.Request(tgbotapi.UnbanChatMemberConfig{
		ChatMemberConfig: tgbotapi.ChatMemberConfig{
			ChatID: fromChatID,
			UserID: userID,
		},
		OnlyIfBanned: true,
	})

	return err
}

func (t *TelegramClient) GetContactInfo(ctx context.Context, userID int64) (*tgbotapi.Chat, error) {
	if err := t.globalLimiter.Wait(ctx); err != nil {
		return nil, err
	}

	contact, err := t.api.GetChat(tgbotapi.ChatInfoConfig{ChatConfig: tgbotapi.ChatConfig{ChatID: userID}})

	if err != nil {
		return nil, err
	}

	return &contact, nil
}

func (t *TelegramClient) GetUpdates() tgbotapi.UpdatesChannel {
	return t.api.GetUpdatesChan(tgbotapi.UpdateConfig{
		Offset:         0,
		Limit:          10,
		Timeout:        30,
		AllowedUpdates: t.allowedUpdates,
	})
}
