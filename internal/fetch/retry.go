package fetch

import (
	"context"
	"errors"
	"fmt"
	"time"

	"node-box/internal/logx"
)

// RetryPolicy controls how a failed GET is retried.
type RetryPolicy struct {
	// Attempts is the total number of tries, including the first.
	Attempts int
	// Base is the delay before the second attempt; it grows linearly.
	Base time.Duration
}

// DefaultRetry is used when a zero RetryPolicy is passed.
var DefaultRetry = RetryPolicy{Attempts: 3, Base: 2 * time.Second}

// GetWithRetry performs a GET, retrying transient failures.
//
// ErrNotModified and non-retryable status codes are returned immediately: a
// 404 will not become a 200 by waiting, and retrying it just delays the run.
func (c *Client) GetWithRetry(ctx context.Context, r Request, policy RetryPolicy) (*Response, error) {
	if policy.Attempts <= 0 {
		policy = DefaultRetry
	}

	var lastErr error
	for attempt := 1; attempt <= policy.Attempts; attempt++ {
		if attempt > 1 {
			delay := time.Duration(attempt-1) * policy.Base
			logx.Warnf("retry %d/%d for %s in %s (%v)", attempt, policy.Attempts, r.URL, delay, lastErr)
			if err := sleep(ctx, delay); err != nil {
				return nil, err
			}
		}

		resp, err := c.Get(ctx, r)
		if err == nil {
			return resp, nil
		}
		if !retryable(err) {
			return nil, err
		}
		lastErr = err
	}

	return nil, fmt.Errorf("GET %s failed after %d attempts: %w", r.URL, policy.Attempts, lastErr)
}

// retryable reports whether an error is worth another attempt.
func retryable(err error) bool {
	// A 304 is a successful outcome and a cancelled context means we are
	// shutting down; neither should be retried.
	if errors.Is(err, ErrNotModified) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	if errors.Is(err, ErrTooLarge) {
		return false
	}
	var se *StatusError
	if errors.As(err, &se) {
		return se.Retryable()
	}
	// Anything left is a transport-level failure: DNS, connection reset,
	// timeout. Those are the ones retrying actually helps.
	return true
}

// sleep waits for d, returning early if the context is cancelled.
func sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
