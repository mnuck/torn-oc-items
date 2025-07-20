package resolution

import (
	"context"
	"fmt"

	"torn_oc_items/internal/torn"

	"github.com/rs/zerolog/log"
)

// GetItemNameByID retrieves an item's name by its ID, with error handling
func GetItemNameByID(ctx context.Context, tornClient *torn.Client, itemID int) string {
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

// GetItemDetails retrieves item details with fallback to ID format on error
func GetItemDetails(ctx context.Context, tornClient *torn.Client, itemID int) string {
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

// GetItemMarketValue retrieves the market value of an item by its ID
func GetItemMarketValue(ctx context.Context, tornClient *torn.Client, itemID int) float64 {
	log.Debug().Int("item_id", itemID).Msg("Getting item market value")
	item, err := tornClient.GetItem(ctx, fmt.Sprintf("%d", itemID))
	if err != nil {
		log.Warn().Err(err).Int("item_id", itemID).Msg("Failed to get item market value")
		return 0
	}
	return item.MarketValue
}

// MatchesItem checks if a sheet item name matches a log item name or ID
func MatchesItem(sheetItemName, logItemName string, logItemID int) bool {
	// Direct name match
	if sheetItemName == logItemName {
		return true
	}

	// Check if sheet has fallback format "Item ID: X"
	expectedFallback := fmt.Sprintf("Item ID: %d", logItemID)
	return sheetItemName == expectedFallback
}
