package resolution

import (
	"context"
	"fmt"
	"log/slog"

	"torn_oc_items/internal/torn"
)

// GetUserNameByID retrieves a user's name by their ID, with error handling
func GetUserNameByID(ctx context.Context, tornClient *torn.Client, userID int) string {
	slog.Debug("Getting user details", "user_id", userID)
	userDetails, err := tornClient.GetUser(ctx, fmt.Sprintf("%d", userID))
	if err != nil {
		slog.Debug("Failed to get user details for matching", "user_id", userID, "error", err)
		return ""
	}
	slog.Debug("Retrieved user details", "user_id", userID, "name", userDetails.Name)
	return userDetails.Name
}

// GetUserDetails retrieves user details with fallback to ID format on error
func GetUserDetails(ctx context.Context, tornClient *torn.Client, userID int) string {
	slog.Debug("Getting user details", "user_id", userID)
	userDetails, err := tornClient.GetUser(ctx, fmt.Sprintf("%d", userID))
	if err == nil {
		slog.Debug("Retrieved user details", "user_id", userID, "name", userDetails.Name)
		return userDetails.Name
	}
	slog.Warn("Failed to get user details", "user_id", userID, "error", err)
	return fmt.Sprintf("User ID: %d", userID)
}

// MatchesUser checks if a sheet user name matches a log user name or ID
func MatchesUser(sheetUserName, logUserName string, logUserID int) bool {
	if sheetUserName == logUserName {
		return true
	}
	expectedFallback := fmt.Sprintf("User ID: %d", logUserID)
	return sheetUserName == expectedFallback
}
