package main

import (
	"context"
	"fmt"
	"time"

	"torn_oc_items/internal/sheets"
	"torn_oc_items/internal/torn"

	"github.com/rs/zerolog/log"
)

func main() {
	setupEnvironment()

	ctx := context.Background()
	tornClient, sheetsClient := initializeClients(ctx)

	log.Info().Msg("Starting Torn OC Items monitor. Running immediately and then every minute...")

	runProcessLoop(ctx, tornClient, sheetsClient)

	// Then start the ticker
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		runProcessLoop(ctx, tornClient, sheetsClient)
	}
}

func initializeClients(ctx context.Context) (*torn.Client, *sheets.Client) {
	apiKey := getRequiredEnv("TORN_API_KEY")
	credsFile := getRequiredEnv("SHEETS_CREDENTIALS")

	tornClient := torn.NewClient(apiKey)
	sheetsClient, err := sheets.NewClient(ctx, credsFile)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to create sheets client")
	}

	return tornClient, sheetsClient
}

func getUnavailableItems(ctx context.Context, tornClient *torn.Client) []torn.UnavailableItem {
	unavailableItems, err := tornClient.GetUnavailableItems(ctx)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to get unavailable items")
	}
	return unavailableItems
}

func readExistingSheetData(ctx context.Context, sheetsClient *sheets.Client) [][]interface{} {
	spreadsheetID := getRequiredEnv("SPREADSHEET_ID")
	existingData, err := sheetsClient.ReadSheet(ctx, spreadsheetID, "Test Sheet!A1:Z1000")
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to read existing sheet data")
	}
	return existingData
}

func buildExistingMap(existingData [][]interface{}) map[string]bool {
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
	return existing
}

func runProcessLoop(ctx context.Context, tornClient *torn.Client, sheetsClient *sheets.Client) {
	unavailableItems := getUnavailableItems(ctx, tornClient)

	if len(unavailableItems) == 0 {
		log.Info().Msg("No unavailable items found in planning crimes.")
		return
	}

	existingData := readExistingSheetData(ctx, sheetsClient)
	existing := buildExistingMap(existingData)

	rows := processUnavailableItems(ctx, tornClient, unavailableItems, existing)
	if len(rows) == 0 {
		log.Info().Msg("No new items to add to sheet.")
		return
	}

	updateSheet(ctx, sheetsClient, rows, len(unavailableItems))
}

func processUnavailableItems(ctx context.Context, tornClient *torn.Client, unavailableItems []torn.UnavailableItem, existing map[string]bool) [][]interface{} {
	var rows [][]interface{}

	for _, itm := range unavailableItems {
		crimeURL := fmt.Sprintf("http://www.torn.com/factions.php?step=your#/tab=crimes&crimeId=%d", itm.CrimeID)

		itemName := getItemDetails(ctx, tornClient, itm.ItemID)
		userName := getUserDetails(ctx, tornClient, itm.UserID)

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

	return rows
}

func getItemDetails(ctx context.Context, tornClient *torn.Client, itemID int) string {
	itemDetails, err := tornClient.GetItem(ctx, fmt.Sprintf("%d", itemID))
	if err == nil {
		return itemDetails.Name
	}
	log.Warn().Err(err).Int("item_id", itemID).Msg("Failed to get item details")
	return fmt.Sprintf("Item ID: %d", itemID)
}

func getUserDetails(ctx context.Context, tornClient *torn.Client, userID int) string {
	userDetails, err := tornClient.GetUser(ctx, fmt.Sprintf("%d", userID))
	if err == nil {
		return userDetails.Name
	}
	log.Warn().Err(err).Int("user_id", userID).Msg("Failed to get user details")
	return fmt.Sprintf("User ID: %d", userID)
}

func updateSheet(ctx context.Context, sheetsClient *sheets.Client, rows [][]interface{}, totalItems int) {
	spreadsheetID := getRequiredEnv("SPREADSHEET_ID")
	sheetRange := getEnvWithDefault("SPREADSHEET_RANGE", "Test Sheet!A1")

	if err := sheetsClient.AppendRows(ctx, spreadsheetID, sheetRange, rows); err != nil {
		log.Fatal().Err(err).Msg("Failed to append rows to sheet")
	}

	skipped := totalItems - len(rows)
	log.Info().
		Int("added", len(rows)).
		Int("skipped", skipped).
		Msg("Sheet update complete")
}
