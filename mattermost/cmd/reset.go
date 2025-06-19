package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/coltoneshaw/demokit/mattermost"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// resetCmd represents the reset command
var resetCmd = &cobra.Command{
	Use:   "reset",
	Short: "Reset (permanently delete) teams and users from configuration",
	Long: `Reset (permanently delete) teams and users from the configuration file.

This command will:
- Check that ServiceSettings.EnableAPIUserDeletion is enabled
- Check that ServiceSettings.EnableAPITeamDeletion is enabled
- Permanently delete all users from the configuration
- Permanently delete all teams from the configuration

WARNING: This operation is irreversible. All data will be permanently deleted.

The reset operation requires the Mattermost server to have the following settings enabled:
- ServiceSettings.EnableAPIUserDeletion = true
- ServiceSettings.EnableAPITeamDeletion = true`,
	Run: func(cmd *cobra.Command, args []string) {
		// Load the config first
		config, err := mattermost.LoadConfig(configPath)
		if err != nil {
			mattermost.Log.WithFields(logrus.Fields{
				"error": err.Error(),
				"path":  configPath,
			}).Fatal("Failed to load config file")
		}

		// Create client using config values
		client := mattermost.NewClient(config.Server, config.AdminUsername, config.AdminPassword, config.DefaultTeam, configPath)
		client.Config = config

		// Load the bulk import data to show what will be deleted
		data, err := client.LoadBulkImportData()
		if err != nil {
			mattermost.Log.WithFields(logrus.Fields{
				"error": err.Error(),
			}).Fatal("Failed to load bulk import data")
		}

		// Show confirmation prompt with summary counts
		fmt.Println("🚨 DESTRUCTIVE OPERATION WARNING 🚨")
		fmt.Println()
		fmt.Printf("This will PERMANENTLY DELETE the following from your Mattermost server:\n")
		fmt.Printf("  • %d teams\n", len(data.Teams))
		fmt.Printf("  • %d users\n", len(data.Users))
		fmt.Printf("  • All associated channels and data")
		fmt.Println()
		fmt.Println("⚠️  This operation is IRREVERSIBLE. All data will be permanently lost.")
		fmt.Println("⚠️  Make sure you have backups if you need to recover this data.")
		fmt.Println("⚠️  Check bulk_import.jsonl for the complete list of items to be deleted.")
		fmt.Println()
		
		// Prompt for confirmation
		fmt.Print("Type 'DELETE' to confirm this destructive operation: ")
		reader := bufio.NewReader(os.Stdin)
		input, err := reader.ReadString('\n')
		if err != nil {
			mattermost.Log.WithFields(logrus.Fields{
				"error": err.Error(),
			}).Fatal("Failed to read input")
		}
		
		input = strings.TrimSpace(input)
		if input != "DELETE" {
			fmt.Println("Operation cancelled. No data was deleted.")
			return
		}

		fmt.Println()
		fmt.Println("Proceeding with reset operation...")

		if err := client.Reset(); err != nil {
			mattermost.Log.WithFields(logrus.Fields{
				"error": err.Error(),
			}).Fatal("Reset failed")
		}
	},
}

func init() {
	RootCmd.AddCommand(resetCmd)
}