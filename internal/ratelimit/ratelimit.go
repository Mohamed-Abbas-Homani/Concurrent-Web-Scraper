// Rate limiting logic (move RateLimiter struct and logic here)
package ratelimit

import (
	"context"
	"time"

	"github.com/sirupsen/logrus"
	"golang.org/x/time/rate"
)

// RateLimiter provides rate limiting functionality
type RateLimiter struct {
	limiter *rate.Limiter
	logger  *logrus.Logger
	stats   RateLimitStats
}

// RateLimitStats holds rate limiting statistics
type RateLimitStats struct {
	TotalRequests int64         `json:"total_requests"`
	BlockedTime   time.Duration `json:"blocked_time"`
	AverageWait   time.Duration `json:"average_wait"`
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(rps float64, logger *logrus.Logger) *RateLimiter {
	burst := max(int(rps)+1, 1)

	return &RateLimiter{
		limiter: rate.NewLimiter(rate.Limit(rps), burst),
		logger:  logger,
		stats:   RateLimitStats{},
	}
}

// Wait blocks until the rate limiter allows the request
func (rl *RateLimiter) Wait(ctx context.Context) error {
	start := time.Now()

	err := rl.limiter.Wait(ctx)

	waitTime := time.Since(start)
	rl.stats.TotalRequests++
	rl.stats.BlockedTime += waitTime

	if rl.stats.TotalRequests > 0 {
		rl.stats.AverageWait = rl.stats.BlockedTime / time.Duration(rl.stats.TotalRequests)
	}

	if err != nil {
		rl.logger.WithError(err).Warn("Rate limiter wait interrupted")
		return err
	}

	if waitTime > 100*time.Millisecond {
		rl.logger.WithField("wait_time", waitTime).Debug("Rate limit applied")
	}

	return nil
}

// Allow checks if a request is allowed without blocking
func (rl *RateLimiter) Allow() bool {
	allowed := rl.limiter.Allow()
	if !allowed {
		rl.logger.Debug("Request blocked by rate limiter")
	}
	return allowed
}

// GetStats returns rate limiting statistics
func (rl *RateLimiter) GetStats() RateLimitStats {
	return rl.stats
}

// SetLimit updates the rate limit
func (rl *RateLimiter) SetLimit(rps float64) {
	rl.limiter.SetLimit(rate.Limit(rps))
	rl.logger.WithField("new_rate", rps).Info("Rate limit updated")
}
