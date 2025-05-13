package cmd

import (
	"github.com/spf13/cobra"
)

// GeneralCmd represents the general command
var GeneralCmd = &cobra.Command{
	Use:   "general",
	Short: "General utility commands",
	Long:  `Commands for general system information and utilities.`,
}

// LoginsCmd displays login information for services
var LoginsCmd = &cobra.Command{
	Use:   "logins",
	Short: "Display login information for services",
	Long:  `Display login information for Mattermost, Keycloak, Grafana, and other services.`,
	Run: func(cmd *cobra.Command, args []string) {
		Logins()
	},
}

func init() {
	RootCmd.AddCommand(GeneralCmd)
	GeneralCmd.AddCommand(LoginsCmd)
}
