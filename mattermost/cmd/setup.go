package cmd

import (
	"github.com/coltoneshaw/demokit/mattermost"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var (
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
It uses a two-phase bulk import system (infrastructure first, then users) for optimal reliability.

Plugin Options:
  --reinstall-plugins local   Rebuild and redeploy custom local plugins only
  --reinstall-plugins all     Rebuild all plugins and redeploy everything
  --check-updates             Check for and install newer plugin versions from GitHub`,
	Run: func(cmd *cobra.Command, args []string) {
		client := mattermost.NewClient(serverURL, adminUser, adminPass, teamName, configPath)

		// Validate reinstall-plugins option
		if reinstallPlugins != "" && reinstallPlugins != "local" && reinstallPlugins != "all" {
			mattermost.Log.WithFields(logrus.Fields{
				"provided": reinstallPlugins,
				"valid_options": []string{"local", "all"},
			}).Fatal("Invalid reinstall-plugins option")
		}

		forcePlugins := reinstallPlugins == "local" || reinstallPlugins == "all"
		forceGitHubPlugins := reinstallPlugins == "all"
		forceAll := false // Data import forcing would need a separate flag

		if err := client.SetupWithForceAndUpdates(forcePlugins, forceGitHubPlugins, forceAll, checkUpdates); err != nil {
			mattermost.Log.WithFields(logrus.Fields{
				"error": err.Error(),
			}).Fatal("Setup failed")
		}
	},
}

func init() {
	RootCmd.AddCommand(setupCmd)
	
	// Add the reinstall-plugins flag
	setupCmd.Flags().StringVar(&reinstallPlugins, "reinstall-plugins", "", "Plugin reinstall options: 'local' (rebuild custom plugins only), 'all' (rebuild all plugins)")
	
	// Add the check-updates flag
	setupCmd.Flags().BoolVar(&checkUpdates, "check-updates", false, "Check for and install newer plugin versions from GitHub")
}
