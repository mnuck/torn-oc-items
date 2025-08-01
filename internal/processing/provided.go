package processing

import (
	"context"
	"strings"
	"time"

	"torn_oc_items/internal/config"
	"torn_oc_items/internal/providers"
	"torn_oc_items/internal/resolution"
	"torn_oc_items/internal/retry"
	"torn_oc_items/internal/sheets"
	"torn_oc_items/internal/torn"

	"github.com/rs/zerolog/log"
)

// ProcessProvidedItems handles the complete workflow of processing provided items
func ProcessProvidedItems(ctx context.Context, tornClient *torn.Client, sheetsClient *sheets.Client, providerList []providers.Provider) {
	log.Debug().Msg("Starting provided items processing")

	// Get current sheet data first
	existingData, err := retry.WithRetry(ctx, config.DefaultResilienceConfig.SheetRead, func(ctx context.Context) ([][]interface{}, error) {
		return sheets.ReadExistingSheetData(ctx, sheetsClient)
	})
	if err != nil {
		log.Error().Err(err).Msg("Failed to read existing sheet data after retries, skipping provided items processing")
		return
	}
	
	sheetItems := sheets.ParseSheetItems(existingData)

	log.Debug().
		Int("total_rows", len(existingData)).
		Int("parsed_items", len(sheetItems)).
		Msg("Parsed sheet items")

	// Get item send logs from all providers
	logResp := providers.AggregateLogs(ctx, providerList)

	// Find sheet rows that need provider updates
	updates := FindProviderUpdates(ctx, tornClient, sheetItems, logResp)
	if len(updates) > 0 {
		log.Debug().
			Int("updates", len(updates)).
			Msg("Updating provided item rows")
		sheets.UpdateProvidedItemRows(ctx, sheetsClient, updates)
	} else {
		log.Debug().Msg("No provided items to update")
	}
}

// FindProviderUpdates finds updates for sheet items based on provider logs
func FindProviderUpdates(ctx context.Context, tornClient *torn.Client, sheetItems []sheets.SheetItem, logResp *torn.LogResponse) []sheets.SheetRowUpdate {
	var updates []sheets.SheetRowUpdate

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
func processLogEntryForUpdates(ctx context.Context, tornClient *torn.Client, logEntry torn.LogEntry, providerName string, sheetItems []sheets.SheetItem) []sheets.SheetRowUpdate {
	var updates []sheets.SheetRowUpdate

	receiverID := logEntry.Data.Receiver
	receiverName := resolution.GetUserNameByID(ctx, tornClient, receiverID)
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
func processLogItemForUpdates(ctx context.Context, tornClient *torn.Client, logItem torn.LogItem, timestamp int64, receiverName string, receiverID int, providerName string, sheetItems []sheets.SheetItem) []sheets.SheetRowUpdate {
	var updates []sheets.SheetRowUpdate

	itemID := logItem.ID
	itemName := resolution.GetItemNameByID(ctx, tornClient, itemID)
	if itemName == "" {
		return updates
	}

	for _, sheetItem := range sheetItems {
		if !sheetItem.HasProvider &&
			resolution.MatchesUser(sheetItem.UserName, receiverName, receiverID) &&
			resolution.MatchesItem(sheetItem.ItemName, itemName, itemID) {

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
func createSheetRowUpdate(ctx context.Context, tornClient *torn.Client, sheetItem sheets.SheetItem, itemID int, timestamp int64, providerName string) sheets.SheetRowUpdate {
	marketValue := resolution.GetItemMarketValue(ctx, tornClient, itemID)
	timestampTime := time.Unix(timestamp, 0)
	dateTime := timestampTime.Format("15:04:05 - 02/01/06")

	return sheets.SheetRowUpdate{
		RowIndex:    sheetItem.RowIndex,
		Provider:    providerName,
		DateTime:    dateTime,
		MarketValue: marketValue,
	}
}

// ProcessProvidedItemsInfinite handles the complete workflow of processing provided items with infinite retry
func ProcessProvidedItemsInfinite(ctx context.Context, tornClient *torn.Client, sheetsClient *sheets.Client, providerList []providers.Provider) {
	log.Debug().Msg("Starting provided items processing (infinite retry)")

	// Get current sheet data with infinite retry
	existingData, _ := retry.WithRetry(ctx, config.InfiniteResilienceConfig.SheetRead, func(ctx context.Context) ([][]interface{}, error) {
		return sheets.ReadExistingSheetData(ctx, sheetsClient)
	})
	
	sheetItems := sheets.ParseSheetItems(existingData)

	log.Debug().
		Int("total_rows", len(existingData)).
		Int("parsed_items", len(sheetItems)).
		Msg("Parsed sheet items")

	// Get item send logs from all providers
	logResp := providers.AggregateLogsInfinite(ctx, providerList)

	// Find sheet rows that need provider updates
	updates := FindProviderUpdatesInfinite(ctx, tornClient, sheetItems, logResp)
	if len(updates) > 0 {
		log.Debug().
			Int("updates", len(updates)).
			Msg("Updating provided item rows")
		sheets.UpdateProvidedItemRowsInfinite(ctx, sheetsClient, updates)
	} else {
		log.Debug().Msg("No provided items to update")
	}
}

// FindProviderUpdatesInfinite finds updates for sheet items based on provider logs with infinite retry
func FindProviderUpdatesInfinite(ctx context.Context, tornClient *torn.Client, sheetItems []sheets.SheetItem, logResp *torn.LogResponse) []sheets.SheetRowUpdate {
	var updates []sheets.SheetRowUpdate

	log.Debug().
		Int("sheet_items", len(sheetItems)).
		Int("log_entries", len(logResp.Log)).
		Msg("Starting provider update matching (infinite retry)")

	for combinedID, logEntry := range logResp.Log {
		providerName := extractProviderName(combinedID)
		logEntryUpdates := processLogEntryForUpdatesInfinite(ctx, tornClient, logEntry, providerName, sheetItems)
		updates = append(updates, logEntryUpdates...)
	}

	log.Debug().
		Int("updates_found", len(updates)).
		Msg("Completed provider update matching")

	return updates
}

// processLogEntryForUpdatesInfinite processes a single log entry and returns any updates found with infinite retry
func processLogEntryForUpdatesInfinite(ctx context.Context, tornClient *torn.Client, logEntry torn.LogEntry, providerName string, sheetItems []sheets.SheetItem) []sheets.SheetRowUpdate {
	var updates []sheets.SheetRowUpdate

	receiverID := logEntry.Data.Receiver
	receiverName := resolution.GetUserNameByIDInfinite(ctx, tornClient, receiverID)
	if receiverName == "" {
		return updates
	}

	for _, logItem := range logEntry.Data.Items {
		itemUpdates := processLogItemForUpdatesInfinite(ctx, tornClient, logItem, logEntry.Timestamp, receiverName, receiverID, providerName, sheetItems)
		updates = append(updates, itemUpdates...)
	}

	return updates
}

// processLogItemForUpdatesInfinite processes a single log item and returns any updates found with infinite retry
func processLogItemForUpdatesInfinite(ctx context.Context, tornClient *torn.Client, logItem torn.LogItem, timestamp int64, receiverName string, receiverID int, providerName string, sheetItems []sheets.SheetItem) []sheets.SheetRowUpdate {
	var updates []sheets.SheetRowUpdate

	itemID := logItem.ID
	itemName := resolution.GetItemNameByIDInfinite(ctx, tornClient, itemID)
	if itemName == "" {
		return updates
	}

	for _, sheetItem := range sheetItems {
		if !sheetItem.HasProvider &&
			resolution.MatchesUser(sheetItem.UserName, receiverName, receiverID) &&
			resolution.MatchesItem(sheetItem.ItemName, itemName, itemID) {

			update := createSheetRowUpdateInfinite(ctx, tornClient, sheetItem, itemID, timestamp, providerName)
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

// createSheetRowUpdateInfinite creates a SheetRowUpdate with market value and formatted timestamp with infinite retry
func createSheetRowUpdateInfinite(ctx context.Context, tornClient *torn.Client, sheetItem sheets.SheetItem, itemID int, timestamp int64, providerName string) sheets.SheetRowUpdate {
	marketValue := resolution.GetItemMarketValueInfinite(ctx, tornClient, itemID)
	timestampTime := time.Unix(timestamp, 0)
	dateTime := timestampTime.Format("15:04:05 - 02/01/06")

	return sheets.SheetRowUpdate{
		RowIndex:    sheetItem.RowIndex,
		Provider:    providerName,
		DateTime:    dateTime,
		MarketValue: marketValue,
	}
}
