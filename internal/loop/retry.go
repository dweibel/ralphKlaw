package loop

import (
	"context"
	"errors"
	"time"
)

var (
	ErrMaxIterationsReached  = errors.New("max iterations reached")
	ErrMaxFixAttemptsReached = errors.New("max fix attempts reached")
)

// retryWithBackoffDelays retries fn up to maxAttempts times with exponential backoff.
// baseDelay is the initial delay, capped at capDelay. Respects context cancellation.
func retryWithBackoffDelays(ctx context.Context, maxAttempts int, fn func() error, baseDelay, capDelay time.Duration) error {
	delay := baseDelay

	for attempt := 0; attempt < maxAttempts; attempt++ {
		if err := fn(); err != nil {
			if attempt == maxAttempts-1 {
				return err
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}
			delay *= 2
			if delay > capDelay {
				delay = capDelay
			}
			continue
		}
		return nil
	}
	return nil
}

// RetryWithBackoff retries fn up to maxAttempts times with exponential backoff.
// Initial delay: 1s, doubles each attempt, capped at 30s.
// Respects context cancellation. Returns the last error after maxAttempts exhausted.
func RetryWithBackoff(ctx context.Context, maxAttempts int, fn func() error) error {
	return retryWithBackoffDelays(ctx, maxAttempts, fn, 1*time.Second, 30*time.Second)
}
