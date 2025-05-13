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
	RunE: func(cmd *cobra.Command, args []string) error {
		return SetupCmdF()
	},
}

// EchoLoginsCmd displays Mattermost login information
var EchoLoginsCmd = &cobra.Command{
	Use:   "echologins",
	Short: "Display Mattermost login information",
	Long:  `Display login credentials for Mattermost users.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return EchoLoginsCmdF()
	},
}

func init() {
	RootCmd.AddCommand(MattermostCmd)
	MattermostCmd.AddCommand(SetupCmd)
	MattermostCmd.AddCommand(EchoLoginsCmd)
}
