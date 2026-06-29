package main

import (
	"context"
	"log/slog"
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
)

var providerList []providers.Provider
var stateTracker *tracking.StateTracker

func main() {
	slog.Debug("Starting application")
	app.SetupEnvironment()

	ctx := context.Background()
	tornClient, sheetsClient := app.InitializeClients(ctx)
	notificationClient := app.InitializeNotificationClient()

	stateTracker = tracking.NewStateTracker()
	providerList = providers.LoadProviders(ctx)

	slog.Info("Starting Torn OC Items monitor. Running immediately and then every minute...")

	runProcessLoopWithRetry(ctx, tornClient, sheetsClient, notificationClient)

	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		runProcessLoopWithRetry(ctx, tornClient, sheetsClient, notificationClient)
	}
}

func runProcessLoopWithRetry(ctx context.Context, tornClient *torn.Client, sheetsClient *sheets.Client, notificationClient *notifications.Client) {
	_, err := retry.WithRetry(ctx, config.DefaultResilienceConfig.ProcessLoop, func(ctx context.Context) (struct{}, error) {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("Recovered from panic in process loop", "panic", r)
			}
		}()
		runProcessLoop(ctx, tornClient, sheetsClient, notificationClient)
		return struct{}{}, nil
	})

	if err != nil {
		slog.Error("All retry attempts exhausted, skipping this cycle", "error", err)
	}
}

func runProcessLoop(ctx context.Context, tornClient *torn.Client, sheetsClient *sheets.Client, notificationClient *notifications.Client) {
	slog.Debug("Starting process loop")
	tornClient.ResetAPICallCount()

	suppliedItems := processing.GetSuppliedItems(ctx, tornClient)
	apiCallsAfterSupplied := tornClient.GetAPICallCount()

	if len(suppliedItems) > 0 {
		slog.Debug("Processing new supplied items", "count", len(suppliedItems))

		existingData, err := retry.WithRetry(ctx, config.DefaultResilienceConfig.SheetRead, func(ctx context.Context) ([][]interface{}, error) {
			return sheets.ReadExistingSheetData(ctx, sheetsClient)
		})
		if err != nil {
			slog.Error("Failed to read existing sheet data after retries, skipping supplied items processing", "error", err)
			return
		}

		existing := sheets.BuildExistingMap(existingData)
		rows := processing.ProcessSuppliedItems(ctx, tornClient, suppliedItems, existing)
		apiCallsAfterProcessing := tornClient.GetAPICallCount()

		if len(rows) > 0 {
			slog.Debug("Updating sheet with new items", "rows", len(rows))
			_, err := retry.WithRetry(ctx, config.DefaultResilienceConfig.SheetRead, func(ctx context.Context) (struct{}, error) {
				return struct{}{}, sheets.UpdateSheet(ctx, sheetsClient, rows, len(suppliedItems), notificationClient)
			})
			if err != nil {
				slog.Error("Failed to update sheet after retries", "error", err)
				return
			}
		} else {
			slog.Debug("No new items to add to sheet")
		}

		slog.Info("API calls for processSuppliedItems()", "api_calls_processing_supplied", apiCallsAfterProcessing-apiCallsAfterSupplied)
	} else {
		slog.Debug("No supplied items found")
	}

	slog.Debug("Starting provided items processing")
	apiCallsBeforeProvided := tornClient.GetAPICallCount()
	processing.ProcessProvidedItems(ctx, tornClient, sheetsClient, providerList)
	apiCallsAfterProvided := tornClient.GetAPICallCount()

	slog.Debug("Starting state transition tracking")
	apiCallsBeforeTracking := tornClient.GetAPICallCount()
	processStateTransitions(ctx, tornClient, notificationClient)
	apiCallsAfterTracking := tornClient.GetAPICallCount()

	totalAPICalls := tornClient.GetAPICallCount()
	slog.Debug("API call summary for runProcessLoop()",
		"api_calls_get_supplied", apiCallsAfterSupplied,
		"api_calls_process_provided", apiCallsAfterProvided-apiCallsBeforeProvided,
		"api_calls_state_tracking", apiCallsAfterTracking-apiCallsBeforeTracking,
		"total_api_calls_this_loop", totalAPICalls,
	)
}

func processStateTransitions(ctx context.Context, tornClient *torn.Client, notificationClient *notifications.Client) {
	planningCrimes, err := retry.WithRetry(ctx, config.DefaultResilienceConfig.StateTracking, func(ctx context.Context) (*torn.CrimesResponse, error) {
		return tornClient.GetPlanningCrimes(ctx)
	})
	if err != nil {
		slog.Error("Failed to get planning crimes for state tracking after retries", "error", err)
		return
	}

	completedCrimes, err := retry.WithRetry(ctx, config.DefaultResilienceConfig.StateTracking, func(ctx context.Context) (*torn.CrimesResponse, error) {
		return tornClient.GetCompletedCrimes(ctx)
	})
	if err != nil {
		slog.Error("Failed to get completed crimes for state tracking after retries", "error", err)
		return
	}

	var transitions []*tracking.StateTransition

	for _, crime := range planningCrimes.Crimes {
		if transition := stateTracker.UpdateCrimeState(crime.ID, crime.Name, "planning"); transition != nil {
			transitions = append(transitions, transition)
		}
	}

	for _, crime := range completedCrimes.Crimes {
		if transition := stateTracker.UpdateCrimeState(crime.ID, crime.Name, "completed"); transition != nil {
			transitions = append(transitions, transition)
		}
	}

	planningToCompleted := 0
	for _, transition := range transitions {
		if tracking.IsTransitionOfInterest(transition) {
			planningToCompleted++
			notificationClient.NotifyStateTransition(ctx, transition.CrimeID, transition.CrimeName,
				transition.FromState, transition.ToState)
		}
	}

	slog.Debug("State transition processing complete",
		"planning_crimes", len(planningCrimes.Crimes),
		"completed_crimes", len(completedCrimes.Crimes),
		"total_transitions", len(transitions),
		"planning_to_completed", planningToCompleted,
		"tracked_crimes", stateTracker.GetTrackedCrimesCount(),
	)
}
