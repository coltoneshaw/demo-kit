package cmd

import (
	"fmt"
	"os"

	"github.com/coltoneshaw/demokit/mattermost"
	"github.com/spf13/cobra"
)

var (
	// Global flags
	serverURL  string
	adminUser  string
	adminPass  string
	teamName   string
	configPath string
)

// RootCmd represents the base command when called without any subcommands
var RootCmd = &cobra.Command{
	Use:   "mmsetup",
	Short: "Mattermost Setup Tool - Works with any Mattermost server",
	Long: `A tool for setting up users, teams, channels, and slash commands on any Mattermost server.

This tool can connect to any Mattermost instance using the connection flags and 
automatically configure it based on a JSON configuration file.`,
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	RootCmd.CompletionOptions.HiddenDefaultCmd = true

	if err := RootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func init() {
	// Global flags available to all commands
	RootCmd.PersistentFlags().StringVar(&serverURL, "server", mattermost.DefaultSiteURL, "Mattermost server URL")
	RootCmd.PersistentFlags().StringVar(&adminUser, "admin", mattermost.DefaultAdminUsername, "Admin username")
	RootCmd.PersistentFlags().StringVar(&adminPass, "password", mattermost.DefaultAdminPassword, "Admin password")
	RootCmd.PersistentFlags().StringVar(&teamName, "team", "test", "Default team name")
	RootCmd.PersistentFlags().StringVar(&configPath, "config", "", "Path to config.json file")
}