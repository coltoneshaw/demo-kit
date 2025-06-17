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
	"github.com/sirupsen/logrus"
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
		Log.WithFields(logrus.Fields{"user_name": c.AdminUser}).Info("✅ Assigned system_admin role to user")
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
		Log.WithFields(logrus.Fields{"license_id": licenseId}).Info("✅ Server is licensed")
	} else {
		Log.Info("✅ Server is licensed")
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
	Log.Info("🚀 Waiting for Mattermost server to start...")
	
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
			Log.Info("✅ Mattermost server is ready")
			return nil
		}

		time.Sleep(1 * time.Second)
	}

	// Clear the progress line
	fmt.Print("\r                                                           \r")
	Log.WithFields(logrus.Fields{"timeout_seconds": MaxWaitSeconds}).Error("❌ Server didn't start within timeout")
	return fmt.Errorf("server didn't start in %d seconds", MaxWaitSeconds)
}

// categorizeChannelAPI implements channel categorization using the Playbooks API
func (c *Client) categorizeChannelAPI(channelID string, categoryName string) error {
	if channelID == "" || categoryName == "" {
		return fmt.Errorf("channel ID and category name are required")
	}

	// Check if channel already has a categorization action to avoid duplicates
	checkURL := fmt.Sprintf("%s/plugins/playbooks/api/v0/actions/channels/%s", c.ServerURL, channelID)
	checkReq, err := http.NewRequest("GET", checkURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create check request: %w", err)
	}
	checkReq.Header.Set("Authorization", "Bearer "+c.API.AuthToken)
	
	client := &http.Client{}
	checkResp, err := client.Do(checkReq)
	if err != nil {
		return fmt.Errorf("failed to check existing actions: %w", err)
	}
	defer func() { _ = checkResp.Body.Close() }()
	
	if checkResp.StatusCode == http.StatusOK {
		// Channel already has actions, check existing categorization
		body, _ := io.ReadAll(checkResp.Body)
		if strings.Contains(string(body), "categorize_channel") {
			// Return a special error to indicate "already categorized" (not a real error)
			return fmt.Errorf("ALREADY_CATEGORIZED")
		}
	}

	// Log only when we're actually going to create a new categorization action
	Log.WithFields(logrus.Fields{"channel_id": channelID, "category_name": categoryName}).Info("📋 Categorizing channel")

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
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send categorize request: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			Log.WithFields(logrus.Fields{"error": closeErr.Error()}).Warn("⚠️ Failed to close response body")
		}
	}()

	// Check response
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("categorize request failed with status %d: %s", resp.StatusCode, string(body))
	}

	Log.WithFields(logrus.Fields{"category_name": categoryName}).Info("✅ Successfully categorized channel using Playbooks API")
	return nil
}


// SetupChannelCommands executes specified slash commands in channels sequentially
// If any command fails, the entire setup process will abort
func (c *Client) SetupChannelCommands() error {
	// Only proceed if we have a config
	if c.Config == nil || len(c.Config.Teams) == 0 {
		return nil
	}

	Log.Info("🚀 Setting up channel commands...")

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
			Log.WithFields(logrus.Fields{"team_name": teamName}).Error("❌ Team not found, can't execute commands")
			return fmt.Errorf("team '%s' not found", teamName)
		}

		// Get all channels for this team
		channels, _, err := c.API.GetPublicChannelsForTeam(context.Background(), team.Id, 0, 1000, "")
		if err != nil {
			Log.WithFields(logrus.Fields{"team_name": teamName, "error": err.Error()}).Error("❌ Failed to get channels for team")
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
				Log.WithFields(logrus.Fields{"channel_name": channelConfig.Name, "team_name": teamName}).Error("❌ Channel not found in team")
				return fmt.Errorf("channel '%s' not found in team '%s'", channelConfig.Name, teamName)
			}

			Log.WithFields(logrus.Fields{"command_count": len(channelConfig.Commands), "channel_name": channelConfig.Name}).Info("📋 Executing commands for channel (sequentially)")

			// Execute each command in order, waiting for each to complete
			for i, command := range channelConfig.Commands {
				// Check if the command has been loaded and trimmed
				commandText := strings.TrimSpace(command)

				if !strings.HasPrefix(commandText, "/") {
					Log.WithFields(logrus.Fields{"command": commandText, "channel_name": channelConfig.Name}).Error("❌ Invalid command - must start with /")
					return fmt.Errorf("invalid command '%s' - must start with /", commandText)
				}

				// Remove the leading slash for the API

				Log.WithFields(logrus.Fields{
					"command_index": i + 1,
					"total_commands": len(channelConfig.Commands),
					"channel_name": channelConfig.Name,
					"command": commandText,
				}).Info("📤 Executing command in channel")

				// Execute the command using the commands/execute API
				_, resp, err := c.API.ExecuteCommand(context.Background(), channel.Id, commandText)

				// Check for any errors or non-200 response
				if err != nil {
					Log.WithFields(logrus.Fields{"command": commandText, "error": err.Error()}).Error("❌ Failed to execute command")
					return fmt.Errorf("failed to execute command '%s': %w", commandText, err)
				}

				if resp.StatusCode != 200 {
					Log.WithFields(logrus.Fields{"command": commandText, "status_code": resp.StatusCode}).Error("❌ Command returned non-200 status code")
					return fmt.Errorf("command '%s' returned status code %d",
						commandText, resp.StatusCode)
				}

				Log.WithFields(logrus.Fields{
					"command_index": i + 1,
					"total_commands": len(channelConfig.Commands),
					"command": commandText,
					"channel_name": channelConfig.Name,
				}).Info("✅ Successfully executed command in channel")

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

	// This should have been converted already - check if log is initialized

	// Run make dist to build the plugin
	cmd := exec.Command("make", "dist")
	cmd.Dir = pluginPath
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to build plugin: %w", err)
	}

	// This should have been converted already - check if log is initialized
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
	Log.WithFields(logrus.Fields{
		"bundle_path": bundlePath,
	}).Info("📤 Uploading plugin bundle")

	// Open the bundle file
	file, err := os.Open(bundlePath)
	if err != nil {
		return fmt.Errorf("failed to open plugin bundle: %w", err)
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			Log.WithFields(logrus.Fields{
				"error": closeErr.Error(),
				"file": bundlePath,
			}).Warn("⚠️ Failed to close plugin bundle file")
		}
	}()

	Log.Info("📤 Uploading with force flag (will overwrite existing plugin)")
	// Reset file position
	if _, seekErr := file.Seek(0, 0); seekErr != nil {
		return fmt.Errorf("❌ failed to reset file position: %w", seekErr)
	}
	manifest, resp, err := c.API.UploadPluginForced(context.Background(), file)
	if err != nil {
		return handleAPIError(fmt.Sprintf("failed to upload plugin bundle '%s': %v", bundlePath, err), err, resp)
	}
	Log.WithFields(logrus.Fields{
		"plugin_name": manifest.Name,
		"plugin_id": manifest.Id,
	}).Info("✅ Plugin uploaded successfully (forced)")

	// Enable the plugin
	enableResp, enableErr := c.API.EnablePlugin(context.Background(), manifest.Id)
	if enableErr != nil {
		return handleAPIError("failed to enable plugin", enableErr, enableResp)
	}

	// This should have been converted already - check if log is initialized
	return nil
}

// Setup performs the main setup based on configuration using individual API calls
func (c *Client) Setup() error {
	return c.SetupWithForce(false, false, false)
}

// SetupWithForce performs the main setup with force options
func (c *Client) SetupWithForce(forcePlugins, forceGitHubPlugins, forceAll bool) error {
	return c.SetupWithForceAndUpdates(forcePlugins, forceGitHubPlugins, forceAll, false)
}

// SetupWithForceAndUpdates performs the main setup with force options and update checking
func (c *Client) SetupWithForceAndUpdates(forcePlugins, forceGitHubPlugins, forceAll, checkUpdates bool) error {
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

	// Use two-phase bulk import for plugins, users, teams, and channels
	if err := c.SetupWithSplitImportAndForce(forcePlugins, forceGitHubPlugins); err != nil {
		return err
	}

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
		"bulk_import.jsonl",    // Current directory (when run from root)
		"../bulk_import.jsonl", // Parent directory (when run from mattermost/)
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
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			Log.WithFields(logrus.Fields{"error": closeErr.Error()}).Warn("⚠️ Failed to close file")
		}
	}()

	var teams []BulkTeam
	var users []BulkUser

	// Define custom types that should be skipped during bulk import parsing
	customTypes := map[string]bool{
		"channel-category": true,
		"command":          true,
	}

	// Read the JSONL file line by line
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}

		// First, extract just the type field to check if it's a custom type
		var typeCheck struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal([]byte(line), &typeCheck); err != nil {
			// If we can't even parse the type, skip with warning
			Log.WithFields(logrus.Fields{"line": line}).Warn("⚠️ Failed to parse type from line, skipping")
			continue
		}

		// Skip custom types silently (no warning)
		if customTypes[typeCheck.Type] {
			continue
		}

		// For standard types, try to unmarshal as ResetImportLine
		var importLine ResetImportLine
		if err := json.Unmarshal([]byte(line), &importLine); err != nil {
			// Only warn for non-custom types that fail to parse
			Log.WithFields(logrus.Fields{"line": line}).Warn("⚠️ Failed to parse standard import line, skipping")
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

	Log.WithFields(logrus.Fields{"teams_count": len(teams), "users_count": len(users)}).Info("📋 Loaded data from bulk import")

	return &BulkImportData{
		Teams: teams,
		Users: users,
	}, nil
}

// DeleteBulkUsers permanently deletes users from bulk import data
func (c *Client) DeleteBulkUsers(users []BulkUser) error {
	if len(users) == 0 {
		Log.Info("📋 No users found in bulk import to delete")
		return nil
	}

	Log.WithFields(logrus.Fields{"user_count": len(users)}).Info("🗑️ Deleting users from bulk import")

	for _, userInfo := range users {
		// Find the user by username
		user, resp, err := c.API.GetUserByUsername(context.Background(), userInfo.Username, "")
		if err != nil {
			if resp != nil && resp.StatusCode == 404 {
				Log.WithFields(logrus.Fields{"user_name": userInfo.Username}).Warn("⚠️ User not found, skipping")
				continue
			}
			return handleAPIError(fmt.Sprintf("failed to find user '%s'", userInfo.Username), err, resp)
		}

		// Permanently delete the user
		_, err = c.API.PermanentDeleteUser(context.Background(), user.Id)
		if err != nil {
			return handleAPIError(fmt.Sprintf("failed to delete user '%s'", userInfo.Username), err, nil)
		}

		Log.WithFields(logrus.Fields{"user_name": userInfo.Username}).Info("✅ Permanently deleted user")
	}

	return nil
}

// DeleteBulkTeams permanently deletes teams from bulk import data
func (c *Client) DeleteBulkTeams(teams []BulkTeam) error {
	if len(teams) == 0 {
		Log.Info("📋 No teams found in bulk import to delete")
		return nil
	}

	Log.WithFields(logrus.Fields{"team_count": len(teams)}).Info("🗑️ Deleting teams from bulk import")

	for _, teamInfo := range teams {
		// Find the team by name
		team, resp, err := c.API.GetTeamByName(context.Background(), teamInfo.Name, "")
		if err != nil {
			if resp != nil && resp.StatusCode == 404 {
				Log.WithFields(logrus.Fields{"team_name": teamInfo.Name}).Warn("⚠️ Team not found, skipping")
				continue
			}
			return handleAPIError(fmt.Sprintf("failed to find team '%s'", teamInfo.Name), err, resp)
		}

		// Permanently delete the team
		_, err = c.API.PermanentDeleteTeam(context.Background(), team.Id)
		if err != nil {
			return handleAPIError(fmt.Sprintf("failed to delete team '%s'", teamInfo.Name), err, nil)
		}

		Log.WithFields(logrus.Fields{"team_name": teamInfo.Name}).Info("✅ Permanently deleted team")
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

	Log.Info("✅ API deletion settings are enabled")
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

	Log.Warn("🚨 WARNING: This will permanently delete all teams and users that are configured in the bulk import file.")
	Log.Warn("⚠️ This operation is irreversible.")

	// Delete users first (they need to be removed from teams before teams can be deleted)
	if err := c.DeleteBulkUsers(bulkData.Users); err != nil {
		return fmt.Errorf("failed to delete users: %w", err)
	}

	// Then delete teams
	if err := c.DeleteBulkTeams(bulkData.Teams); err != nil {
		return fmt.Errorf("failed to delete teams: %w", err)
	}

	Log.Info("✅ Reset completed successfully")
	return nil
}

// DeleteConfigUsers permanently deletes all users from the configuration
func (c *Client) DeleteConfigUsers() error {
	if c.Config == nil || len(c.Config.Users) == 0 {
		Log.Info("📋 No users found in configuration to delete")
		return nil
	}

	Log.WithFields(logrus.Fields{"user_count": len(c.Config.Users)}).Info("🗑️ Deleting users from configuration")

	for _, userConfig := range c.Config.Users {
		// Find the user by username
		user, resp, err := c.API.GetUserByUsername(context.Background(), userConfig.Username, "")
		if err != nil {
			if resp != nil && resp.StatusCode == 404 {
				Log.WithFields(logrus.Fields{"user_name": userConfig.Username}).Warn("⚠️ User not found, skipping")
				continue
			}
			return handleAPIError(fmt.Sprintf("failed to find user '%s'", userConfig.Username), err, resp)
		}

		// Permanently delete the user
		_, err = c.API.PermanentDeleteUser(context.Background(), user.Id)
		if err != nil {
			return handleAPIError(fmt.Sprintf("failed to delete user '%s'", userConfig.Username), err, nil)
		}

		Log.WithFields(logrus.Fields{"user_name": userConfig.Username}).Info("✅ Permanently deleted user")
	}

	return nil
}

// DeleteConfigTeams permanently deletes all teams from the configuration
func (c *Client) DeleteConfigTeams() error {
	if c.Config == nil || len(c.Config.Teams) == 0 {
		Log.Info("📋 No teams found in configuration to delete")
		return nil
	}

	Log.WithFields(logrus.Fields{"team_count": len(c.Config.Teams)}).Info("🗑️ Deleting teams from configuration")

	for teamName := range c.Config.Teams {
		// Find the team by name
		team, resp, err := c.API.GetTeamByName(context.Background(), teamName, "")
		if err != nil {
			if resp != nil && resp.StatusCode == 404 {
				Log.WithFields(logrus.Fields{"team_name": teamName}).Warn("⚠️ Team not found, skipping")
				continue
			}
			return handleAPIError(fmt.Sprintf("failed to find team '%s'", teamName), err, resp)
		}

		// Permanently delete the team
		_, err = c.API.PermanentDeleteTeam(context.Background(), team.Id)
		if err != nil {
			return handleAPIError(fmt.Sprintf("failed to delete team '%s'", teamName), err, nil)
		}

		Log.WithFields(logrus.Fields{"team_name": teamName}).Info("✅ Permanently deleted team")
	}

	return nil
}

// EchoLogins prints login information - always shown regardless of test mode
func (c *Client) EchoLogins() {
	Log.Info("===========================================")
	Log.Info("🔑 Mattermost logins")
	Log.Info("===========================================")

	Log.Info("- System admin")
	Log.WithFields(logrus.Fields{"username": DefaultAdminUsername}).Info("     - username")
	Log.WithFields(logrus.Fields{"password": DefaultAdminPassword}).Info("     - password")

	// If we have configuration users, display them
	if c.Config != nil && len(c.Config.Users) > 0 {
		Log.Info("- Config users:")
		for _, user := range c.Config.Users {
			Log.WithFields(logrus.Fields{"username": user.Username}).Info("     - username")
			Log.WithFields(logrus.Fields{"password": user.Password}).Info("     - password")
		}
	}

	Log.Info("- LDAP or SAML account:")
	Log.Info("     - username: professor")
	Log.Info("     - password: professor")
	Log.Info("")
	Log.Info("For more logins check out https://github.com/coltoneshaw/mattermost#accounts")
	Log.Info("")
	Log.Info("===========================================")
}
