package providers

import (
	"context"
	"log/slog"
	"os"
	"strings"

	"torn_oc_items/internal/torn"
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
func LoadProviders(ctx context.Context) []Provider {
	keys := strings.Split(os.Getenv("PROVIDER_KEYS"), ",")
	var providers []Provider
	for _, raw := range keys {
		key := strings.TrimSpace(raw)
		if key == "" {
			continue
		}
		client := torn.NewClient(key, "")
		name, err := client.WhoAmI(ctx)
		if err != nil {
			slog.Warn("Failed to resolve provider key; skipping", "error", err)
			continue
		}
		providers = append(providers, Provider{Name: name, Client: client})
		slog.Info("Loaded provider API key", "provider", name)
	}
	return providers
}

// AggregateLogs fetches item-send logs for the last 48h from all providers.
func AggregateLogs(ctx context.Context, provs []Provider) []ProviderLogEntry {
	var combined []ProviderLogEntry
	for _, p := range provs {
		resp, err := p.Client.GetItemSendLogs(ctx)
		if err != nil {
			slog.Warn("Failed to fetch logs for provider", "provider", p.Name, "error", err)
			continue
		}
		for _, entry := range resp.Log {
			combined = append(combined, ProviderLogEntry{ProviderName: p.Name, Entry: entry})
		}
	}
	slog.Debug("Aggregated logs from all providers", "combined_log_entries", len(combined))
	return combined
}
