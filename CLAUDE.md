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

- **main.go**: Application entry point with the main processing loop and coordination logic
- **internal/torn/client.go**: Torn API client with caching, rate limiting tracking, and comprehensive API methods
- **internal/sheets/client.go**: Google Sheets API client for reading and updating spreadsheet data
- **internal/providers/manager.go**: Provider management system that aggregates logs from multiple Torn API keys

### Key Data Flow

1. **Supplied Items**: Fetches faction crimes → identifies items needed → checks against existing sheet data → adds new entries
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

## Testing Strategy

- Integration tests exist for Torn and Sheets clients but require valid API credentials
- Tests will fail in CI/development without proper environment setup
- Use `TORN_API_KEY` environment variable for torn client tests
- Use valid `credentials.json` for sheets client tests

## Key Implementation Details

### API Rate Limiting
- Torn client tracks API call counts with thread-safe counters
- Caching implemented for user and item data (1-hour TTL)
- Provider logs are fetched for 48-hour windows

### Sheet Structure
- Column A: Status ("Needed", "Provided", "Cash Sent")
- Column B: Provider name
- Column C: Crime URL
- Column D: DateTime timestamp
- Column E: Item name
- Column F: User name  
- Column G: Market value with conditional formula

### Error Handling
- Robust error handling with structured logging using zerolog
- Failed API calls are logged but don't crash the application
- Invalid provider keys are skipped with warnings

## Security Considerations

- API keys are loaded from environment variables only
- Google credentials stored in separate JSON file
- Kubernetes deployment uses secrets for sensitive data
- Container runs as non-root user (UID 1001)