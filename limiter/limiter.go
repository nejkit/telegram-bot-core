package limiter

import (
	"context"
	"github.com/sirupsen/logrus"
	"golang.org/x/time/rate"
	"sync"
	"time"
)

type UserLimiter struct {
	mu        sync.Mutex
	limits    map[int64]*rate.Limiter
	rate      rate.Limit
	burst     int
	isEnabled bool
}

func NewUserLimiter(rateLimit rate.Limit, burst int) *UserLimiter {
	if rateLimit == -1 {
		return &UserLimiter{
			isEnabled: false,
		}
	}

	return &UserLimiter{
		mu:        sync.Mutex{},
		limits:    make(map[int64]*rate.Limiter),
		rate:      rateLimit,
		burst:     burst,
		isEnabled: true,
	}
}

func (u *UserLimiter) Run(ctx context.Context) {
	ticker := time.NewTicker(time.Minute * 30)
	for {
		select {
		case <-ctx.Done():
			return

		case <-ticker.C:
			u.cleanup()
		}
	}
}

func (u *UserLimiter) cleanup() {
	u.mu.Lock()
	defer u.mu.Unlock()

	for chatID, limit := range u.limits {
		if limit.Tokens() == float64(u.rate) {
			delete(u.limits, chatID)
		}
	}
}

func (u *UserLimiter) Wait(ctx context.Context, userID int64) {
	if !u.isEnabled {
		return
	}

	u.mu.Lock()
	defer u.mu.Unlock()

	limiter, ok := u.limits[userID]
	if !ok {
		limiter = rate.NewLimiter(u.rate, u.burst)
		u.limits[userID] = limiter
	}

	if err := limiter.Wait(ctx); err != nil {
		logrus.WithError(err).Error("error wait limiter")
	}

	return
}

func (u *UserLimiter) Check(userID int64) bool {
	if !u.isEnabled {
		return true
	}

	u.mu.Lock()
	defer u.mu.Unlock()

	limiter, ok := u.limits[userID]

	if !ok {
		limiter = rate.NewLimiter(u.rate, u.burst)
		u.limits[userID] = limiter
	}

	return limiter.Allow()
}
