package storage

import (
	"context"
	"errors"
	"fmt"
	"github.com/redis/go-redis/v9"
)

func (s *UserActionStorage[T]) getUserActionsKey(userID int64) string {
	return fmt.Sprintf("%s:user:action:%d", s.botInstancePrefix, userID)
}

type UserAction interface {
	~int
}

type UserActionStorage[T UserAction] struct {
	botInstancePrefix string
	client            *redis.Client
}

func NewUserActionStorage[T UserAction](
	botInstancePrefix string,
	client *redis.Client,
) *UserActionStorage[T] {
	return &UserActionStorage[T]{botInstancePrefix: botInstancePrefix, client: client}
}

func (s *UserActionStorage[T]) SaveAction(ctx context.Context, userID int64, action T) error {
	if action == 0 {
		return s.client.Del(ctx, s.getUserActionsKey(userID)).Err()
	}

	return s.client.Set(ctx, s.getUserActionsKey(userID), int(action), 0).Err()
}

func (s *UserActionStorage[T]) GetAction(ctx context.Context, userID int64) (T, error) {
	action, err := s.client.Get(ctx, s.getUserActionsKey(userID)).Int()

	if errors.Is(err, redis.Nil) {
		return 0, nil
	}

	if err != nil {
		return T(0), err
	}

	return T(action), err
}

func (s *UserActionStorage[T]) SaveActionWithRollback(ctx context.Context, userID int64, action T) (func(err error) error, error) {
	rollbackToAction, err := s.GetAction(ctx, userID)

	if err != nil {
		return nil, err
	}

	rollbackFunc := func(err error) error {
		if err != nil {
			return s.SaveAction(ctx, userID, rollbackToAction)
		}
		return nil
	}

	err = s.SaveAction(ctx, userID, action)

	if err != nil {
		return nil, rollbackFunc(err)
	}

	return rollbackFunc, nil
}
