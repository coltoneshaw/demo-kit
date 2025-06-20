package cmd

import (
	"github.com/coltoneshaw/demokit/mattermost"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// echoLoginsCmd represents the echo-logins command
var echoLoginsCmd = &cobra.Command{
	Use:   "echo-logins",
	Short: "Display login information",
	Long: `Display login information for users that have been configured in the system.

This command shows the usernames and passwords for:
- System admin user
- Users defined in the config file
- Default LDAP/SAML accounts`,
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
		client.EchoLogins()
	},
}

func init() {
	RootCmd.AddCommand(echoLoginsCmd)
}