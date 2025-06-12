<div align="left">
  <img src="assets/bot.png" alt="Weather Plugin Icon" width="128" height="128">
</div>

# Weather Plugin

A Mattermost plugin that provides mock weather data and subscription management for demonstration purposes.

## Features

- Get mock weather data for any location
- Subscribe to periodic weather updates in channels
- Manage subscriptions (list, subscribe, unsubscribe)
- Dedicated bot user (@weatherbot) for weather messages

## Installation

1. **Build the plugin**:
   ```bash
   make dist
   ```

2. **Install in Mattermost**:
   - Navigate to **System Console > Plugins**
   - Upload the generated `.tar.gz` file from the `dist/` directory
   - Enable the plugin

3. **Start using**:
   - The plugin automatically creates "Weather Bot" (@weatherbot)
   - Use `/weather help` to see available commands

## Commands

### Basic Commands
- `/weather <location>` - Get current weather for a location
- `/weather help` - Show help message
- `/weather list` - List active subscriptions in this channel
- `/weather list --all` - List all subscriptions on the server

### Subscription Commands
- `/weather subscribe --location <location> --frequency <frequency>` - Subscribe to weather updates
- `/weather unsubscribe <subscription_id>` - Unsubscribe from weather updates

### Examples
```bash
/weather London
/weather subscribe --location Tokyo --frequency 1h
/weather subscribe --location "New York" --frequency 30m
/weather unsubscribe sub_1234567890
```

**Note**: This plugin uses mock weather data - any location will return randomized weather information for demonstration purposes.

## Development

### Build Commands
```bash
make dist          # Build and package plugin
make server        # Build server binaries only
make clean         # Remove build artifacts
```