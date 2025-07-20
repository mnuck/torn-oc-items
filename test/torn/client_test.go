package torn_test

import (
	"context"
	"os"
	"testing"

	"torn_oc_items/internal/torn"
)

func TestGetItem(t *testing.T) {
	apiKey := os.Getenv("TORN_API_KEY")
	if apiKey == "" {
		t.Fatalf("TORN_API_KEY environment variable not set")
	}

	factionApiKey := os.Getenv("TORN_FACTION_API_KEY")
	client := torn.NewClient(apiKey, factionApiKey)

	ctx := context.Background()
	item, err := client.GetItem(ctx, "1258")
	if err != nil {
		t.Fatalf("Failed to get item: %v", err)
	}

	if item.Name != "Binoculars" {
		t.Errorf("Expected item name 'Binoculars', got '%s'", item.Name)
	}
}
