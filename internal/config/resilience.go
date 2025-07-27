package config

import (
	"time"

	"torn_oc_items/internal/retry"
)

type ResilienceConfig struct {
	ProcessLoop retry.Config
	APIRequest  retry.Config
	SheetRead   retry.Config
}

var DefaultResilienceConfig = ResilienceConfig{
	ProcessLoop: retry.Config{
		MaxRetries: 3,
		BaseDelay:  5 * time.Second,
		MaxDelay:   60 * time.Second,
		Timeout:    30 * time.Second,
	},
	APIRequest: retry.Config{
		MaxRetries: 3,
		BaseDelay:  1 * time.Second,
		MaxDelay:   30 * time.Second,
		Timeout:    10 * time.Second,
	},
	SheetRead: retry.Config{
		MaxRetries: 3,
		BaseDelay:  2 * time.Second,
		MaxDelay:   30 * time.Second,
		Timeout:    15 * time.Second,
	},
}
