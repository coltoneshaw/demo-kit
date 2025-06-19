// Package mattermost provides tools for setting up and configuring a Mattermost server
package mattermost

import (
	// Third party imports
	"github.com/mattermost/mattermost/server/public/model"
)

// Constants for configuration and behaviors
const (
	// MaxWaitSeconds is the maximum time to wait for server startup
	MaxWaitSeconds = 120

	DefaultSiteURL = "http://localhost:8065"

	// Default admin user credentials
	DefaultAdminUsername = "sysadmin"
	DefaultAdminPassword = "Sys@dmin-sample1"
)

// Client represents a Mattermost API client with configuration for managing
// authentication, team contexts, and integration endpoints.
type Client struct {
	// API is the Mattermost API client from the official SDK
	API *model.Client4

	// ServerURL is the base URL of the Mattermost server (e.g., http://localhost:8065)
	ServerURL string

	// AdminUser is the username for the system admin account
	AdminUser string

	// AdminPass is the password for the system admin account
	AdminPass string

	// TeamName is the name of the team to use for operations
	TeamName string

	// Config is the loaded configuration from config.json
	Config *Config

	// ConfigPath is the path to the configuration file
	ConfigPath string

	// Plugin manager for plugin operations
	PluginManager *PluginManager

	// BulkImportPath is the path to the bulk import JSONL file
	BulkImportPath string
}


// NewClient creates a new Mattermost client with the specified connection parameters.
// It initializes the API client for integrated applications.
//
// Parameters:
//   - serverURL: The base URL of the Mattermost server (e.g., http://localhost:8065)
//   - adminUser: Username of the system administrator account
//   - adminPass: Password for the system administrator account
//   - teamName: Default team name to use for operations
//   - configPath: Optional path to the config.json file (pass "" for default)
//
// Returns a configured Client ready to connect to the Mattermost server.
func NewClient(serverURL, adminUser, adminPass, teamName string, configPath string) *Client {
	client := &Client{
		API:            model.NewAPIv4Client(serverURL),
		ServerURL:      serverURL,
		AdminUser:      adminUser,
		AdminPass:      adminPass,
		TeamName:       teamName,
		ConfigPath:     configPath,
		BulkImportPath: "bulk_import.jsonl",
	}

	// Initialize plugin manager
	client.PluginManager = NewPluginManager(client)

	return client
}
