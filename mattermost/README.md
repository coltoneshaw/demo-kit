# Mattermost Setup Tool

This Go package provides comprehensive functionality to set up and manage a Mattermost demo environment, including:

- **User and Team Management**: Creates users, teams, and channels from configuration
- **Plugin Management**: Installs and manages both local and GitHub plugins with version checking
- **Bulk Import**: Handles large-scale data import with two-phase processing
- **Channel Organization**: Automatically categorizes channels using Playbooks API
- **Data Reset**: Safely removes demo data with confirmation prompts

## Usage

### Build the tool

```bash
cd mattermost
go build -o mmsetup ./cmd
```

### Basic Setup Commands

```bash
# Complete setup with default configuration
./mmsetup setup

# Setup with custom configuration file
./mmsetup setup --config /path/to/config.json

# Show help for configuration format
./mmsetup help-config

# Display login information
./mmsetup echo-logins

# Wait for Mattermost server to be ready
./mmsetup wait-for-start
```

### Plugin Management Commands

```bash
# Standard setup - intelligently skips already installed plugins
./mmsetup setup

# Check for updates and install newer plugin versions
./mmsetup setup --check-updates

# Force reinstall local plugins only (skips GitHub plugins)
./mmsetup setup --reinstall-plugins local

# Force reinstall all plugins (local + GitHub)
./mmsetup setup --reinstall-plugins all

# Combine update checking with forced reinstall
./mmsetup setup --reinstall-plugins all --check-updates
```

### Data Import

The setup tool uses a two-phase import system for optimal reliability:

```bash
# Two-phase import - infrastructure first, then users
./mmsetup setup
```

### Data Management

```bash
# Reset all demo data with confirmation prompt
./mmsetup reset
```

## Configuration

### Command-line Flags

You can configure the tool using command-line flags:

- `--server`: Mattermost server URL (default: http://localhost:8065)
- `--admin`: Admin username (default: sysadmin)
- `--password`: Admin password (default: Testpassword123!)
- `--team`: Team name (default: test)
- `--config`: Path to configuration JSON file

### Plugin Management Features

#### Automatic Plugin Detection
- Detects already installed plugins to avoid conflicts
- Compares plugin IDs and versions intelligently
- Supports both local custom plugins and GitHub releases

#### Version Checking (`--check-updates`)
- Fetches latest release information from GitHub
- Compares semantic versions (v1.2.3 format)
- Only updates when newer versions are available
- Works with both local and GitHub plugin sources

#### Selective Plugin Reinstall
- `--reinstall-plugins local`: Only rebuilds and redeploys custom local plugins
- `--reinstall-plugins all`: Forces reinstall of both local and GitHub plugins
- Separate from data import operations for better control

### Data Import System

#### Two-Phase Import Process
1. **Infrastructure Phase**: Teams and channels created first
2. **Channel Categorization**: Organizes channels using Playbooks API
3. **User Phase**: Users imported and added to teams/channels
4. **Command Execution**: Slash commands executed in appropriate channels

#### Custom Data Types
- **Channel Categories**: Automatically organizes channels into sidebar categories
- **Channel Commands**: Executes slash commands after setup completion
- **Error Handling**: Continues processing even if individual items fail

### Channel Categorization

The tool integrates with Mattermost Playbooks plugin for automatic channel organization:

```json
{
  "type": "channel-category",
  "category": "Operations", 
  "team": "usaf-team",
  "channels": ["mission-planning", "flight-schedules", "ops-weather"]
}
```

Features:
- **Duplicate Prevention**: Checks existing categorization before creating new actions
- **Conflict Detection**: Logs when channels already have different categories
- **Silent Operation**: Minimal logging when everything is already configured
- **Error Recovery**: Continues processing even if individual categorizations fail

### Data Reset Safety

The reset command includes comprehensive safety measures:

- **Confirmation Prompt**: Requires typing "DELETE" to confirm
- **Data Summary**: Shows counts of teams, users, and references to bulk import file
- **Irreversibility Warning**: Clear messaging about permanent data loss
- **API Validation**: Ensures deletion APIs are enabled before proceeding

## Architecture

### Plugin Management Flow
1. **Local Plugin Building**: Compiles custom plugins from source using `make dist`
2. **GitHub Plugin Download**: Fetches latest releases from configured repositories
3. **Installation**: Uploads and enables plugins via Mattermost API
4. **Version Tracking**: Maintains state to avoid unnecessary reinstalls

### Data Import Process
1. **File Parsing**: Reads JSONL files and separates by data type
2. **Zip Creation**: Packages data for Mattermost import API
3. **Upload Session**: Transfers files using proper session management
4. **Job Monitoring**: Waits for import jobs to complete successfully
5. **Cleanup**: Removes temporary files after processing

### Error Handling
- **Graceful Degradation**: Individual failures don't stop entire process
- **Detailed Logging**: Clear feedback on what actions are being taken
- **State Validation**: Checks server configuration before proceeding
- **Recovery**: Safe to re-run setup multiple times
