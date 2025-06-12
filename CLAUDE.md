# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Repository Overview

This is a demo kit for Mattermost with integrated weather and flight tracking applications. The repository contains a containerized environment with multiple services including Mattermost, Keycloak, Elasticsearch, Grafana, PostgreSQL, and custom microservices.

## Key Commands

### Environment Management
- `make run` - Start all services
- `make stop` - Stop all services
- `make start` - Start existing deployment
- `make restart` - Restart services
- `make reset` - Reset data and restart
- `make nuke` - Complete cleanup (removes volumes)
- `make logs` - Follow logs

### Mattermost Setup Tool
- `cd mattermost && go build -o mmsetup ./cmd` - Build the setup tool
- `./mmsetup -setup` - Run setup with default config
- `./mmsetup -setup -config /path/to/config.json` - Run setup with custom config
- `./mmsetup -help-config` - Show configuration file help
- `./mmsetup -echo-logins` - Display login information

### Component Management
- `make run-core` - Start core services (Postgres, LDAP, Prometheus, etc.)
- `make run-ai` - Set up Mattermost AI plugins
- `make run-rtcd` - Start real-time communication daemon
- `make run-integrations` - Start integration services (weather app)
- `make setup-mattermost` - Configure Mattermost
- `make restore-keycloak` - Restore Keycloak configuration
- `make echo-logins` - Display login information

## Code Architecture

### Core Components

1. **Mattermost Setup Tool** (`/mattermost` directory)
   - Written in Go
   - Creates users, teams, channels based on config.json
   - Sets up slash commands and webhooks for the apps
   - Configures integration between apps and Mattermost
   - Customizable via config.json in the root directory

2. **Weather App** (`/weather_app` directory)
   - Go-based microservice
   - Provides weather data through API endpoints
   - Supports subscription-based weather updates
   - Integrates with Mattermost via webhooks
   - Uses mock weather data for demonstration purposes

3. **Flight App** (`/flightaware_app` directory)
   - Go-based microservice
   - Tracks flight departures
   - Supports subscription-based flight monitoring
   - Integrates with Mattermost via webhooks

4. **Docker Environment**
   - Multiple containerized services defined in `docker-compose.yml`
   - Includes PostgreSQL, OpenLDAP, Prometheus, Grafana, Elasticsearch, Keycloak

### Integration Flow

1. The Mattermost setup tool configures slash commands and webhooks
2. Users interact with services via slash commands in Mattermost
3. Services receive commands via webhooks and process requests
4. Services post responses back to Mattermost via outgoing webhooks
5. Subscription-based updates are sent periodically to Mattermost

## Development Notes

- Services communicate via HTTP webhooks
- Configuration files for various services are stored in the `/files` directory
- Custom scripts for initialization are in the `/scripts` directory
- Environment variables are defined in `/files/env_vars.env`