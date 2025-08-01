package providers

import (
	"context"
	"os"
	"strings"

	"torn_oc_items/internal/config"
	"torn_oc_items/internal/retry"
	"torn_oc_items/internal/torn"

	"github.com/rs/zerolog/log"
)

type Provider struct {
	Name   string
	Client *torn.Client
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
// returns a combined LogResponse containing entries tagged by provider.
func AggregateLogs(ctx context.Context, provs []Provider) *torn.LogResponse {
	combined := &torn.LogResponse{Log: make(map[string]torn.LogEntry)}
	for _, p := range provs {
		resp, err := p.Client.GetItemSendLogs(ctx)
		if err != nil {
			log.Warn().Err(err).Str("provider", p.Name).Msg("Failed to fetch logs for provider")
			continue
		}
		for id, entry := range resp.Log {
			// ensure uniqueness with provider prefix
			combined.Log[p.Name+"|"+id] = entry
		}
	}
	log.Debug().Int("combined_log_entries", len(combined.Log)).Msg("Aggregated logs from all providers")
	return combined
}

// LoadProvidersWithRetry reads PROVIDER_KEYS from the environment with infinite retry on API calls
func LoadProvidersWithRetry(ctx context.Context) []Provider {
	keys := strings.Split(os.Getenv("PROVIDER_KEYS"), ",")
	var providers []Provider
	for _, raw := range keys {
		key := strings.TrimSpace(raw)
		if key == "" {
			continue
		}

		client := torn.NewClient(key, "") // empty faction key – not needed for WhoAmI or logs
		name, _ := retry.WithRetry(ctx, config.InfiniteResilienceConfig.APIRequest, func(ctx context.Context) (string, error) {
			return client.WhoAmI(ctx)
		})
		
		if name == "" {
			log.Warn().Str("key", key).Msg("Failed to resolve provider key after infinite retry; skipping")
			continue
		}
		
		providers = append(providers, Provider{Name: name, Client: client})
		log.Info().Str("provider", name).Msg("Loaded provider API key")
	}
	return providers
}

// AggregateLogsInfinite fetches item-send logs for the last 48h from all providers with infinite retry
func AggregateLogsInfinite(ctx context.Context, provs []Provider) *torn.LogResponse {
	combined := &torn.LogResponse{Log: make(map[string]torn.LogEntry)}
	for _, p := range provs {
		resp, _ := retry.WithRetry(ctx, config.InfiniteResilienceConfig.APIRequest, func(ctx context.Context) (*torn.LogResponse, error) {
			return p.Client.GetItemSendLogs(ctx)
		})
		
		for id, entry := range resp.Log {
			// ensure uniqueness with provider prefix
			combined.Log[p.Name+"|"+id] = entry
		}
	}
	log.Debug().Int("combined_log_entries", len(combined.Log)).Msg("Aggregated logs from all providers")
	return combined
}
