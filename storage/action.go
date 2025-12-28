package storage

import (
	"context"
	"errors"
	"fmt"
	"github.com/dgraph-io/ristretto"
	"github.com/redis/go-redis/v9"
)

func (s *RedisUserActionStorage[T]) getUserActionsKey(userID int64) string {
	return fmt.Sprintf("%s:user:action:%d", s.botInstancePrefix, userID)
}

type UserAction interface {
	~int
}

type RedisUserActionStorage[T UserAction] struct {
	botInstancePrefix string
	client            *redis.Client
}

func NewRedisUserActionStorage[T UserAction](
	botInstancePrefix string,
	client *redis.Client,
) *RedisUserActionStorage[T] {
	return &RedisUserActionStorage[T]{botInstancePrefix: botInstancePrefix, client: client}
}

func (s *RedisUserActionStorage[T]) SaveAction(ctx context.Context, userID int64, action T) error {
	if action == 0 {
		return s.client.Del(ctx, s.getUserActionsKey(userID)).Err()
	}

	return s.client.Set(ctx, s.getUserActionsKey(userID), int(action), 0).Err()
}

func (s *RedisUserActionStorage[T]) GetAction(ctx context.Context, userID int64) (T, error) {
	action, err := s.client.Get(ctx, s.getUserActionsKey(userID)).Int()

	if errors.Is(err, redis.Nil) {
		return 0, nil
	}

	if err != nil {
		return T(0), err
	}

	return T(action), err
}

func (s *RedisUserActionStorage[T]) SaveActionWithRollback(ctx context.Context, userID int64, action T) (func(err error) error, error) {
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

type InMemoryUserActionStorage[T UserAction] struct {
	client *ristretto.Cache
}

func NewInMemoryUserActionStorage[action UserAction](client *ristretto.Cache) *InMemoryUserActionStorage[action] {
	return &InMemoryUserActionStorage[action]{client: client}
}

func (i *InMemoryUserActionStorage[T]) getUserActionsKey(userID int64) string {
	return fmt.Sprintf("user:action:%d", userID)
}

func (i *InMemoryUserActionStorage[T]) SaveAction(_ context.Context, userID int64, action T) error {
	if ok := i.client.Set(i.getUserActionsKey(userID), int(action), 0); !ok {
		return errors.New("failed to save action")
	}

	return nil
}

func (i *InMemoryUserActionStorage[T]) GetAction(_ context.Context, userID int64) (action T, err error) {
	data, ok := i.client.Get(i.getUserActionsKey(userID))

	if !ok {
		return 0, errors.New("failed to get action")
	}

	return data.(T), nil
}

func (i *InMemoryUserActionStorage[T]) SaveActionWithRollback(ctx context.Context, userID int64, action T) (func(err error) error, error) {
	currentAction, _ := i.GetAction(ctx, userID)

	rollback := func(err error) error {
		if err != nil {
			return i.SaveAction(ctx, userID, currentAction)
		}

		return nil
	}

	return rollback, i.SaveAction(ctx, userID, action)
}
