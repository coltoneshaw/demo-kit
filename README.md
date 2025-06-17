# Mattermost Demo Kit

This repository contains tools for setting up and configuring a Mattermost demo environment with integrated applications.

## Configuration

### User and Team Configuration

You can customize the users and teams created by the Mattermost setup tool by editing the `config.json` file in the root directory.

Example `config.json`:

```json
{
  "users": [
    {
      "username": "sysadmin",
      "email": "sysadmin@example.com",
      "password": "Testpassword123!",
      "nickname": "System Administrator",
      "isSystemAdmin": true,
      "teams": ["test", "marketing"]
    },
    {
      "username": "user-1",
      "email": "user-1@example.com",
      "password": "Testpassword123!",
      "nickname": "Regular User",
      "isSystemAdmin": false,
      "teams": ["test"]
    }
  ],
  "teams": {
    "test": {
      "name": "test",
      "displayName": "Test Team",
      "description": "Team for testing",
      "type": "O",
      "channels": [
        {
          "name": "announcements",
          "displayName": "Announcements",
          "purpose": "Important team announcements",
          "header": "📢 Official announcements channel",
          "type": "O",
          "members": ["sysadmin", "user-1"]
        },
        {
          "name": "development",
          "displayName": "Development",
          "purpose": "Development discussions",
          "type": "O",
          "members": ["sysadmin", "user-1"]
        },
        {
          "name": "private-notes",
          "displayName": "Private Notes",
          "purpose": "Admin-only private notes",
          "type": "P",
          "members": ["sysadmin"]
        }
      ]
    },
    "marketing": {
      "name": "marketing",
      "displayName": "Marketing",
      "description": "Marketing department team",
      "type": "O",
      "channels": [
        {
          "name": "campaigns",
          "displayName": "Campaigns",
          "purpose": "Marketing campaign planning",
          "type": "O",
          "members": ["sysadmin"]
        }
      ]
    }
  }
}
```

### Configuration Options

#### Users

Each user in the `users` array has the following properties:

- `username` (required): The login name for the user
- `email` (required): The user's email address
- `password` (required): The user's password
- `nickname` (optional): A display name for the user
- `isSystemAdmin` (required): Whether the user should have system admin privileges
- `teams` (required): An array of team names the user should belong to

#### Teams

Each team in the `teams` map has the following properties:

- `name` (required): The team name (URL-friendly identifier)
- `displayName` (required): The human-readable team name
- `description` (optional): A description of the team
- `type` (optional): The team type - "O" for open (default), "I" for invite only
- `channels` (optional): An array of channel configurations for this team (see below)

#### Channels

Each channel in a team's `channels` array has the following properties:

- `name` (required): The channel name (URL-friendly identifier, lowercase with no spaces)
- `displayName` (required): The human-readable channel name
- `purpose` (optional): A brief description of the channel's purpose
- `header` (optional): Text that appears in the channel header
- `type` (optional): The channel type - "O" for public (default), "P" for private
- `members` (optional): An array of usernames to add to this channel

## Usage

### Building

```bash
cd mattermost
go build -o mmsetup ./cmd
```

### Running

```bash
# Use default config.json in root directory
./mmsetup setup

# Specify a custom config file path
./mmsetup setup -config /path/to/config.json

# Display help for config.json format
./mmsetup help-config

# Show login information
./mmsetup echo-logins
```

### Plugin Management

The setup tool uses a JSONL-based plugin management system where plugins are defined in `bulk_import.jsonl`:

#### JSONL Plugin Configuration

Plugins are configured as JSON objects in the `bulk_import.jsonl` file:

```json
{"type": "plugin", "plugin": {"source": "github", "github_repo": "mattermost/mattermost-plugin-playbooks", "plugin_id": "playbooks", "name": "Playbooks", "force_install": false}}
{"type": "plugin", "plugin": {"source": "local", "path": "./apps/weather-plugin", "plugin_id": "com.coltoneshaw.weather", "name": "Weather Plugin", "force_install": false}}
```

#### Plugin Configuration Fields

- **`source`** (required): Either "github" or "local"
- **`github_repo`** (required for GitHub plugins): Repository in "owner/repo" format
- **`path`** (required for local plugins): Relative path to plugin directory
- **`plugin_id`** (required): Unique plugin identifier matching the plugin manifest
- **`name`** (required): Human-readable name for logging
- **`force_install`** (optional): Whether to force reinstall (default: false)

#### GitHub Plugins
- Must have GitHub releases with .tar.gz assets
- Downloads latest release automatically
- Plugin ID must match the actual plugin manifest ID

#### Local Plugins
- Must have a Makefile with `clean` and `dist` targets
- `make dist` must produce a .tar.gz file in the plugin's `dist/` directory
- Plugin path should be relative to the mmsetup binary location

#### Command Line Options

```bash
# Standard setup - skips already installed plugins
./mmsetup setup

# Force reinstall local plugins only
./mmsetup setup --reinstall-plugins local

# Force reinstall all plugins (local + GitHub)
./mmsetup setup --reinstall-plugins all
```

#### Adding New Plugins

1. Add a plugin entry to `bulk_import.jsonl`
2. For GitHub plugins: specify `github_repo` and `plugin_id`
3. For local plugins: specify `path` and ensure Makefile exists
4. Run setup command - plugins are processed automatically

#### Processing Order
1. GitHub plugins are processed first
2. Local plugins are processed second
3. Within each type, plugins are processed in JSONL file order

### Data Management

```bash
# Reset all teams and users (with confirmation prompt)
./mmsetup reset
```

## Features

### JSONL-Based Plugin Management
- **Configuration-Driven**: Plugins defined in `bulk_import.jsonl` for easy management
- **Automatic Plugin Detection**: Skips already installed plugins to avoid conflicts
- **Dual Source Support**: Handles both GitHub releases and local plugin development
- **Force Installation Options**: Granular control over plugin reinstallation
- **Smart Processing Order**: GitHub plugins first, then local plugins for optimal dependency handling
- **Build Integration**: Automatic building of local plugins with Makefile support

### Robust Data Import
- **Two-Phase Import**: Separates infrastructure (teams/channels) from user data for better reliability
- **Custom Type Support**: Handles channel categories and commands alongside standard Mattermost data
- **Error Recovery**: Continues processing even if individual items fail
- **File Lifecycle Management**: Proper cleanup of temporary files and uploads

### Safety and Validation
- **Confirmation Prompts**: Destructive operations require explicit confirmation with data counts
- **Idempotent Operations**: Safe to run setup multiple times without data corruption
- **Comprehensive Logging**: Clear feedback on what actions are being taken
- **License Validation**: Ensures Mattermost Enterprise features are available

## Applications

The demo kit includes several integrated applications:

- **Weather App**: Provides weather data via slash commands
- **Flight App**: Tracks flight departures
- **Main Mattermost Server**: Enterprise Edition deployment