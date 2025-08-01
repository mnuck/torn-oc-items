package resolution

import (
	"context"
	"fmt"

	"torn_oc_items/internal/config"
	"torn_oc_items/internal/retry"
	"torn_oc_items/internal/torn"

	"github.com/rs/zerolog/log"
)

// GetUserNameByID retrieves a user's name by their ID, with error handling
func GetUserNameByID(ctx context.Context, tornClient *torn.Client, userID int) string {
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

// GetUserDetails retrieves user details with fallback to ID format on error
func GetUserDetails(ctx context.Context, tornClient *torn.Client, userID int) string {
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

// MatchesUser checks if a sheet user name matches a log user name or ID
func MatchesUser(sheetUserName, logUserName string, logUserID int) bool {
	// Direct name match
	if sheetUserName == logUserName {
		return true
	}

	// Check if sheet has fallback format "User ID: X"
	expectedFallback := fmt.Sprintf("User ID: %d", logUserID)
	return sheetUserName == expectedFallback
}

// GetUserDetailsInfinite retrieves user details with infinite retry and fallback to ID format
func GetUserDetailsInfinite(ctx context.Context, tornClient *torn.Client, userID int) string {
	log.Debug().Int("user_id", userID).Msg("Getting user details (infinite retry)")
	
	userDetails, err := retry.WithRetry(ctx, config.InfiniteResilienceConfig.APIRequest, func(ctx context.Context) (*torn.UserInfo, error) {
		return tornClient.GetUser(ctx, fmt.Sprintf("%d", userID))
	})
	
	if err == nil {
		log.Debug().
			Int("user_id", userID).
			Str("name", userDetails.Name).
			Msg("Retrieved user details")
		return userDetails.Name
	}
	
	log.Warn().Err(err).Int("user_id", userID).Msg("Failed to get user details after infinite retry")
	return fmt.Sprintf("User ID: %d", userID)
}

// GetUserNameByIDInfinite retrieves a user's name by their ID with infinite retry
func GetUserNameByIDInfinite(ctx context.Context, tornClient *torn.Client, userID int) string {
	log.Debug().Int("user_id", userID).Msg("Getting user details for matching (infinite retry)")
	
	userDetails, err := retry.WithRetry(ctx, config.InfiniteResilienceConfig.APIRequest, func(ctx context.Context) (*torn.UserInfo, error) {
		return tornClient.GetUser(ctx, fmt.Sprintf("%d", userID))
	})
	
	if err != nil {
		log.Debug().Err(err).Int("user_id", userID).Msg("Failed to get user details for matching after infinite retry")
		return ""
	}
	
	log.Debug().
		Int("user_id", userID).
		Str("name", userDetails.Name).
		Msg("Retrieved user details")
	return userDetails.Name
}
