package retry

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand"
	"time"
)

type Config struct {
	MaxRetries int
	BaseDelay  time.Duration
	MaxDelay   time.Duration
	Timeout    time.Duration
}

func WithRetry[T any](ctx context.Context, config Config, operation func(context.Context) (T, error)) (T, error) {
	var zero T
	for attempt := 0; attempt <= config.MaxRetries; attempt++ {
		select {
		case <-ctx.Done():
			slog.Debug("Parent context canceled, aborting retry", "error", ctx.Err())
			return zero, ctx.Err()
		default:
		}

		// Create timeout context for this operation
		opCtx, cancel := context.WithTimeout(ctx, config.Timeout)

		result, err := operation(opCtx)
		cancel()

		if err == nil {
			return result, nil
		}

		slog.Debug("Operation failed",
			"error", err,
			"attempt", attempt+1,
			"max_retries", config.MaxRetries,
		)

		if attempt < config.MaxRetries {
			delay := calculateBackoffDelay(attempt, config.BaseDelay, config.MaxDelay)
			slog.Debug("Retrying after delay",
				"delay", delay,
				"next_attempt", attempt+2,
			)

			select {
			case <-ctx.Done():
				return zero, ctx.Err()
			case <-time.After(delay):
				continue
			}
		}
		return zero, fmt.Errorf("operation failed after %d attempts: %w", config.MaxRetries+1, err)
	}
	return zero, fmt.Errorf("unexpected: exceeded retry loop")
}

func calculateBackoffDelay(attempt int, baseDelay, maxDelay time.Duration) time.Duration {
	// Cap attempt at 30 to prevent overflow (2^30 is safe for int)
	safeAttempt := min(attempt, 30)
	multiplier := 1 << safeAttempt
	delay := time.Duration(multiplier) * baseDelay

	if delay > maxDelay {
		delay = maxDelay
	}

	// Add jitter to prevent thundering herd - random between 0.5x and 1.5x
	jitter := 0.5 + rand.Float64()
	delay = time.Duration(float64(delay) * jitter)

	// Ensure we don't exceed maxDelay after jitter
	if delay > maxDelay {
		delay = maxDelay
	}

	return delay
}
