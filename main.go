package main

import (
	"context"
	"time"

	"torn_oc_items/internal/app"
	"torn_oc_items/internal/config"
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
	tornClient, sheetsClient := app.InitializeClients(ctx)

	// Load providers
	providerList = providers.LoadProviders(ctx)

	log.Info().Msg("Starting Torn OC Items monitor. Running immediately and then every minute...")

	runProcessLoopWithRetry(ctx, tornClient, sheetsClient)

	// Then start the ticker
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		runProcessLoopWithRetry(ctx, tornClient, sheetsClient)
	}
}

// runProcessLoopWithRetry wraps runProcessLoop with retry logic and panic recovery
func runProcessLoopWithRetry(ctx context.Context, tornClient *torn.Client, sheetsClient *sheets.Client) {
	_, err := retry.WithRetry(ctx, config.DefaultResilienceConfig.ProcessLoop, func() (struct{}, error) {
		defer func() {
			if r := recover(); r != nil {
				log.Error().
					Interface("panic", r).
					Msg("Recovered from panic in process loop")
			}
		}()

		runProcessLoop(ctx, tornClient, sheetsClient)
		return struct{}{}, nil
	})

	if err != nil {
		log.Error().
			Err(err).
			Msg("All retry attempts exhausted, skipping this cycle")
	}
}

func runProcessLoop(ctx context.Context, tornClient *torn.Client, sheetsClient *sheets.Client) {
	log.Debug().Msg("Starting process loop")

	// Reset API call counter at the start
	tornClient.ResetAPICallCount()

	// Process new supplied items
	suppliedItems := processing.GetSuppliedItems(ctx, tornClient)
	apiCallsAfterSupplied := tornClient.GetAPICallCount()

	if len(suppliedItems) > 0 {
		log.Debug().Int("count", len(suppliedItems)).Msg("Processing new supplied items")
		
		existingData, err := retry.WithRetry(ctx, config.DefaultResilienceConfig.SheetRead, func() ([][]interface{}, error) {
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
			_, err := retry.WithRetry(ctx, config.DefaultResilienceConfig.SheetRead, func() (struct{}, error) {
				return struct{}{}, sheets.UpdateSheet(ctx, sheetsClient, rows, len(suppliedItems))
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
