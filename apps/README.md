# Mattermost Demo Kit - Applications

This directory contains three Mattermost plugins that demonstrate different integration capabilities and use cases for the demo kit environment.

## 🌤️ Weather Plugin

<div align="left">
  <img src="weather-plugin/assets/bot.png" alt="Weather Plugin" width="100" height="100">
</div>

**Purpose**: Provides mock weather data and subscription management for demonstration purposes.

The Weather Plugin delivers randomized weather information to simulate real-world weather services. It's designed to showcase basic bot integration patterns and subscription-based notifications in Mattermost.

**Key Features**:
- Get weather information for any location (uses randomized mock data)
- Subscribe to periodic weather updates in channels
- Dedicated Weather Bot (@weatherbot) for clean message delivery
- Simple subscription management

**Use Case**: Perfect for demonstrating basic external service integration and automated notifications. Shows how teams can stay informed about environmental conditions that might affect operations.

---

## ✈️ FlightAware Plugin

<div align="left">
  <img src="flightaware-plugin/assets/bot_icon.png" alt="FlightAware Plugin" width="100" height="100">
</div>

**Purpose**: Tracks flight departures and arrivals with subscription-based monitoring.

The FlightAware Plugin provides real-time flight departure information from major airports, enabling teams to monitor flight schedules and receive automated updates about departures.

**Key Features**:
- Get live departure information from airports (SFO, LAX, JFK, RDU, etc.)
- Subscribe to flight departure updates for specific airports
- Airport code support with comprehensive flight data
- Real-time flight status notifications

**Use Case**: Essential for logistics teams, travel coordinators, and operations that depend on flight schedules. Demonstrates integration with external APIs and real-time data streaming.

---

## 🚁 Mission Operations Plugin

<div align="left">
  <img src="missionops-plugin/assets/bot_icon.png" alt="Mission Operations Plugin" width="100" height="100">
</div>

**Purpose**: Military-style mission planning and tracking with comprehensive status management.

The Mission Operations Plugin is the most sophisticated application, providing end-to-end mission lifecycle management. **This plugin integrates with both the Weather and FlightAware plugins** to provide comprehensive situational awareness during mission planning and execution.

**Key Features**:
- Create missions with callsigns, airports, and crew assignments
- Track mission status (stalled, in-air, completed, cancelled)
- Dedicated mission channels with automatic organization
- Post-mission report forms and crew management
- **Integrated weather and flight data** from other plugins
- Advanced subscription system for status updates

**Integration Dependencies**:
- **Weather Plugin**: Provides weather conditions for departure/arrival airports during mission planning
- **FlightAware Plugin**: Offers flight traffic information to avoid conflicts and optimize routing

**Use Case**: Demonstrates complex workflow management, multi-plugin integration, and advanced Mattermost capabilities. Perfect for showing how multiple services can work together to create comprehensive operational dashboards.

---

## 🔗 Plugin Integration Architecture

The three plugins work together to create a comprehensive operational environment:

```
Mission Operations (Core)
    ├── Weather Plugin (Environmental Data)
    └── FlightAware Plugin (Air Traffic Data)
```

- **Mission Operations** serves as the orchestrator, pulling data from both supporting plugins
- **Weather conditions** inform go/no-go decisions for missions
- **Flight traffic data** helps with routing and timing decisions
- All three plugins support **subscription-based notifications** for real-time updates

## 🛠️ Development

### Building All Plugins

```bash
make build-plugins
```

Or build individually:
```bash
cd weather-plugin && make dist
cd flightaware-plugin && make dist
cd missionops-plugin && make dist
```

### Installation

1. Build the desired plugin(s)
2. Upload to Mattermost via **System Console > Plugins**
3. Enable the plugin(s)

**Note**: For full functionality, install all three plugins to demonstrate the complete integration scenario.

## 🎯 Demo Scenarios

These plugins are perfect for demonstrating:

- **Basic Integration** (Weather) - Simple commands with mock data
- **External API Integration** (FlightAware) - Real-time data from external services  
- **Complex Workflows** (Mission Ops) - Multi-step processes with plugin dependencies
- **Subscription Systems** - Channel-based notification management
- **Multi-Plugin Architecture** - How plugins can work together seamlessly

Ideal for showcasing Mattermost's enterprise capabilities in defense, logistics, emergency response, and any scenario requiring coordinated operations with real-time data integration.