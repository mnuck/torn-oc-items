package main

import (
	"context"
	"fmt"
	"time"

	"torn_oc_items/internal/app"
	"torn_oc_items/internal/processing"
	"torn_oc_items/internal/providers"
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
	const maxRetries = 3
	const baseDelay = 5 * time.Second
	const maxDelay = 60 * time.Second

	for attempt := 0; attempt <= maxRetries; attempt++ {
		func() {
			defer func() {
				if r := recover(); r != nil {
					log.Error().
						Interface("panic", r).
						Int("attempt", attempt+1).
						Msg("Recovered from panic in process loop")
				}
			}()

			err := runProcessLoopSafe(ctx, tornClient, sheetsClient)
			if err == nil {
				return // Success
			}

			log.Error().
				Err(err).
				Int("attempt", attempt+1).
				Int("max_retries", maxRetries).
				Msg("Process loop failed, will retry if attempts remaining")

			if attempt < maxRetries {
				delay := time.Duration(min(1<<attempt, int(maxDelay/baseDelay))) * baseDelay
				log.Info().
					Dur("delay", delay).
					Int("next_attempt", attempt+2).
					Msg("Retrying process loop after delay")
				time.Sleep(delay)
			}
		}()
	}

	log.Error().
		Int("max_retries", maxRetries).
		Msg("All retry attempts exhausted, skipping this cycle")
}

// runProcessLoopSafe is a wrapper around runProcessLoop that returns errors instead of panicking
func runProcessLoopSafe(ctx context.Context, tornClient *torn.Client, sheetsClient *sheets.Client) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic in process loop: %v", r)
		}
	}()

	runProcessLoop(ctx, tornClient, sheetsClient)
	return nil
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
		existingData := sheets.ReadExistingSheetData(ctx, sheetsClient)
		existing := sheets.BuildExistingMap(existingData)

		rows := processing.ProcessSuppliedItems(ctx, tornClient, suppliedItems, existing)
		apiCallsAfterProcessing := tornClient.GetAPICallCount()

		if len(rows) > 0 {
			log.Debug().Int("rows", len(rows)).Msg("Updating sheet with new items")
			sheets.UpdateSheet(ctx, sheetsClient, rows, len(suppliedItems))
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
