# Mattermost Mission Operations Plugin

A Mattermost plugin for planning and tracking missions. This plugin allows users to create, track, and update mission statuses, and subscribe to mission updates.

## Features

- Create new missions with departure and arrival airports
- Track mission status (stalled, in-air, completed, cancelled)
- Create dedicated mission channels
- Automatic channel categorization using Mattermost Playbooks
- Subscribe to mission status updates
- Post mission reports
- Integration with weather data (if weather plugin is installed)

## Installation

### Building the plugin

To build the plugin:

1. Make sure you have Go 1.24 or later installed
2. Clone the repository
3. Build the plugin:

```bash
cd /path/to/missionops-plugin
make
```

This will generate a `.tar.gz` file in the `dist` directory.

### Installing the plugin

1. Go to **System Console > Plugins > Plugin Management**
2. Upload the plugin file generated in the build step
3. Enable the plugin

## Usage

The plugin adds a `/mission` slash command with the following subcommands:

- `/mission start --name [name] --callsign [callsign] --departureAirport [code] --arrivalAirport [code] --crew @user1 @user2 ...` - Create a new mission
- `/mission list` - List all missions
- `/mission status [status]` - Update mission status (run in mission channel to skip --id)
- `/mission complete` - Fill out and submit a post-mission report
- `/mission subscribe --type [status1,status2] --frequency [seconds]` - Subscribe to mission status updates
- `/mission subscribe --type all --frequency [seconds]` - Subscribe to all mission status updates
- `/mission unsubscribe --id [subscription_id]` - Unsubscribe from updates
- `/mission subscriptions` - List all subscriptions in this channel
- `/mission help` - Show help message

## Valid Mission Statuses

- `stalled` - Mission is not active
- `in-air` - Mission is in progress
- `completed` - Mission has been completed successfully
- `cancelled` - Mission has been cancelled

## Examples

- `/mission start --name Alpha --callsign Eagle1 --departureAirport JFK --arrivalAirport LAX --crew @john @sarah`
- `/mission status in-air`
- `/mission status completed`
- `/mission status cancelled --id [mission_id]` (when not in mission channel)
- `/mission complete` (in a mission channel)
- `/mission subscribe --type stalled,in-air --frequency 3600` (updates hourly)
- `/mission subscribe --type all --frequency 1800` (updates every 30 minutes)

## Development

### Environment Setup

1. Make sure you have Go 1.24 or later installed
2. Install dependencies:

```bash
go mod download
```

### Testing

To run tests:

```bash
make test
```

## Differences from Standalone App

This plugin version of the Mission Operations app has several key differences from the standalone version:

1. Uses Mattermost's Key-Value store instead of files for data persistence
2. Runs inside the Mattermost server process instead of as a separate service
3. Provides better integration with Mattermost features
4. No longer needs to listen on separate ports or have HTTP endpoints
5. Uses Mattermost's built-in bot account system
6. Command handling is done through slash commands instead of webhooks
7. Uses the Mattermost Playbooks API for channel categorization

## Dependencies

This plugin has the following dependencies:

1. **Mattermost Playbooks Plugin** - Required for channel categorization features
2. **Mattermost Weather Plugin** (optional) - Used for weather integration

## License

This project is licensed under the MIT License.