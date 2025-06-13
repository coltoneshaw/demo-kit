package cmd

import (
	"github.com/coltoneshaw/demokit/mattermost"
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
		client := mattermost.NewClient(serverURL, adminUser, adminPass, teamName, configPath)
		client.EchoLogins()
	},
}

func init() {
	RootCmd.AddCommand(echoLoginsCmd)
}