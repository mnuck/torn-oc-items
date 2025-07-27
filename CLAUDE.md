# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Torn OC Items is a Go application that monitors Torn Online organized crime activities and manages item tracking through Google Sheets integration. The application:

- Fetches faction crime data from Torn API to identify items that need to be supplied to members
- Tracks which items have been provided by monitoring item send logs from multiple providers
- Updates a Google Spreadsheet with supply status, provider information, and timestamps
- Runs continuously with 1-minute intervals to process new data

## Architecture

### Core Components

- **main.go**: Application entry point with resilient main processing loop and coordination logic
- **internal/torn/client.go**: Torn API client with caching, rate limiting tracking, and retry-enabled API methods
- **internal/sheets/client.go**: Google Sheets API client for reading and updating spreadsheet data
- **internal/providers/manager.go**: Provider management system that aggregates logs from multiple Torn API keys
- **internal/notifications/**: Push notification system using ntfy.sh for new item alerts
- **internal/retry/**: Reusable retry utility with exponential backoff, jitter, and context cancellation
- **internal/config/**: Structured configuration for resilience settings and timeouts

### Key Data Flow

1. **Supplied Items**: Fetches faction crimes → identifies items needed → checks against existing sheet data → adds new entries → sends notifications
2. **Provided Items**: Reads sheet data → fetches provider logs → matches items to recipients → updates sheet with provider info

### Important Types

- `SuppliedItem`: Items that need to be provided (ItemID, UserID, CrimeID)
- `SheetRowUpdate`: Updates for existing sheet rows (provider, timestamp, market value)
- `LogResponse`: Aggregated item send logs from all providers

## Development Commands

### Building
```bash
go build                    # Build the application
```

### Testing
```bash
go test ./...              # Run all tests (requires API keys for integration tests)
go test ./internal/retry   # Run retry utility tests (no external dependencies)
go vet ./...               # Static analysis
```

### Code Quality
```bash
gofmt -l .                 # Check formatting (should return nothing if formatted)
go fmt ./...               # Format all Go files
```

### Docker Build
```bash
docker build -t localhost:32000/torn-oc-items:0.0.2 -f build/Dockerfile .
```

## Environment Configuration

The application requires a `.env` file with:

**Required:**
- `SPREADSHEET_ID`: Target Google Spreadsheet ID
- `TORN_API_KEY`: General Torn API access
- `TORN_FACTION_API_KEY`: Faction-specific endpoints
- `PROVIDER_KEYS`: Comma-separated item provider API keys

**Optional:**
- `SPREADSHEET_RANGE`: Sheet range (default: "Test Sheet!A1")
- `ENV`: Environment (development/production)
- `LOGLEVEL`: Logging level (debug/info/warn/error)

**Notifications:**
- `NTFY_ENABLED`: Enable/disable notifications (default: "false")
- `NTFY_URL`: Ntfy server URL (default: "https://ntfy.sh")
- `NTFY_TOPIC`: Notification topic name (default: "torn-oc-items")
- `NTFY_BATCH_MODE`: Send batch notifications vs individual (default: "true")
- `NTFY_PRIORITY`: Notification priority level - "min", "low", "default", "high", "max" (default: "default")
- `NTFY_MAX_RETRIES`: Maximum retry attempts for failed notifications (default: 3)
- `NTFY_BASE_DELAY_MS`: Base delay between retries in milliseconds (default: 1000)
- `NTFY_MAX_DELAY_MS`: Maximum delay between retries in milliseconds (default: 30000)

## Testing Strategy

- Integration tests exist for Torn and Sheets clients but require valid API credentials
- Tests will fail in CI/development without proper environment setup
- Use `TORN_API_KEY` environment variable for torn client tests
- Use valid `credentials.json` for sheets client tests

## Key Implementation Details

### API Rate Limiting & Resilience
- Torn client tracks API call counts with thread-safe counters (only successful requests counted)
- Caching implemented for user and item data (1-hour TTL)
- Provider logs are fetched for 48-hour windows
- Automatic retry with exponential backoff for failed API requests (3 attempts, 1s-30s delays)
- Jitter applied to prevent thundering herd during outages

### Sheet Structure
- Column A: Status ("Needed", "Provided", "Cash Sent")
- Column B: Provider name
- Column C: Crime URL
- Column D: DateTime timestamp
- Column E: Item name
- Column F: User name  
- Column G: Market value with conditional formula

### Error Handling & Resilience
- **Comprehensive retry system** with exponential backoff and jitter
- **Main loop protection** with panic recovery and retry logic (3 attempts, 5s-60s delays)
- **Context-aware operations** with proper timeout and cancellation handling
- **Graceful degradation** - failed cycles are logged and skipped, application continues
- **Structured logging** with zerolog for debugging retry attempts and failures
- **Invalid provider keys** are skipped with warnings
- **Overflow protection** prevents integer overflow in exponential backoff calculations

### Notification Resilience
- **Exponential backoff retry** with jitter for failed notifications
- **Circuit breaker pattern** prevents overwhelming failed ntfy service
- **Categorized error handling** for network, auth, rate limiting, and server errors
- **Metrics tracking** for notification success/failure rates and retry attempts
- **Graceful degradation** when ntfy service is unavailable

## Security Considerations

- API keys are loaded from environment variables only
- Google credentials stored in separate JSON file
- Kubernetes deployment uses secrets for sensitive data
- Container runs as non-root user (UID 1001)