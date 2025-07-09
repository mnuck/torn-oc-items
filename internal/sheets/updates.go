package sheets

import (
	"context"
	"fmt"
	"strings"

	"github.com/rs/zerolog/log"
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
		log.Error().Err(err).Int("row", rowIndex).Str("column", column).Msgf("Failed to update %s column", columnDescription)
		return false
	}
	return true
}
