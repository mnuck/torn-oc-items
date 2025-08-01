package main

import (
	"context"
	"fmt"
	"time"

	"torn_oc_items/internal/app"
	"torn_oc_items/internal/config"
	"torn_oc_items/internal/notifications"
	"torn_oc_items/internal/processing"
	"torn_oc_items/internal/providers"
	"torn_oc_items/internal/retry"
	"torn_oc_items/internal/sheets"
	"torn_oc_items/internal/torn"

	"github.com/rs/zerolog/log"
)

// global provider list
var providerList []providers.Provider

func main() {
	log.Debug().Msg("Starting application")
	app.SetupEnvironment()

	ctx := context.Background()
	
	// Initialize clients with infinite retry
	var tornClient *torn.Client
	var sheetsClient *sheets.Client
	
	retry.WithRetry(ctx, config.InfiniteResilienceConfig.ProcessLoop, func(ctx context.Context) (struct{}, error) {
		var err error
		tornClient, sheetsClient, err = app.InitializeClientsWithRetry(ctx)
		if err != nil {
			log.Error().Err(err).Msg("Failed to initialize clients, will retry infinitely")
			return struct{}{}, err
		}
		return struct{}{}, nil
	})
	
	notificationClient := app.InitializeNotificationClient()

	// Load providers with infinite retry
	retry.WithRetry(ctx, config.InfiniteResilienceConfig.ProcessLoop, func(ctx context.Context) (struct{}, error) {
		providerList = providers.LoadProvidersWithRetry(ctx)
		return struct{}{}, nil
	})

	log.Info().Msg("Starting Torn OC Items monitor. Running immediately and then every minute...")

	runProcessLoopWithInfiniteRetry(ctx, tornClient, sheetsClient, notificationClient)

	// Then start the ticker
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		runProcessLoopWithInfiniteRetry(ctx, tornClient, sheetsClient, notificationClient)
	}
}

// runProcessLoopWithInfiniteRetry wraps runProcessLoop with infinite retry logic and panic recovery
func runProcessLoopWithInfiniteRetry(ctx context.Context, tornClient *torn.Client, sheetsClient *sheets.Client, notificationClient *notifications.Client) {
	retry.WithRetry(ctx, config.InfiniteResilienceConfig.ProcessLoop, func(ctx context.Context) (struct{}, error) {
		var panicErr error
		defer func() {
			if r := recover(); r != nil {
				log.Error().
					Interface("panic", r).
					Msg("Recovered from panic in process loop - will retry infinitely")
				panicErr = fmt.Errorf("panic recovered: %v", r)
			}
		}()

		runProcessLoopInfinite(ctx, tornClient, sheetsClient, notificationClient)
		return struct{}{}, panicErr
	})
	// Note: WithRetry with InfiniteRetry will never return an error, only if context is canceled
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

func runProcessLoopInfinite(ctx context.Context, tornClient *torn.Client, sheetsClient *sheets.Client, notificationClient *notifications.Client) {
	log.Debug().Msg("Starting process loop (infinite retry mode)")

	// Reset API call counter at the start
	tornClient.ResetAPICallCount()

	// Process new supplied items
	suppliedItems := processing.GetSuppliedItemsInfinite(ctx, tornClient)
	apiCallsAfterSupplied := tornClient.GetAPICallCount()

	if len(suppliedItems) > 0 {
		log.Debug().Int("count", len(suppliedItems)).Msg("Processing new supplied items")
		
		existingData, _ := retry.WithRetry(ctx, config.InfiniteResilienceConfig.SheetRead, func(ctx context.Context) ([][]interface{}, error) {
			return sheets.ReadExistingSheetData(ctx, sheetsClient)
		})
		
		existing := sheets.BuildExistingMap(existingData)

		rows := processing.ProcessSuppliedItemsInfinite(ctx, tornClient, suppliedItems, existing)
		apiCallsAfterProcessing := tornClient.GetAPICallCount()

		if len(rows) > 0 {
			log.Debug().Int("rows", len(rows)).Msg("Updating sheet with new items")
			retry.WithRetry(ctx, config.InfiniteResilienceConfig.SheetRead, func(ctx context.Context) (struct{}, error) {
				return struct{}{}, sheets.UpdateSheet(ctx, sheetsClient, rows, len(suppliedItems), notificationClient)
			})
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
	processing.ProcessProvidedItemsInfinite(ctx, tornClient, sheetsClient, providerList)
	apiCallsAfterProvided := tornClient.GetAPICallCount()

	totalAPICalls := tornClient.GetAPICallCount()
	log.Debug().
		Int64("api_calls_get_supplied", apiCallsAfterSupplied).
		Int64("api_calls_process_provided", apiCallsAfterProvided-apiCallsBeforeProvided).
		Int64("total_api_calls_this_loop", totalAPICalls).
		Msg("API call summary for runProcessLoopInfinite()")
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

	totalAPICalls := tornClient.GetAPICallCount()
	log.Debug().
		Int64("api_calls_get_supplied", apiCallsAfterSupplied).
		Int64("api_calls_process_provided", apiCallsAfterProvided-apiCallsBeforeProvided).
		Int64("total_api_calls_this_loop", totalAPICalls).
		Msg("API call summary for runProcessLoop()")
}
