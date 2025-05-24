# Torn OC Items

A Go application for managing Torn Online OC items.

## Prerequisites

- Go 1.21 or later
- Torn API key
- Google Sheets API credentials

## Environment Variables

Create a `.env` file in the project root with the following variables:

```env
# Required
TORN_API_KEY=your_api_key_here        # Your Torn API key
TORN_FACTION_API_KEY=your_api_key     # Your Torn Faction API key
SHEETS_CREDENTIALS=path/to/creds.json # Path to Google Sheets API credentials file
SPREADSHEET_ID=your_spreadsheet_id    # ID of the target Google Spreadsheet

# Optional
ENV=development                       # Environment: development or production
LOGLEVEL=info                         # Log level: debug, info, warn, error, fatal, panic, disabled
SPREADSHEET_RANGE=Test Sheet!A1       # Sheet range to read/write (default: Test Sheet!A1)
```

## Building

```bash
go build
```

## Running

```bash
./torn_oc_items
```

## Logging

The application uses zerolog for logging. Log levels can be configured via the `LOGLEVEL` environment variable:

- debug: Detailed debugging information
- info: General operational information
- warn: Warning messages
- error: Error messages
- fatal: Fatal errors
- panic: Panic messages
- disabled: No logging
