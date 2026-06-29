package resolution

import (
	"context"
	"fmt"
	"log/slog"

	"torn_oc_items/internal/torn"
)

// GetItemNameByID retrieves an item's name by its ID, with error handling
func GetItemNameByID(ctx context.Context, tornClient *torn.Client, itemID int) string {
	slog.Debug("Getting item details", "item_id", itemID)
	itemDetails, err := tornClient.GetItem(ctx, fmt.Sprintf("%d", itemID))
	if err != nil {
		slog.Debug("Failed to get item details for matching", "item_id", itemID, "error", err)
		return ""
	}
	slog.Debug("Retrieved item details", "item_id", itemID, "name", itemDetails.Name)
	return itemDetails.Name
}

// GetItemDetails retrieves item details with fallback to ID format on error
func GetItemDetails(ctx context.Context, tornClient *torn.Client, itemID int) string {
	slog.Debug("Getting item details", "item_id", itemID)
	itemDetails, err := tornClient.GetItem(ctx, fmt.Sprintf("%d", itemID))
	if err == nil {
		slog.Debug("Retrieved item details", "item_id", itemID, "name", itemDetails.Name)
		return itemDetails.Name
	}
	slog.Warn("Failed to get item details", "item_id", itemID, "error", err)
	return fmt.Sprintf("Item ID: %d", itemID)
}

// GetItemMarketValue retrieves the market value of an item by its ID
func GetItemMarketValue(ctx context.Context, tornClient *torn.Client, itemID int) float64 {
	slog.Debug("Getting item market value", "item_id", itemID)
	item, err := tornClient.GetItem(ctx, fmt.Sprintf("%d", itemID))
	if err != nil {
		slog.Warn("Failed to get item market value", "item_id", itemID, "error", err)
		return 0
	}
	return item.MarketValue
}

// MatchesItem checks if a sheet item name matches a log item name or ID
func MatchesItem(sheetItemName, logItemName string, logItemID int) bool {
	if sheetItemName == logItemName {
		return true
	}
	expectedFallback := fmt.Sprintf("Item ID: %d", logItemID)
	return sheetItemName == expectedFallback
}
