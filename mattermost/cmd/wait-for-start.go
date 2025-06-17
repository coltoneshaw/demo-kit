package cmd

import (
	"github.com/coltoneshaw/demokit/mattermost"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// waitForStartCmd represents the wait-for-start command
var waitForStartCmd = &cobra.Command{
	Use:   "wait-for-start",
	Short: "Wait for Mattermost server to start",
	Long: `Wait for the Mattermost server to start and respond to API requests.

This command polls the Mattermost server's ping endpoint until it responds 
successfully or times out. Useful for automation scripts that need to wait
for the server to be ready before proceeding.`,
	Run: func(cmd *cobra.Command, args []string) {
		client := mattermost.NewClient(serverURL, adminUser, adminPass, teamName, configPath)

		if err := client.WaitForStart(); err != nil {
			mattermost.Log.WithFields(logrus.Fields{
				"error": err.Error(),
				"server": serverURL,
			}).Fatal("Failed to connect to Mattermost")
		}
		mattermost.Log.Info("✅ Mattermost API is responding successfully")
	},
}

func init() {
	RootCmd.AddCommand(waitForStartCmd)
}
