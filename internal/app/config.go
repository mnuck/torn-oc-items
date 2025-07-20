package app

import (
	"context"
	"os"
	"strings"
	"time"

	"torn_oc_items/internal/notifications"
	"torn_oc_items/internal/sheets"
	"torn_oc_items/internal/torn"

	"github.com/joho/godotenv"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// SetupEnvironment loads .env file and configures zerolog output and log level.
func SetupEnvironment() {
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

// GetRequiredEnv fetches a required environment variable or exits if not set.
func GetRequiredEnv(key string) string {
	value := os.Getenv(key)
	if value == "" {
		log.Fatal().Msgf("%s environment variable is required", key)
	}
	return value
}

// GetEnvWithDefault fetches an environment variable with a default fallback.
func GetEnvWithDefault(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}

// InitializeClients creates and returns the Torn API client and Google Sheets client
func InitializeClients(ctx context.Context) (*torn.Client, *sheets.Client) {
	log.Debug().Msg("Initializing clients")
	apiKey := GetRequiredEnv("TORN_API_KEY")
	factionApiKey := GetRequiredEnv("TORN_FACTION_API_KEY")
	credsFile := "credentials.json"

	tornClient := torn.NewClient(apiKey, factionApiKey)
	sheetsClient, err := sheets.NewClient(ctx, credsFile)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to create sheets client")
	}

	log.Debug().Msg("Clients initialized successfully")
	return tornClient, sheetsClient
}

// InitializeNotificationClient creates and returns the notification client
func InitializeNotificationClient() *notifications.Client {
	enabled := GetEnvWithDefault("NTFY_ENABLED", "false") == "true"
	baseURL := GetEnvWithDefault("NTFY_URL", "https://ntfy.sh")
	topic := GetEnvWithDefault("NTFY_TOPIC", "torn-oc-items")

	log.Debug().
		Bool("enabled", enabled).
		Str("base_url", baseURL).
		Str("topic", topic).
		Msg("Initializing notification client")

	client := notifications.NewClient(baseURL, topic, enabled)

	if enabled {
		log.Info().Str("topic", topic).Msg("Notifications enabled")
	} else {
		log.Debug().Msg("Notifications disabled")
	}

	return client
}
