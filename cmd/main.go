package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func main() {
	// Root command
	rootCmd := &cobra.Command{
		Use:   "scripts",
		Short: "DevOps scripts for Mattermost and Keycloak",
		Long:  `A collection of utility scripts for managing Mattermost and Keycloak deployments.`,
	}

	// General command
	generalCmd := &cobra.Command{
		Use:   "general",
		Short: "General utility commands",
		Long:  `Commands for general system information and utilities.`,
	}
	rootCmd.AddCommand(generalCmd)

	// General subcommands
	loginsCmd := &cobra.Command{
		Use:   "logins",
		Short: "Display login information for services",
		Long:  `Display login information for Mattermost, Keycloak, Grafana, and other services.`,
		Run: func(cmd *cobra.Command, args []string) {
			general.Logins()
		},
	}
	generalCmd.AddCommand(loginsCmd)

	// Keycloak command
	keycloakCmd := &cobra.Command{
		Use:   "keycloak",
		Short: "Keycloak management commands",
		Long:  `Commands for managing Keycloak backups and restoration.`,
	}
	rootCmd.AddCommand(keycloakCmd)

	// Keycloak subcommands
	restoreCmd := &cobra.Command{
		Use:   "restore",
		Short: "Restore Keycloak from backup",
		Long:  `Restore Keycloak data from a backup file if the data directory doesn't exist.`,
		Run: func(cmd *cobra.Command, args []string) {
			keycloak.Restore()
		},
	}
	keycloakCmd.AddCommand(restoreCmd)

	backupCmd := &cobra.Command{
		Use:   "backup",
		Short: "Backup Keycloak data",
		Long:  `Create a backup of the Keycloak data directory.`,
		Run: func(cmd *cobra.Command, args []string) {
			keycloak.Backup()
		},
	}
	keycloakCmd.AddCommand(backupCmd)

	// Mattermost command
	mattermostCmd := &cobra.Command{
		Use:   "mattermost",
		Short: "Mattermost management commands",
		Long:  `Commands for managing Mattermost setup and configuration.`,
	}
	rootCmd.AddCommand(mattermostCmd)

	// Mattermost subcommands
	setupCmd := &cobra.Command{
		Use:   "setup",
		Short: "Set up Mattermost with test data",
		Long:  `Configure Mattermost with test users, teams, and webhooks.`,
		Run: func(cmd *cobra.Command, args []string) {
			mattermost.Setup()
		},
	}
	mattermostCmd.AddCommand(setupCmd)

	echoLoginsCmd := &cobra.Command{
		Use:   "echologins",
		Short: "Display Mattermost login information",
		Long:  `Display login credentials for Mattermost users.`,
		Run: func(cmd *cobra.Command, args []string) {
			mattermost.EchoLogins()
		},
	}
	mattermostCmd.AddCommand(echoLoginsCmd)

	// Execute the root command
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
