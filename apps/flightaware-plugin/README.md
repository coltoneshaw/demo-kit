<div align="left">
  <img src="assets/bot_icon.png" alt="FlightAware Plugin Icon" width="128" height="128">
</div>

# FlightAware Plugin
  
A Mattermost plugin for tracking flight departures using fake flight data.

## Features

- Query flight departures from specific airports
- Subscribe to periodic flight updates with customizable frequency
- Uses fake flight data stored in `flights.json`
- Supports common airport code conversions (SFO -> KSFO, etc.)

## Commands

### One-time Queries
- `/flights departures --airport [code]` - Get recent departures from an airport
- `/flights departures [code]` - Alternative syntax without flags

### Subscription Commands
- `/flights subscribe --airport [code] --frequency [seconds]` - Subscribe to airport departures
- `/flights subscribe [code] [frequency]` - Alternative syntax without flags
- `/flights unsubscribe --id [subscription_id]` - Unsubscribe from airport departures
- `/flights unsubscribe [subscription_id]` - Alternative syntax without flags
- `/flights list` - List all subscriptions in this channel
- `/flights list --all` - List all subscriptions across the server
- `/flights help` - Show help message

### Examples
- `/flights departures SFO` - Get departures from San Francisco International
- `/flights departures --airport RDU` - Get departures from Raleigh-Durham International
- `/flights subscribe EGLL 3600` - Subscribe to hourly updates for London Heathrow
- `/flights subscribe --airport LAX --frequency 1800` - Subscribe to updates every 30 minutes
- `/flights list --all` - View all active subscriptions on the server

## Technical Details

### Frequency Limits
- Minimum update frequency: 300 seconds (5 minutes)
- Default frequency: 3600 seconds (1 hour)

### Airport Codes
The plugin automatically converts 3-letter IATA codes to 4-letter ICAO codes:
- SFO → KSFO (San Francisco International)
- LAX → KLAX (Los Angeles International)  
- JFK → KJFK (John F. Kennedy International)
- RDU → KRDU (Raleigh-Durham International)
- EGLL (London Heathrow - already ICAO)

### Data Format
Flight information includes:
- Flight callsign and airline
- Departure time
- Destination airport
- Flight duration (when available)

## Building

```bash
make dist
```

This will:
- Check code style and run tests
- Build the server binary for multiple architectures
- Create a bundled tar.gz file in the `dist/` directory

### Development Commands

```bash
make all          # Run style checks, tests, and build
make check-style  # Run linting and style checks
make test         # Run unit tests
make server       # Build server binary only
make clean        # Remove build artifacts
```

## Installation

1. Build the plugin using `make dist`
2. Upload the generated tar.gz file from `dist/` to your Mattermost server
3. Enable the plugin in System Console

### Development Installation

```bash
make deploy       # Build and install to local Mattermost server
make reset        # Reset the plugin (disable/enable)
make logs         # View plugin logs
```

## Configuration

No configuration required. The plugin uses static flight data from `flights.json` for demonstration purposes.