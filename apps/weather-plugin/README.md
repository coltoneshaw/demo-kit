<div align="left">
  <img src="assets/bot.png" alt="Weather Plugin Icon" width="128" height="128">
</div>

# Weather Plugin

A Mattermost plugin for getting weather data and managing weather subscriptions using the Tomorrow.io API.

## Features

- Get current weather data for any location
- Subscribe to periodic weather updates in channels
- Manage API usage limits
- List active subscriptions
- Uses a dedicated bot user "Weather Bot" (@weatherbot) for all weather messages

## Installation

1. Build the plugin: `make dist`
2. Upload the generated `.tar.gz` file to your Mattermost server via System Console > Plugins
3. Configure the Tomorrow.io API key in the plugin settings
4. The plugin will automatically create a bot user named "Weather Bot" (@weatherbot) when activated

## Configuration

- **Tomorrow.io API Key**: Required for accessing weather data. Get your API key from [Tomorrow.io](https://www.tomorrow.io/)

## Commands

### Basic Commands
- `/weather <location>` - Get current weather for a location
- `/weather help` - Show help information
- `/weather limits` - Show API usage limits and current usage
- `/weather list` - List active subscriptions in the channel

### Subscription Commands
- `/weather subscribe <location> <frequency>` - Subscribe to weather updates
- `/weather unsubscribe <subscription_id>` - Unsubscribe from weather updates

### Examples
- `/weather London` - Get current weather for London
- `/weather subscribe Tokyo 3600000` - Get hourly weather updates for Tokyo (3600000ms = 1 hour)
- `/weather subscribe "San Francisco" 1h` - Get hourly weather updates for San Francisco
- `/weather unsubscribe sub_1234567890` - Unsubscribe from a specific subscription

## API Limits

The plugin enforces API usage limits to prevent excessive calls:
- 25 requests per hour
- 500 requests per day

Use `/weather limits` to check current usage.

## Development

### Building
```bash
make server   # Build server binaries
make bundle   # Create plugin bundle
make dist     # Build and bundle
```

### Structure
- `server/plugin.go` - Main plugin entry point
- `server/weather_service.go` - Weather API integration
- `server/subscription_manager.go` - Subscription management
- `server/command_handler.go` - Slash command handling
- `server/configuration.go` - Plugin configuration