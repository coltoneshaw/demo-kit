# AdvancedLogging.json

This is a json example to build the advanced logs for Mattermost. This will include the `stdout` files generated by ldap / sql trace. To use this format to a string and set `LogSettings.AdvancedLogging` or format to a string and use the environment variable `MM_LOGSETTINGS_ADVANCEDLOGGING`.

## Weather API

The repository also includes a simple Go-based weather API in the `weather` directory that provides weather information using mock data.

### Features

- `/weather` endpoint to get weather data for any location
- `/incoming` endpoint for Mattermost webhook integration
- Command-line test mode for quick weather checks
- Uses mock weather data for demonstration purposes

### Setup

1. Run the server:
   ```
   go run main.go
   ```

### Usage

#### Web API

Get weather for a location:
```
curl "http://localhost:8085/weather?location=raleigh,nc"
```

#### Mattermost Integration

Set up a slash command in Mattermost that posts to your `/incoming` endpoint.
The command will return weather for the specified location or default to Wendell, NC.

#### Test Mode

Run a quick test without starting the server:
```
go run main.go -test -location="miami,fl"
```

### Environment Variables

- `PORT`: Server port (default: 8085)
