package sheets

import (
	"os"

	"github.com/rs/zerolog/log"
)

// getRequiredEnv fetches a required environment variable or exits if not set.
func getRequiredEnv(key string) string {
	value := os.Getenv(key)
	if value == "" {
		log.Fatal().Msgf("%s environment variable is required", key)
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
