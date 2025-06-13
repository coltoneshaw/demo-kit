package cmd

import (
	"log"

	mmsetup "github.com/coltoneshaw/demokit/mattermost"
	"github.com/spf13/cobra"
)

// setupCmd represents the setup command
var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Setup users, teams, channels, and slash commands",
	Long: `Setup users, teams, channels, and slash commands on a Mattermost server.

This command will:
- Create users defined in the config file
- Create teams and channels 
- Add users to teams and channels
- Create slash commands
- Execute any configured channel commands

The setup process uses the connection flags to connect to the target Mattermost server.`,
	Run: func(cmd *cobra.Command, args []string) {
		client := mmsetup.NewClient(serverURL, adminUser, adminPass, teamName, configPath)

		if err := client.Setup(); err != nil {
			log.Fatalf("Setup failed: %v", err)
		}
	},
}

func init() {
	RootCmd.AddCommand(setupCmd)
}
