package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// helpConfigCmd represents the help-config command
var helpConfigCmd = &cobra.Command{
	Use:   "help-config",
	Short: "Show config.json file format and examples",
	Long: `Show detailed information about the config.json file format with examples.

The config.json file allows you to define users, teams, channels, and slash commands 
to be created automatically. This tool can connect to any Mattermost server using 
the connection flags.`,
	Run: func(cmd *cobra.Command, args []string) {
		showConfigHelp()
	},
}

func showConfigHelp() {
	fmt.Println("=== Mattermost Config.json Help ===")
	fmt.Println("The config.json file allows you to define users, teams, channels, and slash commands to be created automatically.")
	fmt.Println("This tool can connect to any Mattermost server using the connection flags.")
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
      "type": "O",
      "channels": [
        {
          "name": "general",
          "displayName": "General",
          "purpose": "General discussion",
          "type": "O",
          "members": ["admin-user", "regular-user"]
        }
      ]
    },
    "team2": {
      "name": "team2",
      "displayName": "My Second Team",
      "type": "O"
    }
  }
}`)
	fmt.Println("\nPlace this file in the root directory or specify a custom path with --config flag.")
	fmt.Println("\nUsage examples:")
	fmt.Println("  # Setup against local server (default)")
	fmt.Println("  mmsetup setup")
	fmt.Println("  # Setup against external server")
	fmt.Println("  mmsetup setup --server https://mattermost.example.com --admin sysadmin --password mypassword")
	fmt.Println("  # Setup with custom config file")
	fmt.Println("  mmsetup setup --config /path/to/config.json")
}

func init() {
	RootCmd.AddCommand(helpConfigCmd)
}