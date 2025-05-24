# Torn OC Items

A Go application for managing Torn Online OC items.

## Prerequisites

- Go 1.21 or later
- Torn API key

## Environment Variables

Create a `.env` file in the project root with the following variables:

```
TORN_API_KEY=your_api_key_here
ENV=development  # or production
LOGLEVEL=info    # debug, info, warn, error, fatal, panic, disabled
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