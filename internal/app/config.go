package app

import (
	"context"
	"log/slog"
	"os"
	"strconv"
	"time"

	"torn_oc_items/internal/env"
	"torn_oc_items/internal/log"
	"torn_oc_items/internal/notifications"
	"torn_oc_items/internal/sheets"
	"torn_oc_items/internal/torn"
)

// SetupEnvironment loads .env file and configures logging.
func SetupEnvironment() {
	// Load .env file if it exists
	err := env.Load(".env")

	// Configure logging
	log.Setup()

	// wait until now to report on the .env file so we have the chance to set up logging first
	if err == nil {
		slog.Debug("Loaded environment variables from .env file.")
	} else {
		slog.Debug("No .env file found or error loading .env file; proceeding with existing environment variables.")
	}
}

// GetRequiredEnv fetches a required environment variable or exits if not set.
func GetRequiredEnv(key string) string {
	value := os.Getenv(key)
	if value == "" {
		slog.Error(key+" environment variable is required.")
		os.Exit(1)
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
	slog.Debug("Initializing clients")
	apiKey := GetRequiredEnv("TORN_API_KEY")
	factionApiKey := GetRequiredEnv("TORN_FACTION_API_KEY")
	credsFile := "credentials.json"

	tornClient := torn.NewClient(apiKey, factionApiKey)
	sheetsClient, err := sheets.NewClient(ctx, credsFile)
	if err != nil {
		slog.Error("Failed to create sheets client", "error", err)
		os.Exit(1)
	}

	slog.Debug("Clients initialized successfully")
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

	slog.Debug("Initializing notification client",
		"enabled", enabled,
		"base_url", baseURL,
		"topic", topic,
		"batch_mode", batchMode,
		"priority", priority,
		"max_retries", maxRetries,
		"base_delay", baseDelay,
		"max_delay", maxDelay,
	)

	client := notifications.NewClient(baseURL, topic, enabled, batchMode, priority, maxRetries, baseDelay, maxDelay)

	if enabled {
		mode := "batch"
		if !batchMode {
			mode = "individual"
		}
		slog.Info("Notifications enabled",
			"topic", topic,
			"mode", mode,
			"priority", priority,
			"max_retries", maxRetries,
		)
	} else {
		slog.Debug("Notifications disabled")
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

	slog.Warn("Invalid integer value, using default",
		"key", key,
		"value", str,
		"default", defaultValue,
	)

	return defaultValue
}
