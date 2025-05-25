package main

import (
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// setupEnvironment loads .env file and configures zerolog output and log level.
func setupEnvironment() {
	// Load .env file if it exists
	err := godotenv.Load()

	// Configure logging
	if os.Getenv("ENV") == "production" {
		zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
		log.Logger = log.Output(os.Stderr)
	} else {
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339})
	}

	levelStr := strings.ToLower(os.Getenv("LOGLEVEL"))
	switch levelStr {
	case "debug":
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	case "info":
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	case "warn", "warning":
		zerolog.SetGlobalLevel(zerolog.WarnLevel)
	case "error":
		zerolog.SetGlobalLevel(zerolog.ErrorLevel)
	case "fatal":
		zerolog.SetGlobalLevel(zerolog.FatalLevel)
	case "panic":
		zerolog.SetGlobalLevel(zerolog.PanicLevel)
	case "disabled":
		zerolog.SetGlobalLevel(zerolog.Disabled)
	case "":
		// Default based on environment
		if os.Getenv("ENV") == "production" {
			zerolog.SetGlobalLevel(zerolog.WarnLevel)
		} else {
			zerolog.SetGlobalLevel(zerolog.InfoLevel)
		}
	default:
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
		log.Warn().Msgf("Unknown LOGLEVEL '%s', defaulting to info.", levelStr)
	}

	// wait until now to report on the .env file so we have the chance to set up logging first
	if err == nil {
		log.Debug().Msg("Loaded environment variables from .env file.")
	} else {
		log.Debug().Msg("No .env file found or error loading .env file; proceeding with existing environment variables.")
	}
}

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
