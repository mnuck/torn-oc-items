package processing

import (
	"context"
	"log/slog"
	"time"

	"torn_oc_items/internal/config"
	"torn_oc_items/internal/providers"
	"torn_oc_items/internal/resolution"
	"torn_oc_items/internal/retry"
	"torn_oc_items/internal/sheets"
	"torn_oc_items/internal/torn"
)

// ProcessProvidedItems handles the complete workflow of processing provided items
func ProcessProvidedItems(ctx context.Context, tornClient *torn.Client, sheetsClient *sheets.Client, providerList []providers.Provider) {
	slog.Debug("Starting provided items processing")

	existingData, err := retry.WithRetry(ctx, config.DefaultResilienceConfig.SheetRead, func(ctx context.Context) ([][]interface{}, error) {
		return sheets.ReadExistingSheetData(ctx, sheetsClient)
	})
	if err != nil {
		slog.Error("Failed to read existing sheet data after retries, skipping provided items processing", "error", err)
		return
	}

	sheetItems := sheets.ParseSheetItems(existingData)
	slog.Debug("Parsed sheet items", "total_rows", len(existingData), "parsed_items", len(sheetItems))

	logEntries := providers.AggregateLogs(ctx, providerList)

	updates := FindProviderUpdates(ctx, tornClient, sheetItems, logEntries)
	if len(updates) > 0 {
		slog.Debug("Updating provided item rows", "updates", len(updates))
		sheets.UpdateProvidedItemRows(ctx, sheetsClient, updates)
	} else {
		slog.Debug("No provided items to update")
	}
}

// FindProviderUpdates finds updates for sheet items based on provider logs
func FindProviderUpdates(ctx context.Context, tornClient *torn.Client, sheetItems []sheets.SheetItem, logEntries []providers.ProviderLogEntry) []sheets.SheetRowUpdate {
	var updates []sheets.SheetRowUpdate

	slog.Debug("Starting provider update matching", "sheet_items", len(sheetItems), "log_entries", len(logEntries))

	for _, ple := range logEntries {
		logEntryUpdates := processLogEntryForUpdates(ctx, tornClient, ple.Entry, ple.ProviderName, sheetItems)
		updates = append(updates, logEntryUpdates...)
	}

	slog.Debug("Completed provider update matching", "updates_found", len(updates))
	return updates
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

	for i := len(sheetItems) - 1; i >= 0; i-- {
		sheetItem := sheetItems[i]
		if !sheetItem.HasProvider &&
			resolution.MatchesUser(sheetItem.UserName, receiverName, receiverID) &&
			resolution.MatchesItem(sheetItem.ItemName, itemName, itemID) {

			update := createSheetRowUpdate(ctx, tornClient, sheetItem, itemID, timestamp, providerName)
			updates = append(updates, update)

			slog.Info("Found provided item match",
				"row", sheetItem.RowIndex,
				"item", sheetItem.ItemName,
				"user", sheetItem.UserName,
				"provider", providerName,
				"market_value", update.MarketValue,
			)
			break
		}
	}

	return updates
}

// createSheetRowUpdate creates a SheetRowUpdate with market value and formatted timestamp
func createSheetRowUpdate(ctx context.Context, tornClient *torn.Client, sheetItem sheets.SheetItem, itemID int, timestamp int64, providerName string) sheets.SheetRowUpdate {
	marketValue := resolution.GetItemMarketValue(ctx, tornClient, itemID)
	dateTime := time.Unix(timestamp, 0).Format("15:04:05 - 02/01/06")

	return sheets.SheetRowUpdate{
		RowIndex:    sheetItem.RowIndex,
		Provider:    providerName,
		DateTime:    dateTime,
		MarketValue: marketValue,
	}
}
