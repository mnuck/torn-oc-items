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

func getSuppliedItems(ctx context.Context, tornClient *torn.Client) []torn.SuppliedItem {
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

	// Process new supplied items
	suppliedItems := getSuppliedItems(ctx, tornClient)
	apiCallsAfterSupplied := tornClient.GetAPICallCount()

	if len(suppliedItems) > 0 {
		log.Debug().Int("count", len(suppliedItems)).Msg("Processing new supplied items")
		existingData := readExistingSheetData(ctx, sheetsClient)
		existing := buildExistingMap(existingData)

		rows := processSuppliedItems(ctx, tornClient, suppliedItems, existing)
		apiCallsAfterProcessing := tornClient.GetAPICallCount()

		if len(rows) > 0 {
			log.Debug().Int("rows", len(rows)).Msg("Updating sheet with new items")
			updateSheet(ctx, sheetsClient, rows, len(suppliedItems))
		} else {
			log.Debug().Msg("No new items to add to sheet")
		}

		log.Info().
			Int64("api_calls_processing_supplied", apiCallsAfterProcessing-apiCallsAfterSupplied).
			Msg("API calls for processSuppliedItems()")
	} else {
		log.Debug().Msg("No supplied items found")
	}

	// Process provided items
	log.Debug().Msg("Starting provided items processing")
	apiCallsBeforeProvided := tornClient.GetAPICallCount()
	processProvidedItems(ctx, tornClient, sheetsClient, providerList)
	apiCallsAfterProvided := tornClient.GetAPICallCount()

	totalAPICalls := tornClient.GetAPICallCount()
	log.Debug().
		Int64("api_calls_get_supplied", apiCallsAfterSupplied).
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
		if !isValidSheetRow(row, i+1) {
			continue
		}

		sheetItem := extractSheetItemFromRow(row, i+1)
		if validateSheetItem(sheetItem, i+1) {
			items = append(items, sheetItem)
		}
	}

	log.Debug().
		Int("total_rows", len(existingData)).
		Int("parsed_items", len(items)).
		Msg("Finished parsing sheet items")

	return items
}

// isValidSheetRow checks if a row has sufficient columns
func isValidSheetRow(row []interface{}, rowNum int) bool {
	if len(row) < 6 {
		log.Debug().
			Int("row", rowNum).
			Int("columns", len(row)).
			Msg("Skipping row with insufficient columns")
		return false
	}
	return true
}

// extractSheetItemFromRow extracts all fields from a sheet row
func extractSheetItemFromRow(row []interface{}, rowIndex int) SheetItem {
	// Extract provider information
	provider := ""
	hasProvider := false
	if len(row) > 1 && row[1] != nil {
		provider = strings.TrimSpace(fmt.Sprintf("%v", row[1]))
		hasProvider = provider != ""
	}

	// Extract other fields
	crimeURL := extractStringField(row, 2)
	itemName := extractStringField(row, 4)
	userName := extractStringField(row, 5)

	return SheetItem{
		RowIndex:    rowIndex,
		CrimeURL:    crimeURL,
		ItemName:    itemName,
		UserName:    userName,
		Provider:    provider,
		HasProvider: hasProvider,
	}
}

// extractStringField safely extracts a string field from a row at the given index
func extractStringField(row []interface{}, index int) string {
	if len(row) > index && row[index] != nil {
		return fmt.Sprintf("%v", row[index])
	}
	return ""
}

// validateSheetItem checks if a sheet item has all required fields
func validateSheetItem(item SheetItem, rowNum int) bool {
	if item.CrimeURL != "" && item.ItemName != "" && item.UserName != "" {
		return true
	}

	log.Debug().
		Int("row", rowNum).
		Str("crime_url", item.CrimeURL).
		Str("item_name", item.ItemName).
		Str("user_name", item.UserName).
		Msg("Skipping row with missing required fields")
	return false
}

func findProviderUpdates(ctx context.Context, tornClient *torn.Client, sheetItems []SheetItem, logResp *torn.LogResponse) []SheetRowUpdate {
	var updates []SheetRowUpdate

	log.Debug().
		Int("sheet_items", len(sheetItems)).
		Int("log_entries", len(logResp.Log)).
		Msg("Starting provider update matching")

	for combinedID, logEntry := range logResp.Log {
		providerName := extractProviderName(combinedID)
		logEntryUpdates := processLogEntryForUpdates(ctx, tornClient, logEntry, providerName, sheetItems)
		updates = append(updates, logEntryUpdates...)
	}

	log.Debug().
		Int("updates_found", len(updates)).
		Msg("Completed provider update matching")

	return updates
}

// extractProviderName extracts the provider name from combinedID format: providerName|logID
func extractProviderName(combinedID string) string {
	parts := strings.SplitN(combinedID, "|", 2)
	if len(parts) == 2 {
		return parts[0]
	}
	return "Unknown"
}

// processLogEntryForUpdates processes a single log entry and returns any updates found
func processLogEntryForUpdates(ctx context.Context, tornClient *torn.Client, logEntry torn.LogEntry, providerName string, sheetItems []SheetItem) []SheetRowUpdate {
	var updates []SheetRowUpdate

	receiverID := logEntry.Data.Receiver
	receiverName := getUserNameByID(ctx, tornClient, receiverID)
	if receiverName == "" {
		return updates
	}

	for _, logItem := range logEntry.Data.Items {
		itemUpdates := processLogItemForUpdates(ctx, tornClient, logItem, logEntry.Timestamp, receiverName, receiverID, providerName, sheetItems)
		updates = append(updates, itemUpdates...)
	}

	return updates
}

// processLogItemForUpdates processes a single log item and returns any updates found
func processLogItemForUpdates(ctx context.Context, tornClient *torn.Client, logItem torn.LogItem, timestamp int64, receiverName string, receiverID int, providerName string, sheetItems []SheetItem) []SheetRowUpdate {
	var updates []SheetRowUpdate

	itemID := logItem.ID
	itemName := getItemNameByID(ctx, tornClient, itemID)
	if itemName == "" {
		return updates
	}

	for _, sheetItem := range sheetItems {
		if !sheetItem.HasProvider &&
			matchesUser(sheetItem.UserName, receiverName, receiverID) &&
			matchesItem(sheetItem.ItemName, itemName, itemID) {

			update := createSheetRowUpdate(ctx, tornClient, sheetItem, itemID, timestamp, providerName)
			updates = append(updates, update)

			log.Info().
				Int("row", sheetItem.RowIndex).
				Str("item", sheetItem.ItemName).
				Str("user", sheetItem.UserName).
				Str("provider", providerName).
				Float64("market_value", update.MarketValue).
				Msg("Found provided item match")

			break
		}
	}

	return updates
}

// createSheetRowUpdate creates a SheetRowUpdate with market value and formatted timestamp
func createSheetRowUpdate(ctx context.Context, tornClient *torn.Client, sheetItem SheetItem, itemID int, timestamp int64, providerName string) SheetRowUpdate {
	marketValue := getItemMarketValue(ctx, tornClient, itemID)
	timestampTime := time.Unix(timestamp, 0)
	dateTime := timestampTime.Format("15:04:05 - 02/01/06")

	return SheetRowUpdate{
		RowIndex:    sheetItem.RowIndex,
		Provider:    providerName,
		DateTime:    dateTime,
		MarketValue: marketValue,
	}
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

		if updateAllSheetCells(ctx, sheetsClient, spreadsheetID, sheetName, update) {
			log.Info().
				Int("row", update.RowIndex).
				Str("provider", update.Provider).
				Str("datetime", update.DateTime).
				Float64("market_value", update.MarketValue).
				Msg("Updated provided item row")
		}
	}

	log.Debug().
		Int("updates", len(updates)).
		Msg("Finished updating provided item rows")
}

// updateAllSheetCells updates all required cells for a provided item row
func updateAllSheetCells(ctx context.Context, sheetsClient *sheets.Client, spreadsheetID, sheetName string, update SheetRowUpdate) bool {
	// Update status column (A)
	if !updateSheetCell(ctx, sheetsClient, spreadsheetID, sheetName, "A", update.RowIndex, "Provided", "status") {
		return false
	}

	// Update provider column (B)
	if !updateSheetCell(ctx, sheetsClient, spreadsheetID, sheetName, "B", update.RowIndex, update.Provider, "provider") {
		return false
	}

	// Update datetime column (D)
	if !updateSheetCell(ctx, sheetsClient, spreadsheetID, sheetName, "D", update.RowIndex, update.DateTime, "datetime") {
		return false
	}

	// Update market value column (G)
	if !updateSheetCell(ctx, sheetsClient, spreadsheetID, sheetName, "G", update.RowIndex, update.MarketValue, "market value") {
		return false
	}

	return true
}

// updateSheetCell updates a single cell in the sheet
func updateSheetCell(ctx context.Context, sheetsClient *sheets.Client, spreadsheetID, sheetName, column string, rowIndex int, value interface{}, columnDescription string) bool {
	values := [][]interface{}{
		{value},
	}
	cellRange := fmt.Sprintf("%s!%s%d", sheetName, column, rowIndex)
	if err := sheetsClient.UpdateRange(ctx, spreadsheetID, cellRange, values); err != nil {
		log.Error().Err(err).Int("row", rowIndex).Str("column", column).Msgf("Failed to update %s column", columnDescription)
		return false
	}
	return true
}

func processSuppliedItems(ctx context.Context, tornClient *torn.Client, suppliedItems []torn.SuppliedItem, existing map[string]bool) [][]interface{} {
	log.Debug().Int("count", len(suppliedItems)).Msg("Processing supplied items")
	callsBefore := tornClient.GetAPICallCount()
	var rows [][]interface{}

	for _, itm := range suppliedItems {
		crimeURL := fmt.Sprintf("http://www.torn.com/factions.php?step=your#/tab=crimes&crimeId=%d", itm.CrimeID)

		itemName := getItemDetails(ctx, tornClient, itm.ItemID)
		userName := getUserDetails(ctx, tornClient, itm.UserID)

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
