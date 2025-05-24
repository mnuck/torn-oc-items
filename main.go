package main

import (
	"context"
	"fmt"

	"torn_oc_items/internal/sheets"
	"torn_oc_items/internal/torn"

	"github.com/rs/zerolog/log"
)

func main() {
	setupEnvironment()

	apiKey := getRequiredEnv("TORN_API_KEY")
	credsFile := getRequiredEnv("SHEETS_CREDENTIALS")
	spreadsheetID := getRequiredEnv("SPREADSHEET_ID")
	sheetRange := getEnvWithDefault("SPREADSHEET_RANGE", "Test Sheet!A1")

	ctx := context.Background()

	// Create clients
	tornClient := torn.NewClient(apiKey)
	sheetsClient, err := sheets.NewClient(ctx, credsFile)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to create sheets client")
	}

	unavailableItems, err := tornClient.GetUnavailableItems(ctx)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to get unavailable items")
	}

	if len(unavailableItems) == 0 {
		log.Info().Msg("No unavailable items found in planning crimes.")
		return
	}

	// Read existing sheet data to check for duplicates
	existingData, err := sheetsClient.ReadSheet(ctx, spreadsheetID, "Test Sheet!A1:Z1000")
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to read existing sheet data")
	}

	// Create a map to track existing crime+user combinations
	existing := make(map[string]bool)
	for _, row := range existingData {
		if len(row) >= 6 {
			crimeURL := ""
			userName := ""
			if len(row) > 2 && row[2] != nil {
				crimeURL = fmt.Sprintf("%v", row[2])
			}
			if len(row) > 5 && row[5] != nil {
				userName = fmt.Sprintf("%v", row[5])
			}
			if crimeURL != "" && userName != "" {
				key := fmt.Sprintf("%s|%s", crimeURL, userName)
				existing[key] = true
			}
		}
	}

	log.Info().Int("count", len(unavailableItems)).Msg("Found unavailable items")

	var rows [][]interface{}

	for _, itm := range unavailableItems {
		crimeURL := fmt.Sprintf("http://www.torn.com/factions.php?step=your#/tab=crimes&crimeId=%d", itm.CrimeID)

		// Fetch item details
		itemDetails, err := tornClient.GetItem(ctx, fmt.Sprintf("%d", itm.ItemID))
		itemName := fmt.Sprintf("Item ID: %d", itm.ItemID)
		if err == nil {
			itemName = itemDetails.Name
		} else {
			log.Warn().Err(err).Int("item_id", itm.ItemID).Msg("Failed to get item details")
		}

		// Fetch user details
		userDetails, err := tornClient.GetUser(ctx, fmt.Sprintf("%d", itm.UserID))
		userName := fmt.Sprintf("User ID: %d", itm.UserID)
		if err == nil {
			userName = userDetails.Name
		} else {
			log.Warn().Err(err).Int("user_id", itm.UserID).Msg("Failed to get user details")
		}

		log.Info().
			Int("crime_id", itm.CrimeID).
			Str("item", itemName).
			Str("user", userName).
			Str("crime_url", crimeURL).
			Msg("Unavailable item")

		key := fmt.Sprintf("%s|%s", crimeURL, userName)
		if !existing[key] {
			rows = append(rows, []interface{}{"", "", crimeURL, "", itemName, userName})
		} else {
			log.Debug().Str("key", key).Msg("Skipping duplicate entry")
		}
	}

	// Append rows to Google Sheet
	if len(rows) > 0 {
		if err := sheetsClient.AppendRows(ctx, spreadsheetID, sheetRange, rows); err != nil {
			log.Fatal().Err(err).Msg("Failed to append rows to sheet")
		}
	}

	skipped := len(unavailableItems) - len(rows)
	log.Info().
		Int("added", len(rows)).
		Int("skipped", skipped).
		Msg("Sheet update complete")
}
