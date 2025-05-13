#!/bin/bash

# Constants
DIR="./volumes/mattermost"
MAX_WAIT_SECONDS=120
ENV_FILE="./files/env_vars.env"

# Helper functions
printHeader() {
  echo
  echo "==========================================================="
  echo
  echo "$1"
  echo
  echo "==========================================================="
}

printFooter() {
  echo "==========================================================="
}

# Wait for Mattermost server to start
waitForStart() {
  local total=0
  
  echo "waiting $MAX_WAIT_SECONDS seconds for the server to start"

  while [[ "$total" -le "$MAX_WAIT_SECONDS" ]]; do
    if docker exec -i mattermost mmctl system status --local 2>/dev/null; then
      echo "server started"
      return 0
    else
      ((total = total + 1))
      printf "."
      sleep 1
    fi
  done

  printf "\nserver didn't start in $MAX_WAIT_SECONDS seconds\n"

  make stop
  exit 1
}

# Create test users
createUsers() {
  # Check if sysadmin user exists before creating
  if ! docker exec -it mattermost mmctl user list --local | grep -q "sysadmin"; then
    echo "Creating sysadmin user..."
    docker exec -it mattermost mmctl user create --password Testpassword123! \
      --username sysadmin --email sysadmin@example.com --system-admin --local
  else
    echo "User 'sysadmin' already exists"
  fi
  
  # Check if user-1 exists before creating
  if ! docker exec -it mattermost mmctl user list --local | grep -q "user-1"; then
    echo "Creating user-1 user..."
    docker exec -it mattermost mmctl user create --password Testpassword123! \
      --username user-1 --email user-1@example.com --local
  else
    echo "User 'user-1' already exists"
  fi
}

# Create and set up team
createTeam() {
  # Check if team exists before creating it
  if ! docker exec -it mattermost mmctl team list --local | grep -q "test"; then
    echo "Creating test team..."
    docker exec -it mattermost mmctl team create --name test --display-name "Test Team" --local
  else
    echo "Team 'test' already exists"
  fi
  
  # Add users to the team
  docker exec -it mattermost mmctl team users add test sysadmin user-1 --local
  
  # Create slash commands
  createSlashCommands
}

# Create slash commands
createSlashCommands() {
  # Check if commands exist by listing them
  COMMANDS_LIST=$(docker exec -it mattermost mmctl command list --local)
  
  # Check for flights command
  if echo "$COMMANDS_LIST" | grep -q "flights"; then
    echo "/flights command already exists"
  else
    echo "Creating /flights slash command..."
    RESULT=$(docker exec -it mattermost mmctl command create test --title "Flight Departures" --description "Get flight departures" --trigger-word "flights" --url "http://flightaware-app:8086/webhook" --creator "sysadmin" --response-username "flight-bot" --autocomplete --local 2>&1)
    if echo "$RESULT" | grep -q "Error"; then
      echo "Warning: $RESULT"
    else
      echo "/flights command created successfully"
    fi
  fi
  
  # Check for weather command
  if echo "$COMMANDS_LIST" | grep -q "weather"; then
    echo "/weather command already exists"
  else
    echo "Creating /weather slash command..."
    RESULT=$(docker exec -it mattermost mmctl command create test --title "Weather Information" --description "Get weather information" --trigger-word "weather" --url "http://weather-app:8085/webhook" --creator "sysadmin" --response-username "weather-bot" --autocomplete --local 2>&1)
    if echo "$RESULT" | grep -q "Error"; then
      echo "Warning: $RESULT"
    else
      echo "/weather command created successfully"
    fi
  fi
}

# Update webhook configuration
updateWebhookConfig() {
  local WEBHOOK_ID="$1"
  local APP_NAME="$2"
  local ENV_VAR_NAME="$3"
  local CONTAINER_NAME="$4"
  
  echo "Created webhook with ID: $WEBHOOK_ID for $APP_NAME"
  
  # Update env_vars.env file with the webhook URL
  WEBHOOK_URL="http://mattermost:8065/hooks/$WEBHOOK_ID"
  echo "Setting webhook URL: $WEBHOOK_URL for $ENV_VAR_NAME"
  
  # Update the env_vars.env file - use different sed syntax for macOS compatibility
  if [[ "$OSTYPE" == "darwin"* ]]; then
    # macOS version of sed requires an empty string after -i
    sed -i '' "s|$ENV_VAR_NAME=.*|$ENV_VAR_NAME=$WEBHOOK_URL|" "$ENV_FILE"
  else
    # Linux version
    sed -i "s|$ENV_VAR_NAME=.*|$ENV_VAR_NAME=$WEBHOOK_URL|" "$ENV_FILE"
  fi
  echo "Updated env_vars.env with webhook URL for $APP_NAME"
  
  # Restart the app container to pick up the new webhook URL
  echo "Restarting $CONTAINER_NAME container..."
  docker restart "$CONTAINER_NAME"
  echo "$APP_NAME restarted successfully"
}

# Create app webhook
createAppWebhook() {
  local CHANNEL_ID="$1"
  local APP_NAME="$2"
  local DISPLAY_NAME="$3"
  local DESCRIPTION="$4"
  local ICON_URL="$5"
  local ENV_VAR_NAME="$6"
  local CONTAINER_NAME="$7"
  
  # Check if webhook already exists
  WEBHOOK_EXISTS=$(docker exec -it mattermost mmctl webhook list-incoming --local | grep -w "$DISPLAY_NAME")
  
  if [ -z "$WEBHOOK_EXISTS" ]; then
    echo "Creating incoming webhook for $APP_NAME..."
    WEBHOOK_RESPONSE=$(docker exec -it mattermost mmctl webhook create-incoming \
      --channel "$CHANNEL_ID" \
      --user professor \
      --display-name "$DISPLAY_NAME" \
      --description "$DESCRIPTION" \
      --icon "$ICON_URL" \
      --local)
    
    # Extract webhook ID using basic grep and sed instead of grep -P
    WEBHOOK_ID=$(echo "$WEBHOOK_RESPONSE" | grep "Id:" | sed 's/^Id: //')
    
    if [ -n "$WEBHOOK_ID" ]; then
      updateWebhookConfig "$WEBHOOK_ID" "$APP_NAME" "$ENV_VAR_NAME" "$CONTAINER_NAME"
    else
      echo "Failed to create webhook for $APP_NAME"
    fi
  else
    echo "Webhook '$DISPLAY_NAME' already exists"
  fi
}

# Create weather webhook
createWeatherWebhook() {
  local CHANNEL_ID="$1"
  createAppWebhook "$CHANNEL_ID" "Weather app" "weather" "Weather responses" "http://weather-app:8085/bot.png" "WEATHER_MATTERMOST_WEBHOOK_URL" "weather-app"
}

# Create flight webhook
createFlightWebhook() {
  local CHANNEL_ID="$1"
  createAppWebhook "$CHANNEL_ID" "Flight app" "flight-app" "Flight departures" "http://flightaware-app:8086/bot.png" "FLIGHTS_MATTERMOST_WEBHOOK_URL" "flightaware-app"
}

# Set up webhooks for apps
setupWebhooks() {
  # Get the channel ID for off-topic in the test team using channel search
  echo "Getting channel ID for off-topic in test team..."
  CHANNEL_SEARCH=$(docker exec -it mattermost mmctl channel search --team test off-topic --local)
  CHANNEL_ID=$(echo "$CHANNEL_SEARCH" | grep -o "Channel ID :[a-z0-9]*" | cut -d':' -f2 | tr -d ' ')
  
  if [ -n "$CHANNEL_ID" ]; then
    echo "Found off-topic channel ID: $CHANNEL_ID"
    
    # Setup Weather webhook
    WEATHER_WEBHOOK_URL=$(grep "WEATHER_MATTERMOST_WEBHOOK_URL=" "$ENV_FILE" | cut -d'=' -f2)
    if [ -n "$WEATHER_WEBHOOK_URL" ] && [ "$WEATHER_WEBHOOK_URL" != "" ]; then
      echo "Weather webhook URL already exists in env_vars.env: $WEATHER_WEBHOOK_URL"
    else
      createWeatherWebhook "$CHANNEL_ID"
    fi
    
    # Setup Flight webhook
    FLIGHT_WEBHOOK_URL=$(grep "FLIGHTS_MATTERMOST_WEBHOOK_URL=" "$ENV_FILE" | cut -d'=' -f2)
    if [ -n "$FLIGHT_WEBHOOK_URL" ] && [ "$FLIGHT_WEBHOOK_URL" != "" ]; then
      echo "Flight webhook URL already exists in env_vars.env: $FLIGHT_WEBHOOK_URL"
    else
      createFlightWebhook "$CHANNEL_ID"
    fi
  else
    echo "Could not find off-topic channel in test team"
  fi
}

# Set up test data in Mattermost
setupTestData() {
  printHeader "Setting up test Data for Mattermost"
  
  createUsers
  createTeam
  setupWebhooks
}

# Main setup function
setup() {
  if ! waitForStart; then
    make stop
  else
    setupTestData
    exit 0
  fi

  echo
  echo "Alright, everything seems to be setup and running. Enjoy."
}

# Print login information
echoLogins() {
  printHeader "Mattermost logins"
  
  echo "- System admin"
  echo "     - username: sysadmin"
  echo "     - password: Testpassword123!"
  echo "- Regular account:"
  echo "     - username: user-1"
  echo "     - password: Testpassword123!"
  echo "- LDAP or SAML account:"
  echo "     - username: professor"
  echo "     - password: professor"
  echo
  echo "For more logins check out https://github.com/coltoneshaw/mattermost#accounts"
  echo
  printFooter
}

# Execute the function passed as argument
"$@"
