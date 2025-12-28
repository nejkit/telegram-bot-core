package storage

import (
	"context"
	"time"
)

type UserActionStorage interface {
	SaveAction(ctx context.Context, userID int64, action int) error
	GetAction(ctx context.Context, userID int64) (action int, err error)
	SaveActionWithRollback(ctx context.Context, userID int64, action int) (func(err error) error, error)
}

type InvitesStorage interface {
	SaveInvite(ctx context.Context, deepLinkSecret string, fromUserID int64, expiration time.Duration) error
	GetInvite(ctx context.Context, deepLinkSecret string) (int64, error)
	DeleteInvite(ctx context.Context, deepLinkSecret string) error
}

type UserMessageStorage interface {
	SaveCallbackMessage(ctx context.Context, callbackID string, chatID int64, messageID int) error
	GetCallbackMessage(ctx context.Context, callbackID string) (*MessageInfo, error)
	DeleteCallbackMessage(ctx context.Context, callbackID string) error
	SaveUserMessage(ctx context.Context, chatID int64, messageID int, withKeyboard bool) error
	GetUserMessages(ctx context.Context, chatID int64) ([]MessageInfo, error)
	DeleteUserMessage(ctx context.Context, chatID int64) error
	SaveKeyboardInfo(ctx context.Context, chatID int64, messageID int, keyboard *KeyboardInfo) error
	GetKeyboardInfo(ctx context.Context, chatID int64, messageID int) (*KeyboardInfo, error)
	DeleteKeyboardInfo(ctx context.Context, chatID int64, messageID int) error
}
