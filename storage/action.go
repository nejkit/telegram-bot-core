package storage

import (
	"context"
	"errors"
	"fmt"
	"github.com/nejkit/telegram-bot-core/domain"
	"github.com/nejkit/telegram-bot-core/wrapper"
	"github.com/redis/go-redis/v9"
)

func (s *UserActionStorage[T]) getUserActionsKey(userID int64) string {
	return fmt.Sprintf("%s:user:action:%d", s.botInstancePrefix, userID)
}

type UserAction int

type UserActionStorage[T interface{ UserAction }] struct {
	botInstancePrefix string
	client            *redis.Client
}

func NewUserActionStorage[T UserAction](
	botInstancePrefix string,
	client *redis.Client,
) *UserActionStorage[T] {
	return &UserActionStorage[T]{botInstancePrefix: botInstancePrefix, client: client}
}

func (s *UserActionStorage[T]) SaveAction(ctx context.Context, action T) error {
	userID, ok := wrapper.GetUserID(ctx)

	if !ok {
		return domain.ErrorCallerNotFilled
	}

	if action == 0 {
		return s.client.Del(ctx, s.getUserActionsKey(userID)).Err()
	}

	return s.client.Set(ctx, s.getUserActionsKey(userID), action, 0).Err()
}

func (s *UserActionStorage[T]) GetAction(ctx context.Context) (T, error) {
	userID, ok := wrapper.GetUserID(ctx)

	if !ok {
		return 0, domain.ErrorCallerNotFilled
	}

	action, err := s.client.Get(ctx, s.getUserActionsKey(userID)).Int()

	if errors.Is(err, redis.Nil) {
		return 0, nil
	}

	if err != nil {
		return T(0), err
	}

	return T(action), err
}

func (s *UserActionStorage[T]) SaveActionWithRollback(ctx context.Context, action T) (func(err error) error, error) {
	rollbackToAction, err := s.GetAction(ctx)

	if err != nil {
		return nil, err
	}

	rollbackFunc := func(err error) error {
		if err != nil {
			return s.SaveAction(ctx, rollbackToAction)
		}
		return nil
	}

	err = s.SaveAction(ctx, action)

	if err != nil {
		return nil, rollbackFunc(err)
	}

	return rollbackFunc, nil
}
