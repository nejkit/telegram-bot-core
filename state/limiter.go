package state

import (
	"golang.org/x/time/rate"
	"sync"
)

type UserLimiter struct {
	mu     sync.Mutex
	limits map[int64]*rate.Limiter
	rate   rate.Limit
	burst  int
}

func NewUserLimiter(rateLimit rate.Limit, burst int) *UserLimiter {
	return &UserLimiter{
		mu:     sync.Mutex{},
		limits: make(map[int64]*rate.Limiter),
		rate:   rateLimit,
		burst:  burst,
	}
}

func (u *UserLimiter) Check(userID int64) bool {
	u.mu.Lock()
	defer u.mu.Unlock()

	limiter, ok := u.limits[userID]

	if !ok {
		limiter = rate.NewLimiter(u.rate, u.burst)
		u.limits[userID] = limiter
	}

	return limiter.Allow()
}
