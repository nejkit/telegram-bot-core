package storage

import (
	"context"
	"errors"
	"fmt"
	"github.com/dgraph-io/ristretto"
	"github.com/nejkit/telegram-bot-core/domain"
	"github.com/redis/go-redis/v9"
	"time"
)

type RedisInvitesStorage struct {
	botInstancePrefix string
	client            *redis.Client
}

func NewRedisInvitesStorage(
	botInstancePrefix string,
	client *redis.Client,
) *RedisInvitesStorage {
	return &RedisInvitesStorage{botInstancePrefix: botInstancePrefix, client: client}
}

func (s *RedisInvitesStorage) getInvitesKey(secret string) string {
	return fmt.Sprintf("%s:user:invite:%s", s.botInstancePrefix, secret)
}

func (s *RedisInvitesStorage) SaveInvite(ctx context.Context, deepLinkSecret string, fromUserID int64, expiration time.Duration) error {
	return s.client.SetNX(ctx, s.getInvitesKey(deepLinkSecret), fromUserID, expiration).Err()
}

func (s *RedisInvitesStorage) GetInvite(ctx context.Context, deepLinkSecret string) (int64, error) {
	userID, err := s.client.Get(ctx, s.getInvitesKey(deepLinkSecret)).Int64()

	if errors.Is(err, redis.Nil) {
		return 0, domain.ErrorInviteIsExpired
	}

	if err != nil {
		return 0, err
	}

	return userID, nil
}

func (s *RedisInvitesStorage) DeleteInvite(ctx context.Context, deepLinkSecret string) error {
	return s.client.Del(ctx, s.getInvitesKey(deepLinkSecret)).Err()
}

type InMemoryInvitesStorage struct {
	client *ristretto.Cache
}

func NewInMemoryInvitesStorage(client *ristretto.Cache) *InMemoryInvitesStorage {
	return &InMemoryInvitesStorage{client: client}
}

func (i *InMemoryInvitesStorage) getInvitesKey(secret string) string {
	return fmt.Sprintf("user:invite:%s", secret)
}

func (i *InMemoryInvitesStorage) SaveInvite(_ context.Context, deepLinkSecret string, fromUserID int64, expiration time.Duration) error {
	if ok := i.client.SetWithTTL(i.getInvitesKey(deepLinkSecret), fromUserID, 0, expiration); !ok {
		return errors.New("failed to save invite")
	}

	return nil
}

func (i *InMemoryInvitesStorage) GetInvite(_ context.Context, deepLinkSecret string) (int64, error) {
	fromUserID, ok := i.client.Get(i.getInvitesKey(deepLinkSecret))

	if !ok {
		return 0, domain.ErrorInviteIsExpired
	}

	return fromUserID.(int64), nil
}

func (i *InMemoryInvitesStorage) DeleteInvite(_ context.Context, deepLinkSecret string) error {
	i.client.Del(i.getInvitesKey(deepLinkSecret))
	return nil
}
