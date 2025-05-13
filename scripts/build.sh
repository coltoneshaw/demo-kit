#!/bin/bash

cd "$(dirname "$0")"

echo "Building Go scripts..."
go build -o general general.go
go build -o keycloak keycloak.go
go build -o mattermost mattermost.go

echo "Making executables..."
chmod +x general
chmod +x keycloak
chmod +x mattermost

echo "Done! You can now run the scripts with:"
echo "./scripts/general logins"
echo "./scripts/keycloak restore"
echo "./scripts/keycloak backup"
echo "./scripts/mattermost setup"
echo "./scripts/mattermost echoLogins"
