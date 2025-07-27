# Torn OC Items

A resilient Go application for managing Torn Online OC items with comprehensive timeout and crash protection.

## Prerequisites

- Go 1.21 or later
- Torn API key
- Google Sheets API credentials

## Environment Variables

Create a `.env` file in the project root with the following variables:

```env
# Required
SPREADSHEET_ID=your_spreadsheet_id    # ID of the target Google Spreadsheet
SPREADSHEET_RANGE=Test Sheet!A1       # Sheet range to read/write (default: Test Sheet!A1)
TORN_API_KEY=your_api_key_here        # Your Torn API key for general API access
TORN_FACTION_API_KEY=your_api_key     # Your Torn Faction API key for faction-specific endpoints
PROVIDER_KEYS=key1,key2               # Comma-separated item provider Full Access Torn API keys

# Optional
ENV=development                       # Environment: development or production
LOGLEVEL=info                         # Log level: debug, info, warn, error, fatal, panic, disabled
```

## Features

- **Automated Item Tracking**: Monitors faction crimes and tracks item supply needs
- **Multi-Provider Support**: Aggregates logs from multiple Torn API keys
- **Google Sheets Integration**: Real-time updates to spreadsheet with supply status
- **Comprehensive Resilience**: Automatic retry with exponential backoff and jitter
- **Crash Protection**: Panic recovery and graceful degradation
- **Context-Aware Operations**: Proper timeout and cancellation handling
- **Structured Logging**: Detailed debugging with zerolog

## Architecture

The application consists of several key components:

- **Main Loop**: Resilient processing loop with retry logic and panic recovery
- **Torn API Client**: Rate-limited client with caching and automatic retry
- **Google Sheets Client**: Handles spreadsheet read/write operations
- **Provider Manager**: Aggregates item logs from multiple API keys
- **Retry Utility**: Reusable exponential backoff with jitter and context support
- **Configuration**: Structured resilience settings and timeouts

## Building

```bash
go build
```

## Testing

```bash
# Run all tests (requires API credentials for integration tests)
go test ./...

# Run retry utility tests (no external dependencies)
go test ./internal/retry

# Static analysis
go vet ./...
```

## Resilience Features

### **Timeout & Crash Protection**
- **Main loop retry**: 3 attempts with 5s-60s exponential backoff
- **HTTP request retry**: 3 attempts with 1s-30s exponential backoff
- **Panic recovery**: Graceful handling of unexpected failures
- **Context cancellation**: Proper timeout and cancellation support

### **Smart Retry Logic**
- **Exponential backoff**: 2^attempt * baseDelay with configurable maximum
- **Jitter**: Random 0.5x-1.5x multiplier prevents thundering herd
- **Overflow protection**: Safe handling of large retry counts
- **Accurate API counting**: Only successful requests increment counters

### **Operational Benefits**
- **Continuous operation**: No manual restarts required
- **Graceful degradation**: Failed cycles are logged and skipped
- **Unmonitored deployment**: Suitable for production environments
- **Detailed logging**: Comprehensive debugging information

## Configuration

Resilience settings are configured in `internal/config/resilience.go`:

```go
var DefaultResilienceConfig = ResilienceConfig{
    ProcessLoop: retry.Config{
        MaxRetries: 3,
        BaseDelay:  5 * time.Second,
        MaxDelay:   60 * time.Second,
    },
    APIRequest: retry.Config{
        MaxRetries: 3,
        BaseDelay:  1 * time.Second,
        MaxDelay:   30 * time.Second,
    },
    HTTPTimeout: 10 * time.Second,
}
```
