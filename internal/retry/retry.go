package retry

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"github.com/rs/zerolog/log"
)

type Config struct {
	MaxRetries    int
	BaseDelay     time.Duration
	MaxDelay      time.Duration
	Timeout       time.Duration
	InfiniteRetry bool
}

func WithRetry[T any](ctx context.Context, config Config, operation func(context.Context) (T, error)) (T, error) {
	var zero T
	attempt := 0
	
	for {
		select {
		case <-ctx.Done():
			log.Debug().Err(ctx.Err()).Msg("Parent context canceled, aborting retry")
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

		if config.InfiniteRetry {
			log.Debug().
				Err(err).
				Int("attempt", attempt+1).
				Msg("Operation failed - infinite retry enabled")
		} else {
			log.Debug().
				Err(err).
				Int("attempt", attempt+1).
				Int("max_retries", config.MaxRetries).
				Msg("Operation failed")
		}

		// Check if we should continue retrying
		if !config.InfiniteRetry && attempt >= config.MaxRetries {
			return zero, fmt.Errorf("operation failed after %d attempts: %w", config.MaxRetries+1, err)
		}

		delay := calculateBackoffDelay(attempt, config.BaseDelay, config.MaxDelay)
		if config.InfiniteRetry {
			log.Debug().
				Dur("delay", delay).
				Int("next_attempt", attempt+2).
				Msg("Retrying after delay (infinite retry mode)")
		} else {
			log.Debug().
				Dur("delay", delay).
				Int("next_attempt", attempt+2).
				Msg("Retrying after delay")
		}

		select {
		case <-ctx.Done():
			return zero, ctx.Err()
		case <-time.After(delay):
			attempt++
			continue
		}
	}
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
