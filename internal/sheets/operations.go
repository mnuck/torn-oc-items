package sheets

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"torn_oc_items/internal/notifications"
)

// SheetItem represents a parsed item from the spreadsheet
type SheetItem struct {
	RowIndex    int
	CrimeURL    string
	ItemName    string
	UserName    string
	Provider    string
	HasProvider bool
}

// ReadExistingSheetData reads all existing data from the spreadsheet
func ReadExistingSheetData(ctx context.Context, sheetsClient *Client) ([][]interface{}, error) {
	slog.Debug("Reading existing sheet data")
	spreadsheetID := getRequiredEnv("SPREADSHEET_ID")
	sheetRange := getEnvWithDefault("SPREADSHEET_RANGE", "Test Sheet!A1")
	readRange := strings.Split(sheetRange, "!")[0] + "!A1:Z1000"
	existingData, err := sheetsClient.ReadSheet(ctx, spreadsheetID, readRange)
	if err != nil {
		return nil, fmt.Errorf("failed to read existing sheet data: %w", err)
	}
	slog.Debug("Retrieved existing sheet data", "rows", len(existingData))
	return existingData, nil
}

// BuildExistingMap creates a map of existing items for duplicate detection
func BuildExistingMap(existingData [][]interface{}) map[string]bool {
	slog.Debug("Building existing items map")
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
	slog.Debug("Built existing items map", "entries", len(existing))
	return existing
}

// ParseSheetItems parses raw sheet data into structured SheetItem objects
func ParseSheetItems(existingData [][]interface{}) []SheetItem {
	slog.Debug("Parsing sheet items", "rows", len(existingData))
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

	slog.Debug("Finished parsing sheet items", "total_rows", len(existingData), "parsed_items", len(items))
	return items
}

// isValidSheetRow checks if a row has sufficient columns
func isValidSheetRow(row []interface{}, rowNum int) bool {
	if len(row) < 6 {
		slog.Debug("Skipping row with insufficient columns", "row", rowNum, "columns", len(row))
		return false
	}
	return true
}

// extractSheetItemFromRow extracts all fields from a sheet row
func extractSheetItemFromRow(row []interface{}, rowIndex int) SheetItem {
	provider := ""
	hasProvider := false
	if len(row) > 1 && row[1] != nil {
		provider = strings.TrimSpace(fmt.Sprintf("%v", row[1]))
		hasProvider = provider != ""
	}

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
	slog.Debug("Skipping row with missing required fields",
		"row", rowNum,
		"crime_url", item.CrimeURL,
		"item_name", item.ItemName,
		"user_name", item.UserName,
	)
	return false
}

// UpdateSheet appends new rows to the spreadsheet and sends notifications
func UpdateSheet(ctx context.Context, sheetsClient *Client, rows [][]interface{}, totalItems int, notificationClient *notifications.Client) error {
	slog.Debug("Updating sheet", "rows", len(rows), "total_items", totalItems)

	if len(rows) == 0 {
		slog.Debug("No rows to add, skipping sheet update")
		return nil
	}

	spreadsheetID := getRequiredEnv("SPREADSHEET_ID")
	sheetRange := getEnvWithDefault("SPREADSHEET_RANGE", "Test Sheet!A1")

	if err := sheetsClient.AppendRows(ctx, spreadsheetID, sheetRange, rows); err != nil {
		return fmt.Errorf("failed to append rows to sheet: %w", err)
	}

	skipped := totalItems - len(rows)
	slog.Info("Sheet update complete", "added", len(rows), "skipped", skipped)

	if notificationClient != nil && len(rows) > 0 {
		items := extractNotificationItems(rows)
		notificationClient.NotifyNewItems(ctx, items, len(rows))
	}

	return nil
}

// extractNotificationItems converts sheet rows to notification items
func extractNotificationItems(rows [][]interface{}) []notifications.ItemInfo {
	var items []notifications.ItemInfo
	for _, row := range rows {
		if len(row) >= 6 {
			crimeURL := ""
			itemName := ""
			userName := ""
			if row[2] != nil {
				crimeURL = fmt.Sprintf("%v", row[2])
			}
			if row[4] != nil {
				itemName = fmt.Sprintf("%v", row[4])
			}
			if row[5] != nil {
				userName = fmt.Sprintf("%v", row[5])
			}
			if itemName != "" && userName != "" {
				items = append(items, notifications.ItemInfo{
					ItemName: itemName,
					UserName: userName,
					CrimeURL: crimeURL,
				})
			}
		}
	}
	return items
}
