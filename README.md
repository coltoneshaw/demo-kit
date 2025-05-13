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
      "type": "O"
    },
    "marketing": {
      "name": "marketing",
      "displayName": "Marketing",
      "description": "Marketing department team",
      "type": "O"
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

## Usage

### Building

```bash
cd mattermost
go build -o mmsetup ./cmd
```

### Running

```bash
# Use default config.json in root directory
./mmsetup -setup

# Specify a custom config file path
./mmsetup -setup -config /path/to/config.json

# Display help for config.json format
./mmsetup -help-config

# Show login information
./mmsetup -echo-logins
```

## Applications

The demo kit includes several integrated applications:

- **Weather App**: Provides weather data via slash commands
- **Flight App**: Tracks flight departures
- **Main Mattermost Server**: Enterprise Edition deployment