// Package mattermost provides tools for setting up and configuring a Mattermost server
package mattermost

import (
	// Standard library imports
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

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
}

// LogOptions contains options for controlling log output
type LogOptions struct {
	Suppress bool   // Suppress all output
	Prefix   string // Prefix to add to all log messages
}

// handleAPIError creates a formatted error from API responses.
// This standardizes error handling across API calls.
func handleAPIError(operation string, err error, resp *model.Response) error {
	if err != nil {
		if resp != nil {
			return fmt.Errorf("%s: %w (status code: %v)", operation, err, resp.StatusCode)
		}
		return fmt.Errorf("%s: %w", operation, err)
	}
	return nil
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
		API:        model.NewAPIv4Client(serverURL),
		ServerURL:  serverURL,
		AdminUser:  adminUser,
		AdminPass:  adminPass,
		TeamName:   teamName,
		ConfigPath: configPath,
	}

	// Initialize plugin manager
	client.PluginManager = NewPluginManager(client)

	return client
}

// Login authenticates with the Mattermost server
func (c *Client) Login() error {
	user, resp, err := c.API.Login(context.Background(), c.AdminUser, c.AdminPass)
	if err != nil {
		return handleAPIError(fmt.Sprintf("login failed for user '%s' with password '%s'", c.AdminUser, c.AdminPass), err, resp)
	}

	// Ensure the logged-in user has admin privileges
	if !strings.Contains(user.Roles, "system_admin") {
		// Use UpdateUserRoles API to directly assign system_admin role
		_, err := c.API.UpdateUserRoles(context.Background(), user.Id, "system_admin system_user")
		if err != nil {
			return fmt.Errorf("failed to assign system_admin role to user '%s': %w", c.AdminUser, err)
		}
		fmt.Printf("✅ Assigned system_admin role to user '%s'\n", c.AdminUser)
	}

	return nil
}

// CheckLicense verifies that the Mattermost server has a valid license
func (c *Client) CheckLicense() error {
	// Try to get license information to verify it's valid
	license, resp, err := c.API.GetOldClientLicense(context.Background(), "")
	if err != nil || (resp != nil && resp.StatusCode != 200) {
		return handleAPIError("failed to get license", err, resp)
	}

	if license == nil {
		return fmt.Errorf("❌ No valid license found on the server")
	}

	// Check if the server is licensed
	isLicensed, exists := license["IsLicensed"]
	if !exists || isLicensed != "true" {
		return fmt.Errorf("❌ Mattermost server is not licensed. This setup tool requires a licensed Mattermost Enterprise server (IsLicensed: %s)", isLicensed)
	}

	// Get license ID for confirmation
	licenseId, hasId := license["Id"]
	if hasId {
		fmt.Printf("✅ Server is licensed (ID: %s)\n", licenseId)
	} else {
		fmt.Println("✅ Server is licensed")
	}

	return nil
}

// WaitForStart polls the Mattermost server until it responds or times out.
// It sends periodic ping requests to check if the server is ready to accept connections.
//
// This method will wait up to MaxWaitSeconds (default: 120) before timing out.
// During the wait, it prints a dot every second to indicate progress.
//
// Returns nil if the server starts successfully, or an error if the timeout is reached.
func (c *Client) WaitForStart() error {
	// Progress indicators
	progressChars := []string{"-", "\\", "|", "/"}

	for i := 0; i < MaxWaitSeconds; i++ {
		// Show a spinning progress indicator
		progressChar := progressChars[i%len(progressChars)]
		fmt.Printf("\r[%s] Checking Mattermost API status... (%d/%d seconds)",
			progressChar, i+1, MaxWaitSeconds)

		// Send a ping request
		_, resp, err := c.API.GetPing(context.Background())
		if err == nil && resp != nil && resp.StatusCode == 200 {
			// Clear the progress line
			fmt.Print("\r                                                           \r")
			return nil
		}

		time.Sleep(1 * time.Second)
	}

	// Clear the progress line
	fmt.Print("\r                                                           \r")
	return fmt.Errorf("server didn't start in %d seconds", MaxWaitSeconds)
}

// categorizeChannelAPI implements channel categorization using the Playbooks API
func (c *Client) categorizeChannelAPI(channelID string, categoryName string) error {
	if channelID == "" || categoryName == "" {
		return fmt.Errorf("channel ID and category name are required")
	}

	fmt.Printf("Categorizing channel %s in category '%s' using Playbooks API...\n", channelID, categoryName)

	// Construct the URL for the categorize channel API
	url := fmt.Sprintf("%s/plugins/playbooks/api/v0/actions/channels/%s",
		c.ServerURL, channelID)

	// Create the payload
	type Category struct {
		CategoryName string `json:"category_name"`
	}

	type CategorizePayload struct {
		Enabled     bool     `json:"enabled"`
		Payload     Category `json:"payload"`
		ChannelID   string   `json:"channel_id"`
		ActionType  string   `json:"action_type"`
		TriggerType string   `json:"trigger_type"`
	}

	payload := CategorizePayload{
		Enabled: true,
		Payload: Category{
			CategoryName: categoryName,
		},
		ChannelID:   channelID,
		ActionType:  "categorize_channel",
		TriggerType: "new_member_joins",
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal categorize payload: %w", err)
	}

	// Create the request
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonPayload))
	if err != nil {
		return fmt.Errorf("failed to create categorize request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.API.AuthToken)

	// Send the request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send categorize request: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			fmt.Printf("Warning: failed to close response body: %v\n", closeErr)
		}
	}()

	// Check response
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("categorize request failed with status %d: %s", resp.StatusCode, string(body))
	}

	fmt.Printf("✅ Successfully categorized channel in '%s' using Playbooks API\n", categoryName)
	return nil
}


// SetupChannelCommands executes specified slash commands in channels sequentially
// If any command fails, the entire setup process will abort
func (c *Client) SetupChannelCommands() error {
	// Only proceed if we have a config
	if c.Config == nil || len(c.Config.Teams) == 0 {
		return nil
	}

	fmt.Println("Setting up channel commands...")

	// Get all teams
	teams, resp, err := c.API.GetAllTeams(context.Background(), "", 0, 100)
	if err != nil {
		return handleAPIError("failed to get teams", err, resp)
	}

	// Create a map of team names to team objects
	teamMap := make(map[string]*model.Team)
	for _, team := range teams {
		teamMap[team.Name] = team
	}

	// For each team in the config
	for teamName, teamConfig := range c.Config.Teams {
		team, exists := teamMap[teamName]
		if !exists {
			fmt.Printf("❌ Team '%s' not found, can't execute commands\n", teamName)
			return fmt.Errorf("team '%s' not found", teamName)
		}

		// Get all channels for this team
		channels, _, err := c.API.GetPublicChannelsForTeam(context.Background(), team.Id, 0, 1000, "")
		if err != nil {
			fmt.Printf("❌ Failed to get channels for team '%s': %v\n", teamName, err)
			return fmt.Errorf("failed to get channels for team '%s': %w", teamName, err)
		}

		// Also get private channels
		privateChannels, _, err := c.API.GetPrivateChannelsForTeam(context.Background(), team.Id, 0, 1000, "")
		if err == nil {
			channels = append(channels, privateChannels...)
		}

		// Create a map of channel names to channel objects
		channelMap := make(map[string]*model.Channel)
		for _, ch := range channels {
			channelMap[ch.Name] = ch
		}

		// For each channel with commands
		for _, channelConfig := range teamConfig.Channels {
			// Skip if no commands are configured
			if len(channelConfig.Commands) == 0 {
				continue
			}

			// Find the channel
			channel, exists := channelMap[channelConfig.Name]
			if !exists {
				fmt.Printf("❌ Channel '%s' not found in team '%s'\n", channelConfig.Name, teamName)
				return fmt.Errorf("channel '%s' not found in team '%s'", channelConfig.Name, teamName)
			}

			fmt.Printf("Executing %d commands for channel '%s' (sequentially)...\n",
				len(channelConfig.Commands), channelConfig.Name)

			// Execute each command in order, waiting for each to complete
			for i, command := range channelConfig.Commands {
				// Check if the command has been loaded and trimmed
				commandText := strings.TrimSpace(command)

				if !strings.HasPrefix(commandText, "/") {
					fmt.Printf("❌ Invalid command '%s' for channel '%s' - must start with /\n",
						commandText, channelConfig.Name)
					return fmt.Errorf("invalid command '%s' - must start with /", commandText)
				}

				// Remove the leading slash for the API

				fmt.Printf("Executing command %d/%d in channel '%s': %s\n",
					i+1, len(channelConfig.Commands), channelConfig.Name, commandText)

				// Execute the command using the commands/execute API
				_, resp, err := c.API.ExecuteCommand(context.Background(), channel.Id, commandText)

				// Check for any errors or non-200 response
				if err != nil {
					fmt.Printf("❌ Failed to execute command '%s': %v\n", commandText, err)
					return fmt.Errorf("failed to execute command '%s': %w", commandText, err)
				}

				if resp.StatusCode != 200 {
					fmt.Printf("❌ Command '%s' returned non-200 status code: %d\n",
						commandText, resp.StatusCode)
					return fmt.Errorf("command '%s' returned status code %d",
						commandText, resp.StatusCode)
				}

				fmt.Printf("✅ Successfully executed command %d/%d: '%s' in channel '%s'\n",
					i+1, len(channelConfig.Commands), commandText, channelConfig.Name)

				// Add a small delay between commands to ensure proper ordering
				time.Sleep(500 * time.Millisecond)
			}
		}
	}

	return nil
}

// PluginInfo represents information about a plugin
type PluginInfo struct {
	ID     string
	Name   string
	Path   string
	Built  bool
	Exists bool
}

// GetPluginInfo returns information about required plugins
func (c *Client) GetPluginInfo() []PluginInfo {
	return []PluginInfo{
		{
			ID:   "com.coltoneshaw.weather",
			Name: "Weather Plugin",
			Path: "../apps/weather-plugin",
		},
		{
			ID:   "com.coltoneshaw.flightaware",
			Name: "FlightAware Plugin",
			Path: "../apps/flightaware-plugin",
		},
		{
			ID:   "com.coltoneshaw.missionops",
			Name: "Mission Operations Plugin",
			Path: "../apps/missionops-plugin",
		},
	}
}

// IsPluginInstalled checks if a plugin is installed on the server
func (c *Client) IsPluginInstalled(pluginID string) (bool, error) {
	plugins, resp, err := c.API.GetPlugins(context.Background())
	if err != nil {
		return false, handleAPIError("failed to get plugins", err, resp)
	}

	// Check both active and inactive plugins
	for _, plugin := range plugins.Active {
		if plugin.Id == pluginID {
			return true, nil
		}
	}
	for _, plugin := range plugins.Inactive {
		if plugin.Id == pluginID {
			return true, nil
		}
	}

	return false, nil
}

// IsPluginBuilt checks if a plugin bundle already exists
func (c *Client) IsPluginBuilt(pluginPath string) bool {
	bundlePath, err := c.FindPluginBundle(pluginPath)
	if err != nil {
		return false
	}
	// Check if the bundle file actually exists
	_, err = os.Stat(bundlePath)
	return err == nil
}

// BuildPlugin builds a plugin from its source directory using make
func (c *Client) BuildPlugin(pluginPath string) error {
	// Check if the plugin directory exists
	if _, err := os.Stat(pluginPath); os.IsNotExist(err) {
		return fmt.Errorf("plugin directory does not exist: %s", pluginPath)
	}

	// Check if Makefile exists
	makefilePath := filepath.Join(pluginPath, "Makefile")
	if _, err := os.Stat(makefilePath); os.IsNotExist(err) {
		return fmt.Errorf("makefile not found in plugin directory: %s", pluginPath)
	}

	fmt.Printf("Building plugin in %s...\n", pluginPath)

	// Run make dist to build the plugin
	cmd := exec.Command("make", "dist")
	cmd.Dir = pluginPath
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to build plugin: %w", err)
	}

	fmt.Printf("✅ Plugin built successfully\n")
	return nil
}

// FindPluginBundle finds the built plugin bundle (.tar.gz) in the dist directory
func (c *Client) FindPluginBundle(pluginPath string) (string, error) {
	distPath := filepath.Join(pluginPath, "dist")

	// Check if dist directory exists
	if _, err := os.Stat(distPath); os.IsNotExist(err) {
		return "", fmt.Errorf("dist directory does not exist: %s", distPath)
	}

	// Find .tar.gz files in dist directory
	matches, err := filepath.Glob(filepath.Join(distPath, "*.tar.gz"))
	if err != nil {
		return "", fmt.Errorf("failed to search for plugin bundle: %w", err)
	}

	if len(matches) == 0 {
		return "", fmt.Errorf("no plugin bundle (.tar.gz) found in %s", distPath)
	}

	// Return the first match (should only be one)
	return matches[0], nil
}

// UploadPlugin uploads and installs a plugin to the Mattermost server
func (c *Client) UploadPlugin(bundlePath string) error {
	fmt.Printf("Uploading plugin bundle: %s\n", bundlePath)

	// Open the bundle file
	file, err := os.Open(bundlePath)
	if err != nil {
		return fmt.Errorf("failed to open plugin bundle: %w", err)
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			fmt.Printf("Warning: failed to close plugin bundle file: %v\n", closeErr)
		}
	}()

	fmt.Printf("Uploading with force flag (will overwrite existing plugin)\n")
	// Reset file position
	if _, seekErr := file.Seek(0, 0); seekErr != nil {
		return fmt.Errorf("❌ failed to reset file position: %w", seekErr)
	}
	manifest, resp, err := c.API.UploadPluginForced(context.Background(), file)
	if err != nil {
		return handleAPIError(fmt.Sprintf("failed to upload plugin bundle '%s': %v", bundlePath, err), err, resp)
	}
	fmt.Printf("✅ Plugin '%s' (ID: %s) uploaded successfully (forced)\n", manifest.Name, manifest.Id)

	// Enable the plugin
	enableResp, enableErr := c.API.EnablePlugin(context.Background(), manifest.Id)
	if enableErr != nil {
		return handleAPIError("failed to enable plugin", enableErr, enableResp)
	}

	fmt.Printf("✅ Plugin '%s' enabled successfully\n", manifest.Name)
	return nil
}

// Setup performs the main setup based on configuration using individual API calls
func (c *Client) Setup() error {
	// Safety check - make sure the client and API are properly initialized
	if c == nil || c.API == nil {
		return fmt.Errorf("client not properly initialized")
	}


	if err := c.WaitForStart(); err != nil {
		return err
	}

	if err := c.Login(); err != nil {
		return err
	}

	// Verify the server is licensed before proceeding with setup
	if err := c.CheckLicense(); err != nil {
		return err
	}

	// Download and install latest plugins (no config dependency)
	if err := c.PluginManager.SetupLatestPlugins(nil); err != nil {
		return fmt.Errorf("failed to setup plugins: %w", err)
	}

	// Use two-phase bulk import for users, teams, and channels
	if err := c.SetupWithSplitImport(); err != nil {
		return err
	}

	fmt.Println("Alright, everything seems to be setup and running. Enjoy.")
	return nil
}

// SetupBulk performs the main setup using bulk import instead of individual API calls
func (c *Client) SetupBulk() error {
	// Safety check - make sure the client and API are properly initialized
	if c == nil || c.API == nil {
		return fmt.Errorf("client not properly initialized")
	}

	if err := c.WaitForStart(); err != nil {
		return err
	}

	if err := c.Login(); err != nil {
		return err
	}

	// Verify the server is licensed before proceeding with setup
	if err := c.CheckLicense(); err != nil {
		return err
	}

	// Download and install latest plugins (no config dependency)
	if err := c.PluginManager.SetupLatestPlugins(nil); err != nil {
		return fmt.Errorf("failed to setup plugins: %w", err)
	}

	// Use two-phase bulk import for users, teams, and channels
	if err := c.SetupWithSplitImport(); err != nil {
		return err
	}

	fmt.Println("Alright, everything seems to be setup and running. Enjoy.")
	return nil
}

// BulkImportData represents the parsed data from bulk_import.jsonl
type BulkImportData struct {
	Teams []BulkTeam `json:"teams"`
	Users []BulkUser `json:"users"`
}

// BulkTeam represents a team from bulk import
type BulkTeam struct {
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
	Type        string `json:"type"`
	Description string `json:"description"`
}

// BulkUser represents a user from bulk import
type BulkUser struct {
	Username  string `json:"username"`
	Email     string `json:"email"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Nickname  string `json:"nickname"`
	Position  string `json:"position"`
}

// ResetImportLine represents a single line in the bulk import JSONL file for reset operations
type ResetImportLine struct {
	Type string `json:"type"`
	Team *struct {
		Name        string `json:"name"`
		DisplayName string `json:"display_name"`
		Type        string `json:"type"`
		Description string `json:"description"`
	} `json:"team,omitempty"`
	User *struct {
		Username  string `json:"username"`
		Email     string `json:"email"`
		FirstName string `json:"first_name"`
		LastName  string `json:"last_name"`
		Nickname  string `json:"nickname"`
		Position  string `json:"position"`
	} `json:"user,omitempty"`
}

// LoadBulkImportData loads and parses the bulk_import.jsonl file
func (c *Client) LoadBulkImportData() (*BulkImportData, error) {
	// Try multiple possible locations for the bulk import file
	possiblePaths := []string{
		"bulk_import.jsonl",          // Current directory (when run from root)
		"../bulk_import.jsonl",       // Parent directory (when run from mattermost/)
	}
	
	var bulkImportPath string
	for _, path := range possiblePaths {
		if _, err := os.Stat(path); err == nil {
			bulkImportPath = path
			break
		}
	}
	
	if bulkImportPath == "" {
		return nil, fmt.Errorf("bulk_import.jsonl not found. Tried: %v", possiblePaths)
	}

	file, err := os.Open(bulkImportPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open bulk import file: %w", err)
	}
	defer file.Close()

	var teams []BulkTeam
	var users []BulkUser

	// Read the JSONL file line by line
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}

		var importLine ResetImportLine
		if err := json.Unmarshal([]byte(line), &importLine); err != nil {
			fmt.Printf("Warning: Failed to parse line: %s\n", line)
			continue
		}

		switch importLine.Type {
		case "team":
			if importLine.Team != nil {
				teams = append(teams, BulkTeam{
					Name:        importLine.Team.Name,
					DisplayName: importLine.Team.DisplayName,
					Type:        importLine.Team.Type,
					Description: importLine.Team.Description,
				})
			}
		case "user":
			if importLine.User != nil {
				users = append(users, BulkUser{
					Username:  importLine.User.Username,
					Email:     importLine.User.Email,
					FirstName: importLine.User.FirstName,
					LastName:  importLine.User.LastName,
					Nickname:  importLine.User.Nickname,
					Position:  importLine.User.Position,
				})
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading bulk import file: %w", err)
	}

	fmt.Printf("Loaded %d teams and %d users from bulk import\n", len(teams), len(users))

	return &BulkImportData{
		Teams: teams,
		Users: users,
	}, nil
}

// DeleteBulkUsers permanently deletes users from bulk import data
func (c *Client) DeleteBulkUsers(users []BulkUser) error {
	if len(users) == 0 {
		fmt.Println("No users found in bulk import to delete")
		return nil
	}

	fmt.Printf("Deleting %d users from bulk import...\n", len(users))

	for _, userInfo := range users {
		// Find the user by username
		user, resp, err := c.API.GetUserByUsername(context.Background(), userInfo.Username, "")
		if err != nil {
			if resp != nil && resp.StatusCode == 404 {
				fmt.Printf("⚠️  User '%s' not found, skipping\n", userInfo.Username)
				continue
			}
			return handleAPIError(fmt.Sprintf("failed to find user '%s'", userInfo.Username), err, resp)
		}

		// Permanently delete the user
		_, err = c.API.PermanentDeleteUser(context.Background(), user.Id)
		if err != nil {
			return handleAPIError(fmt.Sprintf("failed to delete user '%s'", userInfo.Username), err, nil)
		}

		fmt.Printf("✅ Permanently deleted user '%s'\n", userInfo.Username)
	}

	return nil
}

// DeleteBulkTeams permanently deletes teams from bulk import data
func (c *Client) DeleteBulkTeams(teams []BulkTeam) error {
	if len(teams) == 0 {
		fmt.Println("No teams found in bulk import to delete")
		return nil
	}

	fmt.Printf("Deleting %d teams from bulk import...\n", len(teams))

	for _, teamInfo := range teams {
		// Find the team by name
		team, resp, err := c.API.GetTeamByName(context.Background(), teamInfo.Name, "")
		if err != nil {
			if resp != nil && resp.StatusCode == 404 {
				fmt.Printf("⚠️  Team '%s' not found, skipping\n", teamInfo.Name)
				continue
			}
			return handleAPIError(fmt.Sprintf("failed to find team '%s'", teamInfo.Name), err, resp)
		}

		// Permanently delete the team
		_, err = c.API.PermanentDeleteTeam(context.Background(), team.Id)
		if err != nil {
			return handleAPIError(fmt.Sprintf("failed to delete team '%s'", teamInfo.Name), err, nil)
		}

		fmt.Printf("✅ Permanently deleted team '%s'\n", teamInfo.Name)
	}

	return nil
}

// CheckDeletionSettings verifies that the server has deletion APIs enabled
func (c *Client) CheckDeletionSettings() error {
	config, resp, err := c.API.GetConfig(context.Background())
	if err != nil {
		return handleAPIError("failed to get server config", err, resp)
	}

	// Check EnableAPIUserDeletion
	if config.ServiceSettings.EnableAPIUserDeletion == nil || !*config.ServiceSettings.EnableAPIUserDeletion {
		return fmt.Errorf("ServiceSettings.EnableAPIUserDeletion is not enabled. Please enable it in the server configuration to use the reset command")
	}

	// Check EnableAPITeamDeletion
	if config.ServiceSettings.EnableAPITeamDeletion == nil || !*config.ServiceSettings.EnableAPITeamDeletion {
		return fmt.Errorf("ServiceSettings.EnableAPITeamDeletion is not enabled. Please enable it in the server configuration to use the reset command")
	}

	fmt.Println("✅ API deletion settings are enabled")
	return nil
}

// Reset permanently deletes all teams and users from the bulk import file
func (c *Client) Reset() error {
	// Safety check - make sure the client and API are properly initialized
	if c == nil || c.API == nil {
		return fmt.Errorf("client not properly initialized")
	}

	if err := c.WaitForStart(); err != nil {
		return err
	}

	if err := c.Login(); err != nil {
		return err
	}

	// Load bulk import data
	bulkData, err := c.LoadBulkImportData()
	if err != nil {
		return fmt.Errorf("failed to load bulk import data: %w", err)
	}

	// Check that deletion APIs are enabled
	if err := c.CheckDeletionSettings(); err != nil {
		return err
	}

	fmt.Println("🚨 WARNING: This will permanently delete all teams and users from the bulk import file.")
	fmt.Println("This operation is irreversible.")

	// Delete users first (they need to be removed from teams before teams can be deleted)
	if err := c.DeleteBulkUsers(bulkData.Users); err != nil {
		return fmt.Errorf("failed to delete users: %w", err)
	}

	// Then delete teams
	if err := c.DeleteBulkTeams(bulkData.Teams); err != nil {
		return fmt.Errorf("failed to delete teams: %w", err)
	}

	fmt.Println("✅ Reset completed successfully")
	return nil
}

// DeleteConfigUsers permanently deletes all users from the configuration
func (c *Client) DeleteConfigUsers() error {
	if c.Config == nil || len(c.Config.Users) == 0 {
		fmt.Println("No users found in configuration to delete")
		return nil
	}

	fmt.Printf("Deleting %d users from configuration...\n", len(c.Config.Users))

	for _, userConfig := range c.Config.Users {
		// Find the user by username
		user, resp, err := c.API.GetUserByUsername(context.Background(), userConfig.Username, "")
		if err != nil {
			if resp != nil && resp.StatusCode == 404 {
				fmt.Printf("⚠️  User '%s' not found, skipping\n", userConfig.Username)
				continue
			}
			return handleAPIError(fmt.Sprintf("failed to find user '%s'", userConfig.Username), err, resp)
		}

		// Permanently delete the user
		_, err = c.API.PermanentDeleteUser(context.Background(), user.Id)
		if err != nil {
			return handleAPIError(fmt.Sprintf("failed to delete user '%s'", userConfig.Username), err, nil)
		}

		fmt.Printf("✅ Permanently deleted user '%s'\n", userConfig.Username)
	}

	return nil
}

// DeleteConfigTeams permanently deletes all teams from the configuration
func (c *Client) DeleteConfigTeams() error {
	if c.Config == nil || len(c.Config.Teams) == 0 {
		fmt.Println("No teams found in configuration to delete")
		return nil
	}

	fmt.Printf("Deleting %d teams from configuration...\n", len(c.Config.Teams))

	for teamName := range c.Config.Teams {
		// Find the team by name
		team, resp, err := c.API.GetTeamByName(context.Background(), teamName, "")
		if err != nil {
			if resp != nil && resp.StatusCode == 404 {
				fmt.Printf("⚠️  Team '%s' not found, skipping\n", teamName)
				continue
			}
			return handleAPIError(fmt.Sprintf("failed to find team '%s'", teamName), err, resp)
		}

		// Permanently delete the team
		_, err = c.API.PermanentDeleteTeam(context.Background(), team.Id)
		if err != nil {
			return handleAPIError(fmt.Sprintf("failed to delete team '%s'", teamName), err, nil)
		}

		fmt.Printf("✅ Permanently deleted team '%s'\n", teamName)
	}

	return nil
}

// EchoLogins prints login information - always shown regardless of test mode
func (c *Client) EchoLogins() {
	fmt.Println("===========================================")
	fmt.Println("Mattermost logins")
	fmt.Println("===========================================")

	fmt.Println("- System admin")
	fmt.Printf("     - username: %s\n", DefaultAdminUsername)
	fmt.Printf("     - password: %s\n", DefaultAdminPassword)

	// If we have configuration users, display them
	if c.Config != nil && len(c.Config.Users) > 0 {
		fmt.Println("- Config users:")
		for _, user := range c.Config.Users {
			fmt.Printf("     - username: %s\n", user.Username)
			fmt.Printf("     - password: %s\n", user.Password)
		}
	}

	fmt.Println("- LDAP or SAML account:")
	fmt.Println("     - username: professor")
	fmt.Println("     - password: professor")
	fmt.Println()
	fmt.Println("For more logins check out https://github.com/coltoneshaw/mattermost#accounts")
	fmt.Println()
	fmt.Println("===========================================")
}
