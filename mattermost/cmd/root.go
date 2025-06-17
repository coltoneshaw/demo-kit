package cmd

import (
	"fmt"
	"os"

	"github.com/coltoneshaw/demokit/mattermost"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var (
	// Global flags
	serverURL  string
	adminUser  string
	adminPass  string
	teamName   string
	configPath string
	
	// Logging flags
	verbose   bool
	logLevel  string
	logFormat string
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
		mattermost.Log.WithFields(logrus.Fields{
			"error": err.Error(),
		}).Error("Command execution failed")
		os.Exit(1)
	}
}

func init() {
	// Initialize logger before any commands run
	cobra.OnInitialize(initLogger)
	
	// Global flags available to all commands
	RootCmd.PersistentFlags().StringVar(&serverURL, "server", mattermost.DefaultSiteURL, "Mattermost server URL")
	RootCmd.PersistentFlags().StringVar(&adminUser, "admin", mattermost.DefaultAdminUsername, "Admin username")
	RootCmd.PersistentFlags().StringVar(&adminPass, "password", mattermost.DefaultAdminPassword, "Admin password")
	RootCmd.PersistentFlags().StringVar(&teamName, "team", "test", "Default team name")
	RootCmd.PersistentFlags().StringVar(&configPath, "config", "", "Path to config.json file")
	
	// Logging flags
	RootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose logging with timestamps")
	RootCmd.PersistentFlags().StringVar(&logLevel, "log-level", "info", "Log level (debug, info, warn, error)")
	RootCmd.PersistentFlags().StringVar(&logFormat, "log-format", "text", "Log format (text, json)")
}

// initLogger initializes the logger based on command-line flags
func initLogger() {
	level, err := logrus.ParseLevel(logLevel)
	if err != nil {
		fmt.Printf("Invalid log level '%s', using info\n", logLevel)
		level = logrus.InfoLevel
	}
	
	mattermost.InitLogger(&mattermost.LogConfig{
		Level:   level,
		Format:  logFormat,
		Verbose: verbose,
	})
}