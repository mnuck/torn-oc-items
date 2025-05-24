package sheets_test

import (
	"context"
	"testing"

	"torn_oc_items/internal/sheets"
)

func TestNewClient(t *testing.T) {
	ctx := context.Background()
	client, err := sheets.NewClient(ctx, "../testdata/credentials.json")
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	if client == nil {
		t.Fatal("Client is nil")
	}
}

func TestReadSheet(t *testing.T) {
	ctx := context.Background()
	client, err := sheets.NewClient(ctx, "../testdata/credentials.json")
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	spreadsheetID := "1QgYUWbeT5laQiwA5qEw76K_tsKZX721vTzXU91AXg_E"
	range_ := "Test Sheet!A1:Z1000"

	values, err := client.ReadSheet(ctx, spreadsheetID, range_)
	if err != nil {
		t.Fatalf("Failed to read sheet: %v", err)
	}
	if values == nil {
		t.Fatal("Values is nil")
	}
}
