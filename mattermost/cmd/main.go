package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/coltoneshaw/demokit/mattermost"
)

func main() {
	// Define command line flags
	setupCmd := flag.Bool("setup", false, "Run the setup process")
	echoLoginsCmd := flag.Bool("echo-logins", false, "Display login information")
	serverURL := flag.String("server", "http://localhost:8065", "Mattermost server URL")
	adminUser := flag.String("admin", "systemadmin", "Admin username")
	adminPass := flag.String("password", "Password123!", "Admin password")
	teamName := flag.String("team", "test", "Team name")
	configPath := flag.String("config", "", "Path to config.json file (default: ../config.json when run from mattermost dir)")

	// Define a help flag to show examples
	helpConfig := flag.Bool("help-config", false, "Show help for the config.json file")

	flag.Parse()

	// Create a new Mattermost client
	client := mattermost.NewClient(*serverURL, *adminUser, *adminPass, *teamName, *configPath)

	// Show help for the config file if requested
	if *helpConfig {
		fmt.Println("=== Mattermost Config.json Help ===")
		fmt.Println("The config.json file allows you to define users and teams to be created automatically.")
		fmt.Println("\nExample config.json:")
		fmt.Println(`
{
  "users": [
    {
      "username": "admin-user",
      "email": "admin@example.com",
      "password": "SecurePassword123!",
      "nickname": "Administrator", 
      "isSystemAdmin": true,
      "teams": ["team1", "team2"]
    },
    {
      "username": "regular-user",
      "email": "user@example.com",
      "password": "SecurePassword123!",
      "isSystemAdmin": false,
      "teams": ["team1"]
    }
  ],
  "teams": {
    "team1": {
      "name": "team1",
      "displayName": "My First Team",
      "description": "Team description",
      "type": "O"
    },
    "team2": {
      "name": "team2",
      "displayName": "My Second Team",
      "type": "O"
    }
  }
}`)
		fmt.Println("\nPlace this file in the root directory or specify a custom path with -config flag.")
		return
	}

	// Execute the requested command
	if *setupCmd {
		if err := client.Setup(); err != nil {
			log.Fatalf("Setup failed: %v", err)
		}
	} else if *echoLoginsCmd {
		client.EchoLogins()
	} else {
		fmt.Println("No command specified. Use -setup or -echo-logins")
		fmt.Println("Use -help-config for information about the config.json file")
		os.Exit(1)
	}
}
