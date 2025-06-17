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

The setup tool includes intelligent plugin management with version checking and automatic updates:

```bash
# Standard setup - skips already installed plugins
./mmsetup setup

# Check for plugin updates and install newer versions
./mmsetup setup --check-updates

# Force reinstall local plugins only
./mmsetup setup --reinstall-plugins local

# Force reinstall all plugins (local + GitHub)
./mmsetup setup --reinstall-plugins all

# Combine update checking with plugin reinstall
./mmsetup setup --reinstall-plugins all --check-updates
```

### Data Management

```bash
# Reset all teams and users (with confirmation prompt)
./mmsetup reset

# Use bulk import method instead of individual API calls
./mmsetup setup --bulk-import
```

## Features

### Intelligent Plugin Management
- **Automatic Plugin Detection**: Skips already installed plugins to avoid conflicts
- **Version Checking**: Compares installed plugin versions with GitHub releases
- **Selective Updates**: Update only when newer versions are available
- **Local vs GitHub Plugins**: Separate handling for custom local plugins and GitHub releases
- **Smart Categorization**: Automatically organizes channels using Playbooks API integration

### Robust Bulk Import
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