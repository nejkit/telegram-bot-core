package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/dgraph-io/ristretto"
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

func (s *RedisUserMessageStorage) getMessagesKey(identifier string) string {
	return fmt.Sprintf("%s:user:message:%s", s.botInstancePrefix, identifier)
}

func (s *RedisUserMessageStorage) getKeyboardsKey(userID int64, messageID int) string {
	return fmt.Sprintf("%s:user:keyboard:%d:%d", s.botInstancePrefix, userID, messageID)
}

type RedisUserMessageStorage struct {
	botInstancePrefix string
	client            *redis.Client
}

func NewRedisUserMessageStorage(
	botInstancePrefix string,
	client *redis.Client,
) *RedisUserMessageStorage {
	return &RedisUserMessageStorage{botInstancePrefix: botInstancePrefix, client: client}
}

func (s *RedisUserMessageStorage) SaveCallbackMessage(ctx context.Context, callbackID string, chatID int64, messageID int) error {
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

func (s *RedisUserMessageStorage) GetCallbackMessage(ctx context.Context, callbackID string) (*MessageInfo, error) {
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

func (s *RedisUserMessageStorage) DeleteCallbackMessage(ctx context.Context, callbackID string) error {
	return s.client.Del(ctx, s.getMessagesKey(callbackID)).Err()
}

func (s *RedisUserMessageStorage) SaveUserMessage(ctx context.Context, chatID int64, messageID int, withKeyboard bool) error {
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

func (s *RedisUserMessageStorage) GetUserMessages(ctx context.Context, chatID int64) ([]MessageInfo, error) {
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

func (s *RedisUserMessageStorage) DeleteUserMessage(ctx context.Context, chatID int64) error {
	return s.client.Del(ctx, s.getMessagesKey(fmt.Sprint(chatID))).Err()
}

func (s *RedisUserMessageStorage) SaveKeyboardInfo(ctx context.Context, chatID int64, messageID int, keyboard *KeyboardInfo) error {
	rawData, err := json.Marshal(keyboard)

	if err != nil {
		return err
	}

	return s.client.Set(ctx, s.getKeyboardsKey(chatID, messageID), rawData, 0).Err()
}

func (s *RedisUserMessageStorage) GetKeyboardInfo(ctx context.Context, chatID int64, messageID int) (*KeyboardInfo, error) {
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

func (s *RedisUserMessageStorage) DeleteKeyboardInfo(ctx context.Context, chatID int64, messageID int) error {
	return s.client.Del(ctx, s.getKeyboardsKey(chatID, messageID)).Err()
}

type InMemoryUserMessageStorage struct {
	client *ristretto.Cache
}

func NewInMemoryUserMessageStorage(client *ristretto.Cache) *InMemoryUserMessageStorage {
	return &InMemoryUserMessageStorage{client: client}
}

func (i *InMemoryUserMessageStorage) getMessagesKey(identifier string) string {
	return fmt.Sprintf("user:message:%s", identifier)
}

func (i *InMemoryUserMessageStorage) getKeyboardsKey(userID int64, messageID int) string {
	return fmt.Sprintf("user:keyboard:%d:%d", userID, messageID)
}

func (i *InMemoryUserMessageStorage) SaveCallbackMessage(_ context.Context, callbackID string, chatID int64, messageID int) error {
	if ok := i.client.Set(i.getMessagesKey(callbackID), &MessageInfo{
		MessageID:      messageID,
		ChatID:         chatID,
		InlineKeyboard: false,
	}, 0); !ok {
		return errors.New("failed to save callback message")
	}

	return nil
}

func (i *InMemoryUserMessageStorage) GetCallbackMessage(_ context.Context, callbackID string) (*MessageInfo, error) {
	data, ok := i.client.Get(i.getMessagesKey(callbackID))

	if !ok {
		return nil, domain.ErrorMessageNotFound
	}

	return data.(*MessageInfo), nil
}

func (i *InMemoryUserMessageStorage) DeleteCallbackMessage(_ context.Context, callbackID string) error {
	i.client.Del(i.getMessagesKey(callbackID))
	return nil
}

func (i *InMemoryUserMessageStorage) SaveUserMessage(_ context.Context, chatID int64, messageID int, withKeyboard bool) error {
	if ok := i.client.Set(i.getMessagesKey(fmt.Sprint(chatID)), &MessageInfo{
		MessageID:      messageID,
		ChatID:         chatID,
		InlineKeyboard: withKeyboard,
	}, 0); !ok {
		return errors.New("failed to save callback message")
	}

	return nil
}

func (i *InMemoryUserMessageStorage) GetUserMessages(_ context.Context, chatID int64) ([]MessageInfo, error) {
	data, ok := i.client.Get(i.getMessagesKey(fmt.Sprint(chatID)))
	if !ok {
		return nil, domain.ErrorMessageNotFound
	}

	return data.([]MessageInfo), nil
}

func (i *InMemoryUserMessageStorage) DeleteUserMessage(_ context.Context, chatID int64) error {
	i.client.Del(i.getMessagesKey(fmt.Sprint(chatID)))
	return nil
}

func (i *InMemoryUserMessageStorage) SaveKeyboardInfo(_ context.Context, chatID int64, messageID int, keyboard *KeyboardInfo) error {
	if ok := i.client.Set(i.getKeyboardsKey(chatID, messageID), keyboard, 0); !ok {
		return errors.New("failed to save keyboard info")
	}

	return nil
}

func (i *InMemoryUserMessageStorage) GetKeyboardInfo(_ context.Context, chatID int64, messageID int) (*KeyboardInfo, error) {
	data, ok := i.client.Get(i.getKeyboardsKey(chatID, messageID))

	if !ok {
		return nil, domain.ErrorMessageNotFound
	}

	return data.(*KeyboardInfo), nil
}

func (i *InMemoryUserMessageStorage) DeleteKeyboardInfo(_ context.Context, chatID int64, messageID int) error {
	i.client.Del(i.getKeyboardsKey(chatID, messageID))
	return nil
}
