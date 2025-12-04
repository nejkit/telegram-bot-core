package state

import (
	"golang.org/x/time/rate"
	"sync"
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
