#!/bin/bash

cd "$(dirname "$0")"

echo "Installing dependencies..."
go get -u github.com/spf13/cobra@latest
go get -u github.com/spf13/viper@latest

echo "Building Go scripts..."
go build -o scripts

echo "Making executable..."
chmod +x scripts

echo "Done! You can now run the scripts with:"
echo "./scripts/scripts general logins"
echo "./scripts/scripts keycloak restore"
echo "./scripts/scripts keycloak backup"
echo "./scripts/scripts mattermost setup"
echo "./scripts/scripts mattermost echologins"

# Create symlinks for backward compatibility
ln -sf scripts general
ln -sf scripts keycloak
ln -sf scripts mattermost

echo "Created symlinks for backward compatibility"
