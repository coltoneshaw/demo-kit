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
  
  # Update the env_vars.env file
  sed -i "s|MATTERMOST_WEBHOOK_URL=.*|MATTERMOST_WEBHOOK_URL=$WEBHOOK_URL|" "$ENV_FILE"
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
    WEBHOOK_ID=$(docker exec -it mattermost mmctl webhook create-incoming \
      --channel "$CHANNEL_ID" \
      --user professor \
      --display-name weather \
      --description "Weather responses" \
      --icon http://weather-app:8085/bot.png \
      --local | grep -oP 'Id: \K[a-z0-9]+')
    
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
