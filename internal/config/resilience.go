package config

import (
	"time"

	"torn_oc_items/internal/retry"
)

type ResilienceConfig struct {
	ProcessLoop retry.Config
	APIRequest  retry.Config
	HTTPTimeout time.Duration
}

var DefaultResilienceConfig = ResilienceConfig{
	ProcessLoop: retry.Config{
		MaxRetries: 3,
		BaseDelay:  5 * time.Second,
		MaxDelay:   60 * time.Second,
	},
	APIRequest: retry.Config{
		MaxRetries: 3,
		BaseDelay:  1 * time.Second,
		MaxDelay:   30 * time.Second,
	},
	HTTPTimeout: 10 * time.Second,
}
