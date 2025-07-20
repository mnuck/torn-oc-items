package app

import (
	"context"
	"os"
	"strconv"
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
	batchMode := GetEnvWithDefault("NTFY_BATCH_MODE", "true") == "true"
	priority := GetEnvWithDefault("NTFY_PRIORITY", "default")
	
	// Parse retry configuration
	maxRetries := parseIntWithDefault("NTFY_MAX_RETRIES", 3)
	baseDelayMs := parseIntWithDefault("NTFY_BASE_DELAY_MS", 1000)
	maxDelayMs := parseIntWithDefault("NTFY_MAX_DELAY_MS", 30000)
	
	baseDelay := time.Duration(baseDelayMs) * time.Millisecond
	maxDelay := time.Duration(maxDelayMs) * time.Millisecond

	log.Debug().
		Bool("enabled", enabled).
		Str("base_url", baseURL).
		Str("topic", topic).
		Bool("batch_mode", batchMode).
		Str("priority", priority).
		Int("max_retries", maxRetries).
		Dur("base_delay", baseDelay).
		Dur("max_delay", maxDelay).
		Msg("Initializing notification client")

	client := notifications.NewClient(baseURL, topic, enabled, batchMode, priority, maxRetries, baseDelay, maxDelay)

	if enabled {
		mode := "batch"
		if !batchMode {
			mode = "individual"
		}
		log.Info().
			Str("topic", topic).
			Str("mode", mode).
			Str("priority", priority).
			Int("max_retries", maxRetries).
			Msg("Notifications enabled")
	} else {
		log.Debug().Msg("Notifications disabled")
	}

	return client
}

// parseIntWithDefault parses an environment variable as int with fallback
func parseIntWithDefault(key string, defaultValue int) int {
	str := os.Getenv(key)
	if str == "" {
		return defaultValue
	}
	
	if val, err := strconv.Atoi(str); err == nil {
		return val
	}
	
	log.Warn().
		Str("key", key).
		Str("value", str).
		Int("default", defaultValue).
		Msg("Invalid integer value, using default")
	
	return defaultValue
}
