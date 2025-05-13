# Mattermost Setup Tool

This Go package provides functionality to set up a Mattermost server with test data, including:

- Creating test users
- Creating a test team
- Setting up slash commands for flight and weather apps
- Creating webhooks for the apps

## Usage

### Build the tool

```bash
cd mattermost
go build -o mattermost-setup ./cmd
```

### Run the setup

```bash
./mattermost-setup -setup
```

### Display login information

```bash
./mattermost-setup -echo-logins
```

### Additional options

```bash
./mattermost-setup -help
```

## Configuration

You can configure the tool using command-line flags:

- `-server`: Mattermost server URL (default: http://localhost:8065)
- `-admin`: Admin username (default: sysadmin)
- `-password`: Admin password (default: Testpassword123!)
- `-team`: Team name (default: test)
