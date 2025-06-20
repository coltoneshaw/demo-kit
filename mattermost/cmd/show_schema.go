package cmd

import (
	"github.com/coltoneshaw/demokit/mattermost"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// showSchemaCmd represents the show-schema command
var showSchemaCmd = &cobra.Command{
	Use:   "show-schema",
	Short: "Display LDAP schema extensions for custom attributes",
	Long: `Display the LDAP schema extensions that would be applied for custom attributes.

This command analyzes the bulk_import.jsonl file and shows:
- Custom attribute definitions with their LDAP mappings
- Generated OIDs for each custom attribute
- LDIF content for schema extensions
- Auxiliary object class definitions

This is useful for understanding how custom attributes are mapped to LDAP
and what schema modifications would be needed in a production environment.`,
	Run: func(cmd *cobra.Command, args []string) {
		// Load the config first
		config, err := mattermost.LoadConfig(configPath)
		if err != nil {
			mattermost.Log.WithFields(logrus.Fields{
				"error": err.Error(),
				"path":  configPath,
			}).Fatal("Failed to load config file")
		}

		// Create client using config values
		client := mattermost.NewClient(config.Server, config.AdminUsername, config.AdminPassword, config.DefaultTeam, configPath)
		client.Config = config
		
		if err := client.ShowLDAPSchemaExtensions(); err != nil {
			mattermost.Log.WithFields(logrus.Fields{
				"error": err.Error(),
			}).Fatal("❌ Failed to show LDAP schema extensions")
		}
	},
}

func init() {
	RootCmd.AddCommand(showSchemaCmd)
}