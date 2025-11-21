package domain

import "errors"

var (
	ErrorCallerNotFilled = errors.New("caller not filled")
	ErrorChatNotFilled   = errors.New("chat not filled")
	ErrorMessageNotFound = errors.New("message not found")
	ErrorInviteIsExpired = errors.New("invite is expired")
)
