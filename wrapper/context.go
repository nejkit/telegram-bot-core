package wrapper

import "context"

type CallerChatCtxKey struct{}

type FromChatCtxKey struct{}

func FillCtx(ctx context.Context, chatID, userID int64) context.Context {
	ctx = context.WithValue(ctx, FromChatCtxKey{}, chatID)
	ctx = context.WithValue(ctx, CallerChatCtxKey{}, userID)

	return ctx
}

func GetChatID(ctx context.Context) (int64, bool) {
	chatID, ok := ctx.Value(FromChatCtxKey{}).(int64)
	return chatID, ok
}

func GetUserID(ctx context.Context) (int64, bool) {
	userID, ok := ctx.Value(CallerChatCtxKey{}).(int64)
	return userID, ok
}
