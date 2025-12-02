package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/nejkit/telegram-bot-core/domain"
	"github.com/nejkit/telegram-bot-core/wrapper"
	"github.com/redis/go-redis/v9"
)

type KeyboardInfo struct {
	Keyboards       []tgbotapi.InlineKeyboardMarkup
	CurrentPosition int
}

type MessageInfo struct {
	MessageID      int           `json:"message_id"`
	ChatID         int64         `json:"chat_id"`
	InlineKeyboard *KeyboardInfo `json:"inline_keyboard,omitempty"`
}

func (s *UserMessageStorage) getMessagesKey(identifier string) string {
	return fmt.Sprintf("%s:user:message:%s", s.botInstancePrefix, identifier)
}

type UserMessageStorage struct {
	botInstancePrefix string
	client            *redis.Client
}

func NewUserMessageStorage(
	botInstancePrefix string,
	client *redis.Client,
) *UserMessageStorage {
	return &UserMessageStorage{botInstancePrefix: botInstancePrefix, client: client}
}

func (s *UserMessageStorage) SaveCallbackMessage(ctx context.Context, callbackID string, chatID int64, messageID int) error {
	payload := &MessageInfo{
		MessageID: messageID,
		ChatID:    chatID,
	}

	payloadBytes, err := json.Marshal(payload)

	if err != nil {
		return err
	}

	return s.client.Set(ctx, s.getMessagesKey(callbackID), payloadBytes, 0).Err()
}

func (s *UserMessageStorage) GetCallbackMessage(ctx context.Context, callbackID string) (*MessageInfo, error) {
	rawData, err := s.client.Get(ctx, s.getMessagesKey(callbackID)).Bytes()

	if errors.Is(err, redis.Nil) {
		return nil, domain.ErrorMessageNotFound
	}

	if err != nil {
		return nil, err
	}

	var message MessageInfo

	err = json.Unmarshal(rawData, &message)

	if err != nil {
		return nil, err
	}

	return &message, nil
}

func (s *UserMessageStorage) DeleteCallbackMessage(ctx context.Context, callbackID string) error {
	return s.client.Del(ctx, s.getMessagesKey(callbackID)).Err()
}

func (s *UserMessageStorage) SaveUserMessage(ctx context.Context, messageID int) error {
	chatID, ok := wrapper.GetChatID(ctx)
	if !ok {
		return domain.ErrorChatNotFilled
	}

	payload := &MessageInfo{
		MessageID: messageID,
		ChatID:    chatID,
	}

	payloadBytes, err := json.Marshal(payload)

	if err != nil {
		return err
	}

	return s.client.SAdd(ctx, s.getMessagesKey(fmt.Sprint(chatID)), payloadBytes, 0).Err()
}

func (s *UserMessageStorage) GetUserMessages(ctx context.Context) ([]MessageInfo, error) {
	chatID, ok := wrapper.GetChatID(ctx)
	if !ok {
		return nil, domain.ErrorChatNotFilled
	}

	rawMessages, err := s.client.SMembers(ctx, s.getMessagesKey(fmt.Sprint(chatID))).Result()

	if err != nil {
		return nil, err
	}

	var messages []MessageInfo

	for _, rawMessage := range rawMessages {
		var message MessageInfo

		if err := json.Unmarshal([]byte(rawMessage), &message); err != nil {
			return nil, err
		}

		messages = append(messages, message)
	}

	return messages, nil
}

func (s *UserMessageStorage) DeleteUserMessage(ctx context.Context) error {
	chatID, ok := wrapper.GetChatID(ctx)
	if !ok {
		return domain.ErrorChatNotFilled
	}

	return s.client.Del(ctx, s.getMessagesKey(fmt.Sprint(chatID))).Err()
}
