package cmd

import (
	"github.com/spf13/cobra"
)

// KeycloakCmd represents the keycloak command
var KeycloakCmd = &cobra.Command{
	Use:   "keycloak",
	Short: "Keycloak management commands",
	Long:  `Commands for managing Keycloak backups and restoration.`,
}

// RestoreCmd restores Keycloak from backup
var RestoreCmd = &cobra.Command{
	Use:   "restore",
	Short: "Restore Keycloak from backup",
	Long:  `Restore Keycloak data from a backup file if the data directory doesn't exist.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return RestoreCmdF()
	},
}

// BackupCmd backs up Keycloak data
var BackupCmd = &cobra.Command{
	Use:   "backup",
	Short: "Backup Keycloak data",
	Long:  `Create a backup of the Keycloak data directory.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return BackupCmdF()
	},
}

func init() {
	RootCmd.AddCommand(KeycloakCmd)
	KeycloakCmd.AddCommand(RestoreCmd)
	KeycloakCmd.AddCommand(BackupCmd)
}
