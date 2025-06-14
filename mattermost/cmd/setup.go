package cmd

import (
	"log"

	mmsetup "github.com/coltoneshaw/demokit/mattermost"
	"github.com/spf13/cobra"
)

var useBulkImport bool

// setupCmd represents the setup command
var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Setup users, teams, channels, and slash commands",
	Long: `Setup users, teams, channels, and slash commands on a Mattermost server.

This command will:
- Create teams and channels from bulk_import.jsonl
- Create users and add them to teams/channels
- Execute any configured channel commands

The setup process uses the connection flags to connect to the target Mattermost server.
By default, it uses a two-phase bulk import (infrastructure first, then users).

Use --bulk-import to use the original single-phase bulk import instead.`,
	Run: func(cmd *cobra.Command, args []string) {
		client := mmsetup.NewClient(serverURL, adminUser, adminPass, teamName, configPath)

		if useBulkImport {
			if err := client.SetupBulk(); err != nil {
				log.Fatalf("Bulk setup failed: %v", err)
			}
		} else {
			if err := client.Setup(); err != nil {
				log.Fatalf("Setup failed: %v", err)
			}
		}
	},
}

func init() {
	RootCmd.AddCommand(setupCmd)
	
	// Add the bulk import flag
	setupCmd.Flags().BoolVar(&useBulkImport, "bulk-import", false, "Use bulk import API instead of individual API calls")
}
