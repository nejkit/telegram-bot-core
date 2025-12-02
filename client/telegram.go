package client

import (
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/nejkit/telegram-bot-core/config"
	"github.com/sirupsen/logrus"
	"io"
	"net/http"
	"path/filepath"
)

type TelegramClient struct {
	api            *tgbotapi.BotAPI
	allowedUpdates []string
}

type MessageOptions func(msgCfg *tgbotapi.MessageConfig)
type EditMessageOptions func(msgCfg *tgbotapi.EditMessageTextConfig)

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

	return &TelegramClient{
		api:            botApi,
		allowedUpdates: cfg.AllowedUpdates,
	}
}

func (t *TelegramClient) SendMessage(recipientChatID int64, messageText string, options ...MessageOptions) (int, error) {
	cfg := tgbotapi.NewMessage(recipientChatID, messageText)

	for _, opt := range options {
		opt(&cfg)
	}

	response, err := t.api.Send(cfg)

	if err != nil {
		return 0, err
	}

	return response.MessageID, nil
}

func (t *TelegramClient) EditMessage(recipientChatID int64, messageID int, options ...EditMessageOptions) error {
	cfg := &tgbotapi.EditMessageTextConfig{
		BaseEdit: tgbotapi.BaseEdit{
			ChatID:    recipientChatID,
			MessageID: messageID,
		},
	}

	for _, opt := range options {
		opt(cfg)
	}

	_, err := t.api.Send(cfg)

	return err
}

func (t *TelegramClient) DeleteMessage(recipientChatID int64, messageID int) error {
	cfg := tgbotapi.NewDeleteMessage(recipientChatID, messageID)

	_, err := t.api.Request(cfg)

	return err
}

func (t *TelegramClient) UploadFile(recipientChatID int64, fileName string, fileContent []byte) (int, error) {
	cfg := tgbotapi.NewDocument(recipientChatID, tgbotapi.FileBytes{
		Name:  fileName,
		Bytes: fileContent,
	})

	response, err := t.api.Send(cfg)

	if err != nil {
		return 0, err
	}

	return response.MessageID, nil
}

func (t *TelegramClient) DownloadFile(fileID string) (*DownloadFileInfo, error) {
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

func (t *TelegramClient) GetBotCommands(fromChatID int64) ([]tgbotapi.BotCommand, error) {
	commands, err := t.api.GetMyCommandsWithConfig(tgbotapi.NewGetMyCommandsWithScope(tgbotapi.NewBotCommandScopeChat(fromChatID)))

	if err != nil {
		return nil, err
	}

	return commands, nil
}

func (t *TelegramClient) SetBotCommands(toChatID int64, commands []tgbotapi.BotCommand) error {
	cfg := tgbotapi.NewSetMyCommandsWithScope(tgbotapi.NewBotCommandScopeChat(toChatID), commands...)

	_, err := t.api.Request(cfg)

	return err
}

func (t *TelegramClient) KickUserFromChat(fromChatID, userID int64, withBan bool) error {
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

func (t *TelegramClient) GetContactInfo(userID int64) (*tgbotapi.Chat, error) {
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
		Timeout:        10,
		AllowedUpdates: t.allowedUpdates,
	})
}
