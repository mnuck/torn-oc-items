package providers

import (
	"context"
	"os"
	"strings"

	"torn_oc_items/internal/torn"

	"github.com/rs/zerolog/log"
)

type Provider struct {
	Name   string
	Client *torn.Client
}

// ProviderLogEntry pairs a log entry with the provider name that fetched it.
type ProviderLogEntry struct {
	ProviderName string
	Entry        torn.LogEntry
}

// LoadProviders reads PROVIDER_KEYS from the environment (comma-separated list of Torn API keys),
// resolves each key to a player name via WhoAmI, and returns a slice of Provider instances.
// Invalid keys are skipped with a warning.
func LoadProviders(ctx context.Context) []Provider {
	keys := strings.Split(os.Getenv("PROVIDER_KEYS"), ",")
	var providers []Provider
	for _, raw := range keys {
		key := strings.TrimSpace(raw)
		if key == "" {
			continue
		}

		client := torn.NewClient(key, "") // empty faction key – not needed for WhoAmI or logs
		name, err := client.WhoAmI(ctx)
		if err != nil {
			log.Warn().Err(err).Msg("Failed to resolve provider key; skipping")
			continue
		}
		providers = append(providers, Provider{Name: name, Client: client})
		log.Info().Str("provider", name).Msg("Loaded provider API key")
	}
	return providers
}

// AggregateLogs fetches item-send logs for the last 48h from all providers and
// returns a slice of ProviderLogEntry with each entry tagged with its provider name.
func AggregateLogs(ctx context.Context, provs []Provider) []ProviderLogEntry {
	var combined []ProviderLogEntry
	for _, p := range provs {
		resp, err := p.Client.GetItemSendLogs(ctx)
		if err != nil {
			log.Warn().Err(err).Str("provider", p.Name).Msg("Failed to fetch logs for provider")
			continue
		}
		for _, entry := range resp.Log {
			combined = append(combined, ProviderLogEntry{ProviderName: p.Name, Entry: entry})
		}
	}
	log.Debug().Int("combined_log_entries", len(combined)).Msg("Aggregated logs from all providers")
	return combined
}
