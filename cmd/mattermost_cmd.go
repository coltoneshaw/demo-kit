package cmd

import (
	"github.com/spf13/cobra"
)

// MattermostCmd represents the mattermost command
var MattermostCmd = &cobra.Command{
	Use:   "mattermost",
	Short: "Mattermost management commands",
	Long:  `Commands for managing Mattermost setup and configuration.`,
}

// SetupCmd sets up Mattermost with test data
var SetupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Set up Mattermost with test data",
	Long:  `Configure Mattermost with test users, teams, and webhooks.`,
	Run: func(cmd *cobra.Command, args []string) {
		Setup()
	},
}

// EchoLoginsCmd displays Mattermost login information
var EchoLoginsCmd = &cobra.Command{
	Use:   "echologins",
	Short: "Display Mattermost login information",
	Long:  `Display login credentials for Mattermost users.`,
	Run: func(cmd *cobra.Command, args []string) {
		EchoLogins()
	},
}

func init() {
	RootCmd.AddCommand(MattermostCmd)
	MattermostCmd.AddCommand(SetupCmd)
	MattermostCmd.AddCommand(EchoLoginsCmd)
}
