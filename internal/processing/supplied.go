package processing

import (
	"context"
	"fmt"

	"torn_oc_items/internal/config"
	"torn_oc_items/internal/resolution"
	"torn_oc_items/internal/retry"
	"torn_oc_items/internal/torn"

	"github.com/rs/zerolog/log"
)

// GetSuppliedItems fetches and returns supplied items from the Torn API
func GetSuppliedItems(ctx context.Context, tornClient *torn.Client) []torn.SuppliedItem {
	log.Debug().Msg("Fetching supplied items")
	callsBefore := tornClient.GetAPICallCount()

	suppliedItems, err := tornClient.GetSuppliedItems(ctx)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to get supplied items")
	}

	callsAfter := tornClient.GetAPICallCount()
	log.Debug().
		Int("count", len(suppliedItems)).
		Int64("api_calls", callsAfter-callsBefore).
		Msg("Retrieved supplied items")
	return suppliedItems
}

// ProcessSuppliedItems processes supplied items and returns rows to be added to the sheet
func ProcessSuppliedItems(ctx context.Context, tornClient *torn.Client, suppliedItems []torn.SuppliedItem, existing map[string]bool) [][]interface{} {
	log.Debug().Int("count", len(suppliedItems)).Msg("Processing supplied items")
	callsBefore := tornClient.GetAPICallCount()
	var rows [][]interface{}

	for _, itm := range suppliedItems {
		crimeURL := fmt.Sprintf("http://www.torn.com/factions.php?step=your#/tab=crimes&crimeId=%d", itm.CrimeID)

		itemName := resolution.GetItemDetails(ctx, tornClient, itm.ItemID)
		userName := resolution.GetUserDetails(ctx, tornClient, itm.UserID)

		log.Info().
			Int("crime_id", itm.CrimeID).
			Str("item", itemName).
			Str("user", userName).
			Str("crime_url", crimeURL).
			Msg("Supplied item")

		key := fmt.Sprintf("%s|%s|%s", crimeURL, userName, itemName)
		if !existing[key] {
			log.Debug().
				Str("key", key).
				Msg("Adding new item to sheet")
			formula := "=IF(OR(INDIRECT(\"A\"&ROW())=\"Provided\",INDIRECT(\"A\"&ROW())=\"Cash Sent\"), INDIRECT(\"G\"&ROW()), 0)"
			rows = append(rows, []interface{}{"Needed", "", crimeURL, "", itemName, userName, "", formula})
		} else {
			log.Debug().
				Str("key", key).
				Msg("Skipping duplicate entry")
		}
	}

	callsAfter := tornClient.GetAPICallCount()
	log.Debug().
		Int("total_items", len(suppliedItems)).
		Int("new_rows", len(rows)).
		Int64("api_calls", callsAfter-callsBefore).
		Msg("Finished processing supplied items")

	return rows
}

// GetSuppliedItemsInfinite fetches and returns supplied items from the Torn API with infinite retry
func GetSuppliedItemsInfinite(ctx context.Context, tornClient *torn.Client) []torn.SuppliedItem {
	log.Debug().Msg("Fetching supplied items (infinite retry)")
	callsBefore := tornClient.GetAPICallCount()

	suppliedItems, _ := retry.WithRetry(ctx, config.InfiniteResilienceConfig.APIRequest, func(ctx context.Context) ([]torn.SuppliedItem, error) {
		return tornClient.GetSuppliedItems(ctx)
	})

	callsAfter := tornClient.GetAPICallCount()
	log.Debug().
		Int("count", len(suppliedItems)).
		Int64("api_calls", callsAfter-callsBefore).
		Msg("Retrieved supplied items")
	return suppliedItems
}

// ProcessSuppliedItemsInfinite processes supplied items and returns rows to be added to the sheet with infinite retry on API calls
func ProcessSuppliedItemsInfinite(ctx context.Context, tornClient *torn.Client, suppliedItems []torn.SuppliedItem, existing map[string]bool) [][]interface{} {
	log.Debug().Int("count", len(suppliedItems)).Msg("Processing supplied items (infinite retry)")
	callsBefore := tornClient.GetAPICallCount()
	var rows [][]interface{}

	for _, itm := range suppliedItems {
		crimeURL := fmt.Sprintf("http://www.torn.com/factions.php?step=your#/tab=crimes&crimeId=%d", itm.CrimeID)

		itemName := resolution.GetItemDetailsInfinite(ctx, tornClient, itm.ItemID)
		userName := resolution.GetUserDetailsInfinite(ctx, tornClient, itm.UserID)

		log.Info().
			Int("crime_id", itm.CrimeID).
			Str("item", itemName).
			Str("user", userName).
			Str("crime_url", crimeURL).
			Msg("Supplied item")

		key := fmt.Sprintf("%s|%s|%s", crimeURL, userName, itemName)
		if !existing[key] {
			log.Debug().
				Str("key", key).
				Msg("Adding new item to sheet")
			formula := "=IF(OR(INDIRECT(\"A\"&ROW())=\"Provided\",INDIRECT(\"A\"&ROW())=\"Cash Sent\"), INDIRECT(\"G\"&ROW()), 0)"
			rows = append(rows, []interface{}{"Needed", "", crimeURL, "", itemName, userName, "", formula})
		} else {
			log.Debug().
				Str("key", key).
				Msg("Skipping duplicate entry")
		}
	}

	callsAfter := tornClient.GetAPICallCount()
	log.Debug().
		Int("total_items", len(suppliedItems)).
		Int("new_rows", len(rows)).
		Int64("api_calls", callsAfter-callsBefore).
		Msg("Finished processing supplied items")

	return rows
}
