package sheets

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
)

// SheetRowUpdate represents an update to be made to a sheet row
type SheetRowUpdate struct {
	RowIndex    int
	Provider    string
	DateTime    string
	MarketValue float64
}

// UpdateProvidedItemRows updates multiple rows in the sheet with provider information
func UpdateProvidedItemRows(ctx context.Context, sheetsClient *Client, updates []SheetRowUpdate) {
	slog.Debug("Updating provided item rows", "updates", len(updates))

	spreadsheetID := getRequiredEnv("SPREADSHEET_ID")
	sheetRange := getEnvWithDefault("SPREADSHEET_RANGE", "Test Sheet!A1")
	sheetName := strings.Split(sheetRange, "!")[0]

	for _, update := range updates {
		slog.Debug("Updating row",
			"row", update.RowIndex,
			"provider", update.Provider,
			"datetime", update.DateTime,
			"market_value", update.MarketValue,
		)

		if updateAllSheetCells(ctx, sheetsClient, spreadsheetID, sheetName, update) {
			slog.Info("Updated provided item row",
				"row", update.RowIndex,
				"provider", update.Provider,
				"datetime", update.DateTime,
				"market_value", update.MarketValue,
			)
		}
	}

	slog.Debug("Finished updating provided item rows", "updates", len(updates))
}

// updateAllSheetCells updates all required cells for a provided item row
func updateAllSheetCells(ctx context.Context, sheetsClient *Client, spreadsheetID, sheetName string, update SheetRowUpdate) bool {
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
func updateSheetCell(ctx context.Context, sheetsClient *Client, spreadsheetID, sheetName, column string, rowIndex int, value interface{}, columnDescription string) bool {
	values := [][]interface{}{
		{value},
	}
	cellRange := fmt.Sprintf("%s!%s%d", sheetName, column, rowIndex)
	if err := sheetsClient.UpdateRange(ctx, spreadsheetID, cellRange, values); err != nil {
		slog.Error(fmt.Sprintf("Failed to update %s column", columnDescription),
			"error", err,
			"row", rowIndex,
			"column", column,
		)
		return false
	}
	return true
}
