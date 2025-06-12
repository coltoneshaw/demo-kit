<div align="left">
  <img src="assets/bot_icon.png" alt="Mission Operations Plugin Icon" width="128" height="128">
</div>

# Mission Operations Plugin

A Mattermost plugin for planning and tracking military-style missions with status updates and subscription management.

## Features

- Create missions with callsigns, departure/arrival airports, and crew assignments
- Track mission status (stalled, in-air, completed, cancelled)
- Subscribe to mission status updates in channels
- Post-mission report forms
- Dedicated mission channels with automatic organization

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
   - Use `/mission help` to see available commands

## Commands

### Mission Management
- `/mission start --name [name] --callsign [callsign] --departureAirport [code] --arrivalAirport [code] --crew @user1 @user2` - Create a new mission
- `/mission list` - List all missions
- `/mission status [status]` - Update mission status (run in mission channel to skip --id)
- `/mission complete` - Fill out and submit a post-mission report form
- `/mission help` - Show help message

### Subscription Management
- `/mission subscribe --type [status1,status2] --frequency [seconds]` - Subscribe to mission status updates
- `/mission subscribe --type all --frequency [seconds]` - Subscribe to all mission status updates
- `/mission unsubscribe --id [subscription_id]` - Unsubscribe from updates
- `/mission subscriptions` - List all subscriptions in this channel

### Mission Statuses
- `stalled` - Mission is not active
- `in-air` - Mission is in progress
- `completed` - Mission has been completed successfully
- `cancelled` - Mission has been cancelled

### Examples
```bash
# Create a mission
/mission start --name Alpha --callsign Eagle1 --departureAirport JFK --arrivalAirport LAX --crew @john @sarah

# Update status (in mission channel)
/mission status in-air

# Update status (outside mission channel)
/mission status completed --id mission_123

# Subscribe to updates
/mission subscribe --type stalled,in-air --frequency 3600
/mission subscribe --type all --frequency 1800

# Complete mission with report
/mission complete
```

## Development

### Build Commands
```bash
make dist          # Build and package plugin
make server        # Build server binaries only
make test          # Run tests
make clean         # Remove build artifacts
```

### Project Structure
```
apps/missionops-plugin/
├── server/                    # Go server code
│   ├── plugin.go             # Main plugin entry point
│   ├── command/              # Slash command handling
│   ├── mission/              # Mission management logic
│   ├── subscription/         # Subscription management
│   └── bot/                  # Bot user management
├── assets/                   # Plugin assets
│   └── bot_icon.png         # Bot icon
├── plugin.json              # Plugin manifest
└── Makefile                 # Build configuration
```

### Key Features
- **Mission Channels**: Automatically creates dedicated channels for each mission
- **Status Tracking**: Real-time mission status updates with notifications
- **Crew Management**: Assign and manage crew members for missions
- **Subscription System**: Channel-based notifications for mission updates
- **Report Forms**: Interactive post-mission report submission