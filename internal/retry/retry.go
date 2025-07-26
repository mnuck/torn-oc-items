package retry

import (
	"context"
	"fmt"
	"time"

	"github.com/rs/zerolog/log"
)

type Config struct {
	MaxRetries int
	BaseDelay  time.Duration
	MaxDelay   time.Duration
}

func WithRetry[T any](ctx context.Context, config Config, operation func() (T, error)) (T, error) {
	var zero T
	for attempt := 0; attempt <= config.MaxRetries; attempt++ {
		result, err := operation()
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
			time.Sleep(delay)
			continue
		}
		return zero, fmt.Errorf("operation failed after %d attempts: %w", config.MaxRetries+1, err)
	}
	return zero, fmt.Errorf("unexpected: exceeded retry loop")
}

func calculateBackoffDelay(attempt int, baseDelay, maxDelay time.Duration) time.Duration {
	return time.Duration(min(1<<attempt, int(maxDelay/baseDelay))) * baseDelay
}
