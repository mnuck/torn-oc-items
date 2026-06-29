package sheets

import (
	"os"

	"log/slog"
)

// getRequiredEnv fetches a required environment variable or exits if not set.
func getRequiredEnv(key string) string {
	value := os.Getenv(key)
	if value == "" {
		slog.Error(key + " environment variable is required")
		os.Exit(1)
	}
	return value
}

// getEnvWithDefault fetches an environment variable with a default fallback.
func getEnvWithDefault(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}
