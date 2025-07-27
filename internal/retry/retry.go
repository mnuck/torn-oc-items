package retry

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"github.com/rs/zerolog/log"
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

		log.Debug().
			Err(err).
			Int("attempt", attempt+1).
			Msg("Operation failed")

		if attempt < config.MaxRetries {
			delay := calculateBackoffDelay(attempt, config.BaseDelay, config.MaxDelay)
			log.Debug().
				Dur("delay", delay).
				Int("next_attempt", attempt+2).
				Msg("Retrying after delay")

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
