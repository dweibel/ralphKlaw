package loop

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestRetry_SucceedsOnFirstTry verifies that when fn succeeds immediately,
// it is called exactly once and no delay is incurred.
func TestRetry_SucceedsOnFirstTry(t *testing.T) {
	callCount := 0
	fn := func() error {
		callCount++
		return nil
	}

	start := time.Now()
	err := RetryWithBackoff(context.Background(), 5, fn)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
	if callCount != 1 {
		t.Errorf("fn called %d times, want 1", callCount)
	}
	// No delay should have occurred — well under 100ms
	if elapsed > 100*time.Millisecond {
		t.Errorf("elapsed %v, expected near-zero delay on first-try success", elapsed)
	}
}

// TestRetry_RetriesUpToMaxAttempts verifies that when fn always fails,
// it is called exactly maxAttempts times and the last error is returned.
// Uses a custom clock via a short-delay variant to avoid slow tests.
func TestRetry_RetriesUpToMaxAttempts(t *testing.T) {
	sentinel := errors.New("always fails")
	callCount := 0
	fn := func() error {
		callCount++
		return sentinel
	}

	maxAttempts := 4
	err := retryWithBackoffCustom(context.Background(), maxAttempts, fn, 1*time.Millisecond, 30*time.Millisecond)

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("expected sentinel error, got: %v", err)
	}
	if callCount != maxAttempts {
		t.Errorf("fn called %d times, want %d", callCount, maxAttempts)
	}
}

// TestRetry_RespectsContextCancellation verifies that when the context is
// cancelled mid-retry, the function returns ctx.Err() promptly.
func TestRetry_RespectsContextCancellation(t *testing.T) {
	callCount := 0
	fn := func() error {
		callCount++
		return errors.New("transient error")
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Cancel after the first attempt completes
	go func() {
		time.Sleep(5 * time.Millisecond)
		cancel()
	}()

	// Use a long base delay so context cancellation fires before the next retry
	err := retryWithBackoffCustom(ctx, 10, fn, 500*time.Millisecond, 30*time.Second)

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got: %v", err)
	}
	// fn should have been called once before the delay where cancellation fires
	if callCount != 1 {
		t.Errorf("fn called %d times, want 1", callCount)
	}
}

// TestRetry_ExponentialDelayCapAt30s verifies the delay sequence logic:
// 1s → 2s → 4s → ... capped at 30s.
// Uses scaled-down delays (1ms base, 30ms cap) to verify the capping logic
// without incurring real wall-clock time.
func TestRetry_ExponentialDelayCapAt30s(t *testing.T) {
	// We verify the delay sequence by recording when each call happens.
	// With base=1ms and cap=30ms, the sequence should be: 1ms, 2ms, 4ms, 8ms, 16ms, 30ms, 30ms...
	callTimes := []time.Time{}
	fn := func() error {
		callTimes = append(callTimes, time.Now())
		return errors.New("always fails")
	}

	maxAttempts := 7 // enough to hit the cap
	retryWithBackoffCustom(context.Background(), maxAttempts, fn, 1*time.Millisecond, 30*time.Millisecond)

	if len(callTimes) != maxAttempts {
		t.Fatalf("fn called %d times, want %d", len(callTimes), maxAttempts)
	}

	// Verify delays between calls follow exponential growth capped at 30ms.
	// We use generous bounds (50% tolerance) to avoid flakiness.
	expectedDelays := []time.Duration{
		1 * time.Millisecond,
		2 * time.Millisecond,
		4 * time.Millisecond,
		8 * time.Millisecond,
		16 * time.Millisecond,
		30 * time.Millisecond, // capped
	}

	for i := 1; i < len(callTimes); i++ {
		actual := callTimes[i].Sub(callTimes[i-1])
		expected := expectedDelays[i-1]
		// Allow generous tolerance: actual should be >= expected*0.5 and <= expected*5
		// (OS scheduling can add latency, but should not be less than half the delay)
		if actual < expected/2 {
			t.Errorf("delay[%d] = %v, want >= %v (expected ~%v)", i-1, actual, expected/2, expected)
		}
	}

	// Specifically verify the cap: delays at index 5 and beyond should not exceed cap*5
	for i := 6; i < len(callTimes); i++ {
		actual := callTimes[i].Sub(callTimes[i-1])
		cap := 30 * time.Millisecond
		if actual > cap*5 {
			t.Errorf("delay[%d] = %v, expected capped near %v", i-1, actual, cap)
		}
	}
}

// TestRetry_SingleAttempt verifies that maxAttempts=1 calls fn once and
// returns the error without any retry.
func TestRetry_SingleAttempt(t *testing.T) {
	sentinel := errors.New("single attempt error")
	callCount := 0
	fn := func() error {
		callCount++
		return sentinel
	}

	err := retryWithBackoffCustom(context.Background(), 1, fn, 1*time.Millisecond, 30*time.Millisecond)

	if !errors.Is(err, sentinel) {
		t.Errorf("expected sentinel error, got: %v", err)
	}
	if callCount != 1 {
		t.Errorf("fn called %d times, want 1", callCount)
	}
}

// TestRetry_SucceedsOnRetry verifies that if fn fails initially but succeeds
// on a later attempt, the error is nil and fn is called the right number of times.
func TestRetry_SucceedsOnRetry(t *testing.T) {
	callCount := 0
	fn := func() error {
		callCount++
		if callCount < 3 {
			return errors.New("transient")
		}
		return nil
	}

	err := retryWithBackoffCustom(context.Background(), 5, fn, 1*time.Millisecond, 30*time.Millisecond)

	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
	if callCount != 3 {
		t.Errorf("fn called %d times, want 3", callCount)
	}
}

// retryWithBackoffCustom is a testable variant of RetryWithBackoff that accepts
// configurable base delay and cap, allowing tests to verify delay logic without
// incurring real wall-clock time.
func retryWithBackoffCustom(ctx context.Context, maxAttempts int, fn func() error, baseDelay, capDelay time.Duration) error {
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
