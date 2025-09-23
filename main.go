package main

import (
	"context"
	"time"

	"torn_oc_items/internal/app"
	"torn_oc_items/internal/config"
	"torn_oc_items/internal/notifications"
	"torn_oc_items/internal/processing"
	"torn_oc_items/internal/providers"
	"torn_oc_items/internal/retry"
	"torn_oc_items/internal/sheets"
	"torn_oc_items/internal/torn"
	"torn_oc_items/internal/tracking"

	"github.com/rs/zerolog/log"
)

// global provider list
var providerList []providers.Provider
var stateTracker *tracking.StateTracker

func main() {
	log.Debug().Msg("Starting application")
	app.SetupEnvironment()

	ctx := context.Background()
	tornClient, sheetsClient := app.InitializeClients(ctx)
	notificationClient := app.InitializeNotificationClient()

	// Initialize state tracker
	stateTracker = tracking.NewStateTracker()

	// Load providers
	providerList = providers.LoadProviders(ctx)

	log.Info().Msg("Starting Torn OC Items monitor. Running immediately and then every minute...")

	runProcessLoopWithRetry(ctx, tornClient, sheetsClient, notificationClient)

	// Then start the ticker
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		runProcessLoopWithRetry(ctx, tornClient, sheetsClient, notificationClient)
	}
}

// runProcessLoopWithRetry wraps runProcessLoop with retry logic and panic recovery
func runProcessLoopWithRetry(ctx context.Context, tornClient *torn.Client, sheetsClient *sheets.Client, notificationClient *notifications.Client) {
	_, err := retry.WithRetry(ctx, config.DefaultResilienceConfig.ProcessLoop, func(ctx context.Context) (struct{}, error) {
		defer func() {
			if r := recover(); r != nil {
				log.Error().
					Interface("panic", r).
					Msg("Recovered from panic in process loop")
			}
		}()

		runProcessLoop(ctx, tornClient, sheetsClient, notificationClient)
		return struct{}{}, nil
	})

	if err != nil {
		log.Error().
			Err(err).
			Msg("All retry attempts exhausted, skipping this cycle")
	}
}

func runProcessLoop(ctx context.Context, tornClient *torn.Client, sheetsClient *sheets.Client, notificationClient *notifications.Client) {
	log.Debug().Msg("Starting process loop")

	// Reset API call counter at the start
	tornClient.ResetAPICallCount()

	// Process new supplied items
	suppliedItems := processing.GetSuppliedItems(ctx, tornClient)
	apiCallsAfterSupplied := tornClient.GetAPICallCount()

	if len(suppliedItems) > 0 {
		log.Debug().Int("count", len(suppliedItems)).Msg("Processing new supplied items")

		existingData, err := retry.WithRetry(ctx, config.DefaultResilienceConfig.SheetRead, func(ctx context.Context) ([][]interface{}, error) {
			return sheets.ReadExistingSheetData(ctx, sheetsClient)
		})
		if err != nil {
			log.Error().Err(err).Msg("Failed to read existing sheet data after retries, skipping supplied items processing")
			return
		}

		existing := sheets.BuildExistingMap(existingData)

		rows := processing.ProcessSuppliedItems(ctx, tornClient, suppliedItems, existing)
		apiCallsAfterProcessing := tornClient.GetAPICallCount()

		if len(rows) > 0 {
			log.Debug().Int("rows", len(rows)).Msg("Updating sheet with new items")
			_, err := retry.WithRetry(ctx, config.DefaultResilienceConfig.SheetRead, func(ctx context.Context) (struct{}, error) {
				return struct{}{}, sheets.UpdateSheet(ctx, sheetsClient, rows, len(suppliedItems), notificationClient)
			})
			if err != nil {
				log.Error().Err(err).Msg("Failed to update sheet after retries")
				return
			}
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
	processing.ProcessProvidedItems(ctx, tornClient, sheetsClient, providerList)
	apiCallsAfterProvided := tornClient.GetAPICallCount()

	// Track state transitions
	log.Debug().Msg("Starting state transition tracking")
	apiCallsBeforeTracking := tornClient.GetAPICallCount()
	processStateTransitions(ctx, tornClient, notificationClient)
	apiCallsAfterTracking := tornClient.GetAPICallCount()

	totalAPICalls := tornClient.GetAPICallCount()
	log.Debug().
		Int64("api_calls_get_supplied", apiCallsAfterSupplied).
		Int64("api_calls_process_provided", apiCallsAfterProvided-apiCallsBeforeProvided).
		Int64("api_calls_state_tracking", apiCallsAfterTracking-apiCallsBeforeTracking).
		Int64("total_api_calls_this_loop", totalAPICalls).
		Msg("API call summary for runProcessLoop()")
}

func processStateTransitions(ctx context.Context, tornClient *torn.Client, notificationClient *notifications.Client) {
	// Get both planning and completed crimes to track transitions with retry
	planningCrimes, err := retry.WithRetry(ctx, config.DefaultResilienceConfig.StateTracking, func(ctx context.Context) (*torn.CrimesResponse, error) {
		return tornClient.GetPlanningCrimes(ctx)
	})
	if err != nil {
		log.Error().Err(err).Msg("Failed to get planning crimes for state tracking after retries")
		return
	}

	completedCrimes, err := retry.WithRetry(ctx, config.DefaultResilienceConfig.StateTracking, func(ctx context.Context) (*torn.CrimesResponse, error) {
		return tornClient.GetCompletedCrimes(ctx)
	})
	if err != nil {
		log.Error().Err(err).Msg("Failed to get completed crimes for state tracking after retries")
		return
	}

	// Track all crimes we see
	var transitions []*tracking.StateTransition

	// Process planning crimes
	for _, crime := range planningCrimes.Crimes {
		if transition := stateTracker.UpdateCrimeState(crime.ID, crime.Name, "planning"); transition != nil {
			transitions = append(transitions, transition)
		}
	}

	// Process completed crimes
	for _, crime := range completedCrimes.Crimes {
		if transition := stateTracker.UpdateCrimeState(crime.ID, crime.Name, "completed"); transition != nil {
			transitions = append(transitions, transition)
		}
	}

	// Handle transitions of interest
	planningToCompleted := 0
	for _, transition := range transitions {
		if tracking.IsTransitionOfInterest(transition) {
			planningToCompleted++
			notificationClient.NotifyStateTransition(ctx, transition.CrimeID, transition.CrimeName,
				transition.FromState, transition.ToState)
		}
	}

	log.Debug().
		Int("planning_crimes", len(planningCrimes.Crimes)).
		Int("completed_crimes", len(completedCrimes.Crimes)).
		Int("total_transitions", len(transitions)).
		Int("planning_to_completed", planningToCompleted).
		Int("tracked_crimes", stateTracker.GetTrackedCrimesCount()).
		Msg("State transition processing complete")
}
