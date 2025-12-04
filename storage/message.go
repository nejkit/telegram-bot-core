package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/nejkit/telegram-bot-core/domain"
	"github.com/redis/go-redis/v9"
)

type KeyboardInfo struct {
	Keyboards       []tgbotapi.InlineKeyboardMarkup
	CurrentPosition int
}

type MessageInfo struct {
	MessageID      int   `json:"message_id"`
	ChatID         int64 `json:"chat_id"`
	InlineKeyboard bool  `json:"inline_keyboard,omitempty"`
}

func (s *UserMessageStorage) getMessagesKey(identifier string) string {
	return fmt.Sprintf("%s:user:message:%s", s.botInstancePrefix, identifier)
}

func (s *UserMessageStorage) getKeyboardsKey(userID int64, messageID int) string {
	return fmt.Sprintf("%s:user:keyboard:%d:%d", s.botInstancePrefix, userID, messageID)
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

func (s *UserMessageStorage) SaveUserMessage(ctx context.Context, chatID int64, messageID int, withKeyboard bool) error {
	payload := &MessageInfo{
		MessageID:      messageID,
		ChatID:         chatID,
		InlineKeyboard: withKeyboard,
	}

	payloadBytes, err := json.Marshal(payload)

	if err != nil {
		return err
	}

	return s.client.SAdd(ctx, s.getMessagesKey(fmt.Sprint(chatID)), payloadBytes).Err()
}

func (s *UserMessageStorage) GetUserMessages(ctx context.Context, chatID int64) ([]MessageInfo, error) {
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

func (s *UserMessageStorage) DeleteUserMessage(ctx context.Context, chatID int64) error {
	return s.client.Del(ctx, s.getMessagesKey(fmt.Sprint(chatID))).Err()
}

func (s *UserMessageStorage) SaveKeyboardInfo(ctx context.Context, chatID int64, messageID int, keyboard *KeyboardInfo) error {
	rawData, err := json.Marshal(keyboard)

	if err != nil {
		return err
	}

	return s.client.Set(ctx, s.getKeyboardsKey(chatID, messageID), rawData, 0).Err()
}

func (s *UserMessageStorage) GetKeyboardInfo(ctx context.Context, chatID int64, messageID int) (*KeyboardInfo, error) {
	rawData, err := s.client.Get(ctx, s.getKeyboardsKey(chatID, messageID)).Bytes()

	if err != nil {
		return nil, err
	}

	var keyboard KeyboardInfo

	err = json.Unmarshal(rawData, &keyboard)

	if err != nil {
		return nil, err
	}

	return &keyboard, nil
}

func (s *UserMessageStorage) DeleteKeyboardInfo(ctx context.Context, chatID int64, messageID int) error {
	return s.client.Del(ctx, s.getKeyboardsKey(chatID, messageID)).Err()
}
