#!/bin/bash

DIR="./volumes/mattermost"

setup() {
  if ! waitForStart; then
    make stop
  else

    echo ===========================================================
    echo
    echo "setting up test Data for Mattermost"
    echo
    echo ===========================================================

    # docker exec -it mattermost mmctl config patch /mattermost/config/defaultConfig.json --local
    
    # Check if sysadmin user exists before creating
    if ! docker exec -it mattermost mmctl user list --local | grep -q "sysadmin"; then
        echo "Creating sysadmin user..."
        docker exec -it mattermost mmctl user create --password Testpassword123! --username sysadmin --email sysadmin@example.com --system-admin --local
    else
        echo "User 'sysadmin' already exists"
    fi
    
    # Check if user-1 exists before creating
    if ! docker exec -it mattermost mmctl user list --local | grep -q "user-1"; then
        echo "Creating user-1 user..."
        docker exec -it mattermost mmctl user create --password Testpassword123! --username user-1 --email user-1@example.com --local
    else
        echo "User 'user-1' already exists"
    fi
    
    # Check if team exists before creating it
    if ! docker exec -it mattermost mmctl team list --local | grep -q "test"; then
        echo "Creating test team..."
        docker exec -it mattermost mmctl team create --name test --display-name "Test Team" --local
    else
        echo "Team 'test' already exists"
    fi
    
    # Add users to the team
    docker exec -it mattermost mmctl team users add test sysadmin user-1 --local
    exit 0
  fi

  echo
  echo "Alright, everything seems to be setup and running. Enjoy."

}

total=0
max_wait_seconds=120

waitForStart() {
  echo "waiting $max_wait_seconds seconds for the server to start"

  while [[ "$total" -le "$max_wait_seconds" ]]; do
    if docker exec -i mattermost mmctl system status --local 2>/dev/null; then
      echo "server started"
      return 0
    else
      ((total = total + 1))
      printf "."
      sleep 1
    fi
  done

  printf "\nserver didn't start in $max_wait_seconds seconds\n"

  make stop
  exit 1
}

echoLogins() {
  echo
  echo ========================================================================
  echo
  echo "Mattermost logins:"
  echo
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
  echo ========================================================================
}

"$@"
