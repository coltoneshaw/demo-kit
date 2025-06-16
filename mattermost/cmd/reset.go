package cmd

import (
	"log"

	mmsetup "github.com/coltoneshaw/demokit/mattermost"
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
		client := mmsetup.NewClient(serverURL, adminUser, adminPass, teamName, configPath)

		if err := client.Reset(); err != nil {
			log.Fatalf("Reset failed: %v", err)
		}
	},
}

func init() {
	RootCmd.AddCommand(resetCmd)
}