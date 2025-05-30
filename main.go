package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"torn_oc_items/internal/providers"
	"torn_oc_items/internal/sheets"
	"torn_oc_items/internal/torn"

	"github.com/rs/zerolog/log"
)

// global provider list
var providerList []providers.Provider

type SheetRow struct {
	RowIndex int
	CrimeURL string
	ItemName string
	UserName string
	Provider string // Column B
	DateTime string // Column D
}

type SheetRowUpdate struct {
	RowIndex    int
	Provider    string
	DateTime    string
	MarketValue float64
}

func main() {
	log.Debug().Msg("Starting application")
	setupEnvironment()

	ctx := context.Background()
	tornClient, sheetsClient := initializeClients(ctx)

	// Load providers
	providerList = providers.LoadProviders(ctx)

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
	log.Debug().Msg("Initializing clients")
	apiKey := getRequiredEnv("TORN_API_KEY")
	factionApiKey := getRequiredEnv("TORN_FACTION_API_KEY")
	credsFile := "credentials.json"

	tornClient := torn.NewClient(apiKey, factionApiKey)
	sheetsClient, err := sheets.NewClient(ctx, credsFile)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to create sheets client")
	}

	log.Debug().Msg("Clients initialized successfully")
	return tornClient, sheetsClient
}

func getUnavailableItems(ctx context.Context, tornClient *torn.Client) []torn.UnavailableItem {
	log.Debug().Msg("Fetching unavailable items")
	callsBefore := tornClient.GetAPICallCount()

	unavailableItems, err := tornClient.GetUnavailableItems(ctx)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to get unavailable items")
	}

	callsAfter := tornClient.GetAPICallCount()
	log.Debug().
		Int("count", len(unavailableItems)).
		Int64("api_calls", callsAfter-callsBefore).
		Msg("Retrieved unavailable items")
	return unavailableItems
}

func readExistingSheetData(ctx context.Context, sheetsClient *sheets.Client) [][]interface{} {
	log.Debug().Msg("Reading existing sheet data")
	spreadsheetID := getRequiredEnv("SPREADSHEET_ID")
	sheetRange := getEnvWithDefault("SPREADSHEET_RANGE", "Test Sheet!A1")
	// Extend range to Z1000 for reading all data
	readRange := strings.Split(sheetRange, "!")[0] + "!A1:Z1000"
	existingData, err := sheetsClient.ReadSheet(ctx, spreadsheetID, readRange)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to read existing sheet data")
	}
	log.Debug().Int("rows", len(existingData)).Msg("Retrieved existing sheet data")
	return existingData
}

func buildExistingMap(existingData [][]interface{}) map[string]bool {
	log.Debug().Msg("Building existing items map")
	existing := make(map[string]bool)
	for _, row := range existingData {
		if len(row) >= 6 {
			crimeURL := ""
			userName := ""
			itemName := ""
			if len(row) > 2 && row[2] != nil {
				crimeURL = fmt.Sprintf("%v", row[2])
			}
			if len(row) > 4 && row[4] != nil {
				itemName = fmt.Sprintf("%v", row[4])
			}
			if len(row) > 5 && row[5] != nil {
				userName = fmt.Sprintf("%v", row[5])
			}
			if crimeURL != "" && userName != "" && itemName != "" {
				key := fmt.Sprintf("%s|%s|%s", crimeURL, userName, itemName)
				existing[key] = true
			}
		}
	}
	log.Debug().Int("entries", len(existing)).Msg("Built existing items map")
	return existing
}

func runProcessLoop(ctx context.Context, tornClient *torn.Client, sheetsClient *sheets.Client) {
	log.Debug().Msg("Starting process loop")

	// Reset API call counter at the start
	tornClient.ResetAPICallCount()

	// Process new unavailable items
	unavailableItems := getUnavailableItems(ctx, tornClient)
	apiCallsAfterUnavailable := tornClient.GetAPICallCount()

	if len(unavailableItems) > 0 {
		log.Debug().Int("count", len(unavailableItems)).Msg("Processing new unavailable items")
		existingData := readExistingSheetData(ctx, sheetsClient)
		existing := buildExistingMap(existingData)

		rows := processUnavailableItems(ctx, tornClient, unavailableItems, existing)
		apiCallsAfterProcessing := tornClient.GetAPICallCount()

		if len(rows) > 0 {
			log.Debug().Int("rows", len(rows)).Msg("Updating sheet with new items")
			updateSheet(ctx, sheetsClient, rows, len(unavailableItems))
		} else {
			log.Debug().Msg("No new items to add to sheet")
		}

		log.Info().
			Int64("api_calls_processing_unavailable", apiCallsAfterProcessing-apiCallsAfterUnavailable).
			Msg("API calls for processUnavailableItems()")
	} else {
		log.Debug().Msg("No unavailable items found")
	}

	// Process provided items
	log.Debug().Msg("Starting provided items processing")
	apiCallsBeforeProvided := tornClient.GetAPICallCount()
	processProvidedItems(ctx, tornClient, sheetsClient, providerList)
	apiCallsAfterProvided := tornClient.GetAPICallCount()

	totalAPICalls := tornClient.GetAPICallCount()
	log.Debug().
		Int64("api_calls_get_unavailable", apiCallsAfterUnavailable).
		Int64("api_calls_process_provided", apiCallsAfterProvided-apiCallsBeforeProvided).
		Int64("total_api_calls_this_loop", totalAPICalls).
		Msg("API call summary for runProcessLoop()")
}

func processProvidedItems(ctx context.Context, tornClient *torn.Client, sheetsClient *sheets.Client, providerList []providers.Provider) {
	log.Debug().Msg("Starting provided items processing")

	// Get current sheet data first
	existingData := readExistingSheetData(ctx, sheetsClient)
	sheetItems := parseSheetItems(existingData)

	log.Debug().
		Int("total_rows", len(existingData)).
		Int("parsed_items", len(sheetItems)).
		Msg("Parsed sheet items")

	// Get item send logs from all providers
	log.Debug().Msg("Fetching item send logs from all providers")
	logResp := providers.AggregateLogs(ctx, providerList)

	// Find sheet rows that need provider updates
	updates := findProviderUpdates(ctx, tornClient, sheetItems, logResp)
	if len(updates) > 0 {
		log.Debug().
			Int("updates", len(updates)).
			Msg("Updating provided item rows")
		updateProvidedItemRows(ctx, sheetsClient, updates)
	} else {
		log.Debug().Msg("No provided items to update")
	}
}

type SheetItem struct {
	RowIndex    int
	CrimeURL    string
	ItemName    string
	UserName    string
	Provider    string
	HasProvider bool
}

func parseSheetItems(existingData [][]interface{}) []SheetItem {
	log.Debug().Int("rows", len(existingData)).Msg("Parsing sheet items")
	var items []SheetItem

	for i, row := range existingData {
		if len(row) < 6 {
			log.Debug().
				Int("row", i+1).
				Int("columns", len(row)).
				Msg("Skipping row with insufficient columns")
			continue
		}

		// Check if provider column (B) is blank
		provider := ""
		hasProvider := false
		if len(row) > 1 && row[1] != nil {
			provider = strings.TrimSpace(fmt.Sprintf("%v", row[1]))
			hasProvider = provider != ""
		}

		// Extract other information
		crimeURL := ""
		itemName := ""
		userName := ""

		if len(row) > 2 && row[2] != nil {
			crimeURL = fmt.Sprintf("%v", row[2])
		}
		if len(row) > 4 && row[4] != nil {
			itemName = fmt.Sprintf("%v", row[4])
		}
		if len(row) > 5 && row[5] != nil {
			userName = fmt.Sprintf("%v", row[5])
		}

		if crimeURL != "" && itemName != "" && userName != "" {
			items = append(items, SheetItem{
				RowIndex:    i + 1, // 1-indexed for sheets
				CrimeURL:    crimeURL,
				ItemName:    itemName,
				UserName:    userName,
				Provider:    provider,
				HasProvider: hasProvider,
			})
		} else {
			log.Debug().
				Int("row", i+1).
				Str("crime_url", crimeURL).
				Str("item_name", itemName).
				Str("user_name", userName).
				Msg("Skipping row with missing required fields")
		}
	}

	log.Debug().
		Int("total_rows", len(existingData)).
		Int("parsed_items", len(items)).
		Msg("Finished parsing sheet items")

	return items
}

func findProviderUpdates(ctx context.Context, tornClient *torn.Client, sheetItems []SheetItem, logResp *torn.LogResponse) []SheetRowUpdate {
	var updates []SheetRowUpdate

	log.Debug().
		Int("sheet_items", len(sheetItems)).
		Int("log_entries", len(logResp.Log)).
		Msg("Starting provider update matching")

	for combinedID, logEntry := range logResp.Log {
		// combinedID format: providerName|logID
		parts := strings.SplitN(combinedID, "|", 2)
		providerName := "Unknown"
		if len(parts) == 2 {
			providerName = parts[0]
		}

		receiverID := logEntry.Data.Receiver
		receiverName := getUserNameByID(ctx, tornClient, receiverID)
		if receiverName == "" {
			continue
		}

		for _, logItem := range logEntry.Data.Items {
			itemID := logItem.ID
			itemName := getItemNameByID(ctx, tornClient, itemID)
			if itemName == "" {
				continue
			}

			for _, sheetItem := range sheetItems {
				if !sheetItem.HasProvider &&
					matchesUser(sheetItem.UserName, receiverName, receiverID) &&
					matchesItem(sheetItem.ItemName, itemName, itemID) {

					marketValue := getItemMarketValue(ctx, tornClient, itemID)
					timestamp := time.Unix(logEntry.Timestamp, 0)
					dateTime := timestamp.Format("15:04:05 - 02/01/06")

					updates = append(updates, SheetRowUpdate{
						RowIndex:    sheetItem.RowIndex,
						Provider:    providerName,
						DateTime:    dateTime,
						MarketValue: marketValue,
					})

					log.Info().
						Int("row", sheetItem.RowIndex).
						Str("item", sheetItem.ItemName).
						Str("user", sheetItem.UserName).
						Str("provider", providerName).
						Float64("market_value", marketValue).
						Msg("Found provided item match")

					break
				}
			}
		}
	}

	log.Debug().
		Int("updates_found", len(updates)).
		Msg("Completed provider update matching")

	return updates
}

func getUserNameByID(ctx context.Context, tornClient *torn.Client, userID int) string {
	log.Debug().Int("user_id", userID).Msg("Getting user details")
	userDetails, err := tornClient.GetUser(ctx, fmt.Sprintf("%d", userID))
	if err != nil {
		log.Debug().Err(err).Int("user_id", userID).Msg("Failed to get user details for matching")
		return ""
	}
	log.Debug().
		Int("user_id", userID).
		Str("name", userDetails.Name).
		Msg("Retrieved user details")
	return userDetails.Name
}

func getItemNameByID(ctx context.Context, tornClient *torn.Client, itemID int) string {
	log.Debug().Int("item_id", itemID).Msg("Getting item details")
	itemDetails, err := tornClient.GetItem(ctx, fmt.Sprintf("%d", itemID))
	if err != nil {
		log.Debug().Err(err).Int("item_id", itemID).Msg("Failed to get item details for matching")
		return ""
	}
	log.Debug().
		Int("item_id", itemID).
		Str("name", itemDetails.Name).
		Msg("Retrieved item details")
	return itemDetails.Name
}

func matchesUser(sheetUserName, logUserName string, logUserID int) bool {
	// Direct name match
	if sheetUserName == logUserName {
		return true
	}

	// Check if sheet has fallback format "User ID: X"
	expectedFallback := fmt.Sprintf("User ID: %d", logUserID)
	return sheetUserName == expectedFallback
}

func matchesItem(sheetItemName, logItemName string, logItemID int) bool {
	// Direct name match
	if sheetItemName == logItemName {
		return true
	}

	// Check if sheet has fallback format "Item ID: X"
	expectedFallback := fmt.Sprintf("Item ID: %d", logItemID)
	return sheetItemName == expectedFallback
}

func getItemMarketValue(ctx context.Context, tornClient *torn.Client, itemID int) float64 {
	log.Debug().Int("item_id", itemID).Msg("Getting item market value")
	item, err := tornClient.GetItem(ctx, fmt.Sprintf("%d", itemID))
	if err != nil {
		log.Warn().Err(err).Int("item_id", itemID).Msg("Failed to get item market value")
		return 0
	}
	return item.MarketValue
}

func updateProvidedItemRows(ctx context.Context, sheetsClient *sheets.Client, updates []SheetRowUpdate) {
	log.Debug().
		Int("updates", len(updates)).
		Msg("Updating provided item rows")

	spreadsheetID := getRequiredEnv("SPREADSHEET_ID")
	sheetRange := getEnvWithDefault("SPREADSHEET_RANGE", "Test Sheet!A1")
	sheetName := strings.Split(sheetRange, "!")[0]

	for _, update := range updates {
		log.Debug().
			Int("row", update.RowIndex).
			Str("provider", update.Provider).
			Str("datetime", update.DateTime).
			Float64("market_value", update.MarketValue).
			Msg("Updating row")

		// Update individual cells: B (provider), D (datetime), G (market value)
		values := [][]interface{}{
			{update.Provider},
		}
		bRange := fmt.Sprintf("%s!B%d", sheetName, update.RowIndex)
		if err := sheetsClient.UpdateRange(ctx, spreadsheetID, bRange, values); err != nil {
			log.Error().Err(err).Int("row", update.RowIndex).Msg("Failed to update provider column")
			continue
		}

		values = [][]interface{}{
			{update.DateTime},
		}
		dRange := fmt.Sprintf("%s!D%d", sheetName, update.RowIndex)
		if err := sheetsClient.UpdateRange(ctx, spreadsheetID, dRange, values); err != nil {
			log.Error().Err(err).Int("row", update.RowIndex).Msg("Failed to update datetime column")
			continue
		}

		values = [][]interface{}{
			{update.MarketValue},
		}
		gRange := fmt.Sprintf("%s!G%d", sheetName, update.RowIndex)
		if err := sheetsClient.UpdateRange(ctx, spreadsheetID, gRange, values); err != nil {
			log.Error().Err(err).Int("row", update.RowIndex).Msg("Failed to update market value column")
			continue
		}

		log.Info().
			Int("row", update.RowIndex).
			Str("provider", update.Provider).
			Str("datetime", update.DateTime).
			Float64("market_value", update.MarketValue).
			Msg("Updated provided item row")
	}

	log.Debug().
		Int("updates", len(updates)).
		Msg("Finished updating provided item rows")
}

func processUnavailableItems(ctx context.Context, tornClient *torn.Client, unavailableItems []torn.UnavailableItem, existing map[string]bool) [][]interface{} {
	log.Debug().Int("count", len(unavailableItems)).Msg("Processing unavailable items")
	callsBefore := tornClient.GetAPICallCount()
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

		key := fmt.Sprintf("%s|%s|%s", crimeURL, userName, itemName)
		if !existing[key] {
			log.Debug().
				Str("key", key).
				Msg("Adding new item to sheet")
			rows = append(rows, []interface{}{"", "", crimeURL, "", itemName, userName})
		} else {
			log.Debug().
				Str("key", key).
				Msg("Skipping duplicate entry")
		}
	}

	callsAfter := tornClient.GetAPICallCount()
	log.Debug().
		Int("total_items", len(unavailableItems)).
		Int("new_rows", len(rows)).
		Int64("api_calls", callsAfter-callsBefore).
		Msg("Finished processing unavailable items")

	return rows
}

func getItemDetails(ctx context.Context, tornClient *torn.Client, itemID int) string {
	log.Debug().Int("item_id", itemID).Msg("Getting item details")
	itemDetails, err := tornClient.GetItem(ctx, fmt.Sprintf("%d", itemID))
	if err == nil {
		log.Debug().
			Int("item_id", itemID).
			Str("name", itemDetails.Name).
			Msg("Retrieved item details")
		return itemDetails.Name
	}
	log.Warn().Err(err).Int("item_id", itemID).Msg("Failed to get item details")
	return fmt.Sprintf("Item ID: %d", itemID)
}

func getUserDetails(ctx context.Context, tornClient *torn.Client, userID int) string {
	log.Debug().Int("user_id", userID).Msg("Getting user details")
	userDetails, err := tornClient.GetUser(ctx, fmt.Sprintf("%d", userID))
	if err == nil {
		log.Debug().
			Int("user_id", userID).
			Str("name", userDetails.Name).
			Msg("Retrieved user details")
		return userDetails.Name
	}
	log.Warn().Err(err).Int("user_id", userID).Msg("Failed to get user details")
	return fmt.Sprintf("User ID: %d", userID)
}

func updateSheet(ctx context.Context, sheetsClient *sheets.Client, rows [][]interface{}, totalItems int) {
	log.Debug().
		Int("rows", len(rows)).
		Int("total_items", totalItems).
		Msg("Updating sheet")

	spreadsheetID := getRequiredEnv("SPREADSHEET_ID")
	sheetRange := getEnvWithDefault("SPREADSHEET_RANGE", "Test Sheet!A1")

	log.Debug().Str("sheet_range", sheetRange)

	if err := sheetsClient.AppendRows(ctx, spreadsheetID, sheetRange, rows); err != nil {
		log.Fatal().Err(err).Msg("Failed to append rows to sheet")
	}

	skipped := totalItems - len(rows)
	log.Info().
		Int("added", len(rows)).
		Int("skipped", skipped).
		Msg("Sheet update complete")
}
