package processing

import (
	"context"
	"fmt"
	"log/slog"

	"torn_oc_items/internal/resolution"
	"torn_oc_items/internal/torn"
)

// GetSuppliedItems fetches and returns supplied items from the Torn API
func GetSuppliedItems(ctx context.Context, tornClient *torn.Client) []torn.SuppliedItem {
	slog.Debug("Fetching supplied items")
	callsBefore := tornClient.GetAPICallCount()

	suppliedItems, err := tornClient.GetSuppliedItems(ctx)
	if err != nil {
		slog.Error("Failed to get supplied items, skipping this cycle", "error", err)
		return nil
	}

	callsAfter := tornClient.GetAPICallCount()
	slog.Debug("Retrieved supplied items", "count", len(suppliedItems), "api_calls", callsAfter-callsBefore)
	return suppliedItems
}

// ProcessSuppliedItems processes supplied items and returns rows to be added to the sheet
func ProcessSuppliedItems(ctx context.Context, tornClient *torn.Client, suppliedItems []torn.SuppliedItem, existing map[string]bool) [][]interface{} {
	slog.Debug("Processing supplied items", "count", len(suppliedItems))
	callsBefore := tornClient.GetAPICallCount()
	var rows [][]interface{}

	for _, itm := range suppliedItems {
		crimeURL := fmt.Sprintf("http://www.torn.com/factions.php?step=your#/tab=crimes&crimeId=%d", itm.CrimeID)

		itemName := resolution.GetItemDetails(ctx, tornClient, itm.ItemID)
		userName := resolution.GetUserDetails(ctx, tornClient, itm.UserID)

		slog.Info("Supplied item",
			"crime_id", itm.CrimeID,
			"item", itemName,
			"user", userName,
			"crime_url", crimeURL,
		)

		key := fmt.Sprintf("%s|%s|%s", crimeURL, userName, itemName)
		if !existing[key] {
			slog.Debug("Adding new item to sheet", "key", key)
			formula := "=IF(OR(INDIRECT(\"A\"&ROW())=\"Provided\",INDIRECT(\"A\"&ROW())=\"Cash Sent\"), INDIRECT(\"G\"&ROW()), 0)"
			rows = append(rows, []interface{}{"Needed", "", crimeURL, "", itemName, userName, "", formula})
		} else {
			slog.Debug("Skipping duplicate entry", "key", key)
		}
	}

	callsAfter := tornClient.GetAPICallCount()
	slog.Debug("Finished processing supplied items",
		"total_items", len(suppliedItems),
		"new_rows", len(rows),
		"api_calls", callsAfter-callsBefore,
	)

	return rows
}
