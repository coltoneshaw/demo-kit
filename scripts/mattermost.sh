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
}

# Update webhook configuration
updateWebhookConfig() {
  local WEBHOOK_ID="$1"
  
  echo "Created webhook with ID: $WEBHOOK_ID"
  
  # Update env_vars.env file with the webhook URL
  WEBHOOK_URL="http://mattermost:8065/hooks/$WEBHOOK_ID"
  echo "Setting webhook URL: $WEBHOOK_URL"
  
  # Update the env_vars.env file - use different sed syntax for macOS compatibility
  if [[ "$OSTYPE" == "darwin"* ]]; then
    # macOS version of sed requires an empty string after -i
    sed -i '' "s|MATTERMOST_WEBHOOK_URL=.*|MATTERMOST_WEBHOOK_URL=$WEBHOOK_URL|" "$ENV_FILE"
  else
    # Linux version
    sed -i "s|MATTERMOST_WEBHOOK_URL=.*|MATTERMOST_WEBHOOK_URL=$WEBHOOK_URL|" "$ENV_FILE"
  fi
  echo "Updated env_vars.env with webhook URL"
  
  # Restart the weather-app container to pick up the new webhook URL
  echo "Restarting weather-app container..."
  docker restart weather-app
  echo "Weather app restarted successfully"
}

# Create weather webhook
createWeatherWebhook() {
  local CHANNEL_ID="$1"
  
  # Check if webhook already exists
  WEBHOOK_EXISTS=$(docker exec -it mattermost mmctl webhook list-incoming --local | grep -w "weather")
  
  if [ -z "$WEBHOOK_EXISTS" ]; then
    echo "Creating incoming webhook for weather app..."
    WEBHOOK_RESPONSE=$(docker exec -it mattermost mmctl webhook create-incoming \
      --channel "$CHANNEL_ID" \
      --user professor \
      --display-name weather \
      --description "Weather responses" \
      --icon http://weather-app:8085/bot.png \
      --local)
    
    # Extract webhook ID using basic grep and sed instead of grep -P
    WEBHOOK_ID=$(echo "$WEBHOOK_RESPONSE" | grep "Id:" | sed 's/^Id: //')
    
    if [ -n "$WEBHOOK_ID" ]; then
      updateWebhookConfig "$WEBHOOK_ID"
    else
      echo "Failed to create webhook"
    fi
  else
    echo "Webhook 'weather' already exists"
  fi
}

# Set up webhook for weather app
setupWebhook() {
  # Check if webhook URL is already set in env_vars.env
  WEBHOOK_URL=$(grep "MATTERMOST_WEBHOOK_URL=" "$ENV_FILE" | cut -d'=' -f2)
  
  if [ -n "$WEBHOOK_URL" ] && [ "$WEBHOOK_URL" != "" ]; then
    echo "Webhook URL already exists in env_vars.env: $WEBHOOK_URL"
    echo "Skipping webhook creation"
    return
  fi
  
  # Get the channel ID for off-topic in the test team using channel search
  echo "Getting channel ID for off-topic in test team..."
  CHANNEL_SEARCH=$(docker exec -it mattermost mmctl channel search off-topic --team test --local)
  CHANNEL_ID=$(echo "$CHANNEL_SEARCH" | grep -o "Channel ID :[a-z0-9]*" | cut -d':' -f2 | tr -d ' ')
  
  if [ -n "$CHANNEL_ID" ]; then
    echo "Found off-topic channel ID: $CHANNEL_ID"
    createWeatherWebhook "$CHANNEL_ID"
  else
    echo "Could not find off-topic channel in test team"
  fi
}

# Set up test data in Mattermost
setupTestData() {
  printHeader "Setting up test Data for Mattermost"
  
  createUsers
  createTeam
  setupWebhook
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
