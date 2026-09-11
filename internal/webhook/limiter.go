package webhook

import (
	"sync"
	"time"
)

// limiter is a token bucket: burst tokens, refilled at burst per window.
type limiter struct {
	mu       sync.Mutex
	tokens   float64
	burst    float64
	perSec   float64
	lastFill time.Time
}

// newLimiter allows burst requests immediately and refills a full burst over
// each window.
func newLimiter(burst int, window time.Duration) *limiter {
	return &limiter{
		tokens:   float64(burst),
		burst:    float64(burst),
		perSec:   float64(burst) / window.Seconds(),
		lastFill: time.Now(),
	}
}

// allow consumes a token, reporting whether one was available.
func (l *limiter) allow() bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	l.tokens = min(l.burst, l.tokens+now.Sub(l.lastFill).Seconds()*l.perSec)
	l.lastFill = now

	if l.tokens < 1 {
		return false
	}
	l.tokens--
	return true
}
