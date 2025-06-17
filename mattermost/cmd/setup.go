package cmd

import (
	"log"

	mmsetup "github.com/coltoneshaw/demokit/mattermost"
	"github.com/spf13/cobra"
)

var (
	useBulkImport     bool
	reinstallPlugins  string
	checkUpdates      bool
)

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

Plugin Options:
  --reinstall-plugins local   Rebuild and redeploy custom local plugins only
  --reinstall-plugins all     Rebuild all plugins and redeploy everything
  --check-updates             Check for and install newer plugin versions from GitHub

Use --bulk-import to use the original single-phase bulk import instead.`,
	Run: func(cmd *cobra.Command, args []string) {
		client := mmsetup.NewClient(serverURL, adminUser, adminPass, teamName, configPath)

		// Validate reinstall-plugins option
		if reinstallPlugins != "" && reinstallPlugins != "local" && reinstallPlugins != "all" {
			log.Fatalf("Invalid reinstall-plugins option: %s. Valid options: 'local', 'all'", reinstallPlugins)
		}

		forcePlugins := reinstallPlugins == "local" || reinstallPlugins == "all"
		forceGitHubPlugins := reinstallPlugins == "all"
		forceAll := false // Data import forcing would need a separate flag

		if useBulkImport {
			if err := client.SetupBulkWithForceAndUpdates(forcePlugins, forceGitHubPlugins, forceAll, checkUpdates); err != nil {
				log.Fatalf("Bulk setup failed: %v", err)
			}
		} else {
			if err := client.SetupWithForceAndUpdates(forcePlugins, forceGitHubPlugins, forceAll, checkUpdates); err != nil {
				log.Fatalf("Setup failed: %v", err)
			}
		}
	},
}

func init() {
	RootCmd.AddCommand(setupCmd)
	
	// Add the bulk import flag
	setupCmd.Flags().BoolVar(&useBulkImport, "bulk-import", false, "Use bulk import API instead of individual API calls")
	
	// Add the reinstall-plugins flag
	setupCmd.Flags().StringVar(&reinstallPlugins, "reinstall-plugins", "", "Plugin reinstall options: 'local' (rebuild custom plugins only), 'all' (rebuild all plugins)")
	
	// Add the check-updates flag
	setupCmd.Flags().BoolVar(&checkUpdates, "check-updates", false, "Check for and install newer plugin versions from GitHub")
}
