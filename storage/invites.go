package storage

import (
	"context"
	"errors"
	"fmt"
	"github.com/nejkit/telegram-bot-core/domain"
	"github.com/redis/go-redis/v9"
	"time"
)

type InvitesStorage struct {
	botInstancePrefix string
	client            *redis.Client
}

func (s *InvitesStorage) getInvitesKey(secret string) string {
	return fmt.Sprintf("%s:user:invite:%s", s.botInstancePrefix, secret)
}

func (s *InvitesStorage) SaveInvite(ctx context.Context, deepLinkSecret string, fromUserID int64, expiration time.Duration) error {
	return s.client.SetNX(ctx, s.getInvitesKey(deepLinkSecret), fromUserID, expiration).Err()
}

func (s *InvitesStorage) GetInvite(ctx context.Context, deepLinkSecret string) (int64, error) {
	userID, err := s.client.Get(ctx, s.getInvitesKey(deepLinkSecret)).Int64()

	if errors.Is(err, redis.Nil) {
		return 0, domain.ErrorInviteIsExpired
	}

	if err != nil {
		return 0, err
	}

	return userID, nil
}

func (s *InvitesStorage) DeleteInvite(ctx context.Context, deepLinkSecret string) error {
	return s.client.Del(ctx, s.getInvitesKey(deepLinkSecret)).Err()
}
