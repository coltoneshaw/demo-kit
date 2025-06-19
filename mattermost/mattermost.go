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
	"github.com/go-ldap/ldap/v3"
	"github.com/mattermost/mattermost/server/public/model"
	"github.com/sirupsen/logrus"

	// Local imports
	ldapPkg "github.com/coltoneshaw/demokit/mattermost/ldap"
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

	for i := range MaxWaitSeconds {
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
func (c *Client) categorizeChannelAPI(channelID string, channelName string, categoryName string) error {
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
	Log.WithFields(logrus.Fields{
		"channel_id": channelID,
	}).Debug(fmt.Sprintf("📋 Categorizing %s into %s", channelName, categoryName))

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

	Log.WithFields(logrus.Fields{
		"channel_id": channelID,
	}).Info(fmt.Sprintf("✅ Successfully categorized %s into %s", channelName, categoryName))
	return nil
}

// setChannelBannerAPI sets a banner for a channel using the Mattermost API
func (c *Client) setChannelBannerAPI(channelID, channelName, text, backgroundColor string, enabled bool) error {
	if channelID == "" {
		return fmt.Errorf("channel ID is required")
	}

	Log.WithFields(logrus.Fields{
		"channel_id": channelID,
	}).Debug(fmt.Sprintf("🎯 Setting banner for %s: %s", channelName, text))

	// Create the banner info using the proper model structure
	bannerInfo := &model.ChannelBannerInfo{
		Text:            &text,
		BackgroundColor: &backgroundColor,
		Enabled:         &enabled,
	}

	// Use PatchChannel to update the channel with banner info
	channelPatch := &model.ChannelPatch{
		BannerInfo: bannerInfo,
	}

	_, resp, err := c.API.PatchChannel(context.Background(), channelID, channelPatch)
	if err != nil {
		return handleAPIError(fmt.Sprintf("failed to set banner for channel '%s'", channelName), err, resp)
	}

	Log.WithFields(logrus.Fields{
		"channel_id": channelID,
	}).Info(fmt.Sprintf("✅ Successfully set banner for %s", channelName))
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
					"command_index":  i + 1,
					"total_commands": len(channelConfig.Commands),
					"channel_name":   channelConfig.Name,
					"command":        commandText,
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
					"command_index":  i + 1,
					"total_commands": len(channelConfig.Commands),
					"command":        commandText,
					"channel_name":   channelConfig.Name,
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
				"file":  bundlePath,
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
		"plugin_id":   manifest.Id,
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

// CustomProfileField represents a custom profile field definition
type CustomProfileField struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	DisplayName string   `json:"display_name"`
	Type        string   `json:"type"`
	Options     []string `json:"options,omitempty"`
}

// UserCustomProfileFields represents a user's custom profile field values
type UserCustomProfileFields struct {
	Fields map[string]string `json:"fields"`
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

// ListCustomProfileFields retrieves all custom profile fields from the server
func (c *Client) ListCustomProfileFields() ([]CustomProfileField, error) {
	url := fmt.Sprintf("%s/api/v4/custom_profile_attributes/fields", c.ServerURL)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.API.AuthToken)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get custom fields: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to get custom fields, status %d: %s", resp.StatusCode, string(body))
	}

	var fields []CustomProfileField
	if err := json.NewDecoder(resp.Body).Decode(&fields); err != nil {
		return nil, fmt.Errorf("failed to decode custom fields: %w", err)
	}

	return fields, nil
}

// CreateCustomProfileField creates a new custom profile field with extended configuration
func (c *Client) CreateCustomProfileField(name, displayName, fieldType string, options []string) (*CustomProfileField, error) {
	url := fmt.Sprintf("%s/api/v4/custom_profile_attributes/fields", c.ServerURL)

	payload := map[string]any{
		"name":         name,
		"display_name": displayName,
		"type":         fieldType,
	}

	if len(options) > 0 {
		payload["options"] = options
	}

	return c.createCustomProfileFieldWithPayload(url, payload)
}

// CreateCustomProfileFieldExtended creates a new custom profile field with full extended configuration
func (c *Client) CreateCustomProfileFieldExtended(field UserAttributeField) (*CustomProfileField, error) {
	url := fmt.Sprintf("%s/api/v4/custom_profile_attributes/fields", c.ServerURL)

	payload := map[string]any{
		"name":         field.Name,
		"display_name": field.DisplayName,
		"type":         field.Type,
	}

	// Add extended attributes based on the new JSON structure
	attrs := map[string]any{}

	if field.LDAPAttribute != "" {
		attrs["ldap"] = field.LDAPAttribute
	}
	if field.SAMLAttribute != "" {
		attrs["saml"] = field.SAMLAttribute
	}
	if len(field.Options) > 0 {
		attrs["options"] = field.Options
	}
	if field.SortOrder > 0 {
		attrs["sort_order"] = field.SortOrder
	}
	if field.ValueType != "" {
		attrs["value_type"] = field.ValueType
	}
	if field.Visibility != "" {
		attrs["visibility"] = field.Visibility
	}

	// Set attrs if we have any extended configuration
	if len(attrs) > 0 {
		payload["attrs"] = attrs
	}

	// Add basic options for backward compatibility
	if len(field.Options) > 0 {
		payload["options"] = field.Options
	}

	return c.createCustomProfileFieldWithPayload(url, payload)
}

// createCustomProfileFieldWithPayload handles the actual API call
func (c *Client) createCustomProfileFieldWithPayload(url string, payload map[string]any) (*CustomProfileField, error) {

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonPayload))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.API.AuthToken)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to create custom field: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to create custom field, status %d: %s", resp.StatusCode, string(body))
	}

	var field CustomProfileField
	if err := json.NewDecoder(resp.Body).Decode(&field); err != nil {
		return nil, fmt.Errorf("failed to decode created field: %w", err)
	}

	return &field, nil
}

// SetupLDAP extracts users from JSONL and imports them directly into LDAP using default configuration
func (c *Client) SetupLDAP() error {
	defaultConfig := &LDAPConfig{
		URL:          "ldap://localhost:10389",
		BindDN:       "cn=admin,dc=planetexpress,dc=com",
		BindPassword: "GoodNewsEveryone",
		BaseDN:       "dc=planetexpress,dc=com",
	}
	return c.SetupLDAPWithConfig(defaultConfig)
}

// ShowLDAPSchemaExtensions displays the LDAP schema extensions that would be applied
func (c *Client) ShowLDAPSchemaExtensions() error {
	Log.Info("🔍 Showing LDAP schema extensions for custom attributes")

	// Extract custom attribute definitions from JSONL (Mattermost-specific logic)
	attributeFields, err := c.extractCustomAttributeDefinitions(c.BulkImportPath)
	if err != nil {
		return fmt.Errorf("failed to extract custom attribute definitions: %w", err)
	}

	// Convert to LDAP package types and delegate to LDAP package
	schemaConfig := ldapPkg.DefaultSchemaConfig()
	ldapClient := ldapPkg.NewClient(&LDAPConfig{})
	ldapAttributes := convertToLDAPAttributes(attributeFields)

	return ldapClient.ShowSchemaExtensions(ldapAttributes, schemaConfig)
}

// SetupLDAPWithConfig extracts users from JSONL and imports them directly into LDAP with custom configuration
func (c *Client) SetupLDAPWithConfig(config *LDAPConfig) error {
	Log.WithFields(logrus.Fields{
		"ldap_url": config.URL,
		"bind_dn":  config.BindDN,
		"base_dn":  config.BaseDN,
	}).Info("🔐 Starting LDAP setup")

	// Extract users from JSONL
	users, err := c.ExtractUsersFromJSONL(c.BulkImportPath)
	if err != nil {
		return fmt.Errorf("failed to extract users from JSONL: %w", err)
	}

	// Import users directly into LDAP
	if err := c.importUsersToLDAPWithConfig(users, config); err != nil {
		return fmt.Errorf("failed to import users to LDAP: %w", err)
	}

	// Setup LDAP groups
	if err := c.setupLDAPGroups(config); err != nil {
		return fmt.Errorf("failed to setup LDAP groups: %w", err)
	}

	// Migrate existing Mattermost users from email auth to LDAP auth
	if err := c.migrateUsersToLDAPAuth(users); err != nil {
		return fmt.Errorf("failed to migrate users to LDAP auth: %w", err)
	}

	// Link LDAP groups to Mattermost (optional - API may not be available)
	if err := c.linkLDAPGroups(); err != nil {
		Log.WithFields(logrus.Fields{
			"error": err.Error(),
		}).Warn("Failed to link LDAP groups to Mattermost API (groups still created in LDAP)")
		// Don't fail the entire setup if API linking fails
	}

	// Trigger LDAP sync to ensure Mattermost picks up all LDAP attributes and groups
	Log.Info("🔄 Triggering LDAP sync to update user attributes and groups")
	if err := c.syncLDAP(); err != nil {
		return fmt.Errorf("failed to sync LDAP: %w", err)
	}

	Log.Info("✅ LDAP setup completed successfully")
	return nil
}

// syncLDAP triggers an LDAP sync to ensure Mattermost picks up all LDAP attributes
func (c *Client) syncLDAP() error {
	Log.Info("🔄 Starting LDAP sync...")

	// SyncLdap requires a boolean parameter for includeRemovedMembers
	includeRemovedMembers := false
	resp, err := c.API.SyncLdap(context.Background(), &includeRemovedMembers)
	if err != nil {
		return handleAPIError("failed to trigger LDAP sync", err, resp)
	}

	Log.Info("✅ LDAP sync completed successfully")
	return nil
}

// LDAPConfig is an alias for the LDAP package configuration
type LDAPConfig = ldapPkg.LDAPConfig

// LDAPUser represents a user for LDAP import
type LDAPUser struct {
	Username         string
	Email            string
	FirstName        string
	LastName         string
	Password         string
	Position         string
	CustomAttributes map[string]string // Maps custom attribute names to their values
}

// ExtractUsersFromJSONL extracts user data from the JSONL file
func (c *Client) ExtractUsersFromJSONL(jsonlPath string) ([]LDAPUser, error) {
	// First, extract custom attribute definitions to know what attributes exist
	attributeFields, err := c.extractCustomAttributeDefinitions(jsonlPath)
	if err != nil {
		return nil, fmt.Errorf("failed to extract custom attribute definitions: %w", err)
	}

	// Extract user profile assignments
	userProfiles, err := c.extractUserProfiles(jsonlPath)
	if err != nil {
		return nil, fmt.Errorf("failed to extract user profiles: %w", err)
	}

	file, err := os.Open(jsonlPath)
	if err != nil {
		return nil, err
	}
	defer closeWithLog(file, "JSONL file")

	var users []LDAPUser
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}

		if entryType, ok := entry["type"].(string); ok && entryType == "user" {
			if userData, ok := entry["user"].(map[string]any); ok {
				username := getStringFromMap(userData, "username")
				user := LDAPUser{
					Username:         username,
					Email:            getStringFromMap(userData, "email"),
					FirstName:        getStringFromMap(userData, "first_name"),
					LastName:         getStringFromMap(userData, "last_name"),
					Password:         getStringFromMap(userData, "password"),
					Position:         getStringFromMap(userData, "position"),
					CustomAttributes: make(map[string]string),
				}

				// Extract custom attributes from user-profile entries
				user.CustomAttributes = c.extractCustomAttributesFromProfiles(username, userProfiles, attributeFields)

				users = append(users, user)
			}
		}
	}

	return users, scanner.Err()
}

// getStringFromMap safely extracts a string value from a map
func getStringFromMap(m map[string]any, key string) string {
	if value, ok := m[key].(string); ok {
		return value
	}
	return ""
}

// getStringFromInterface safely extracts a string value from an interface
func getStringFromInterface(value any) string {
	if str, ok := value.(string); ok {
		return str
	}
	return ""
}

// extractCustomAttributeDefinitions extracts custom attribute definitions from JSONL
func (c *Client) extractCustomAttributeDefinitions(jsonlPath string) ([]UserAttributeField, error) {
	file, err := os.Open(jsonlPath)
	if err != nil {
		return nil, err
	}
	defer closeWithLog(file, "JSONL file for attribute definitions")

	var attributeFields []UserAttributeField
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var attributeImport UserAttributeImport
		if err := json.Unmarshal([]byte(line), &attributeImport); err != nil {
			continue
		}

		if attributeImport.Type == "user-attribute" {
			attributeFields = append(attributeFields, attributeImport.Attribute)
		}
	}

	return attributeFields, scanner.Err()
}

// extractUserProfiles extracts user profile assignments from JSONL
func (c *Client) extractUserProfiles(jsonlPath string) (map[string]map[string]string, error) {
	file, err := os.Open(jsonlPath)
	if err != nil {
		return nil, err
	}
	defer closeWithLog(file, "JSONL file for user profiles")

	userProfiles := make(map[string]map[string]string)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}

		if entryType, ok := entry["type"].(string); ok && entryType == "user-profile" {
			username := getStringFromInterface(entry["user"])
			if username == "" {
				continue
			}

			if attributesInterface, ok := entry["attributes"]; ok {
				if attributesMap, ok := attributesInterface.(map[string]any); ok {
					attributes := make(map[string]string)
					for key, value := range attributesMap {
						if strValue := getStringFromInterface(value); strValue != "" {
							if len(strValue) > 64 {
								return nil, fmt.Errorf("user attribute '%s' for user '%s' exceeds 64 character limit (length: %d): %s", key, username, len(strValue), strValue)
							}
							attributes[key] = strValue
						}
					}
					userProfiles[username] = attributes
				}
			}
		}
	}

	return userProfiles, scanner.Err()
}

// extractCustomAttributesFromProfiles extracts custom attribute values from user-profile entries
func (c *Client) extractCustomAttributesFromProfiles(username string, userProfiles map[string]map[string]string, attributeFields []UserAttributeField) map[string]string {
	customAttributes := make(map[string]string)

	// Get the user's profile attributes if they exist
	userAttributes, exists := userProfiles[username]
	if !exists {
		Log.WithFields(logrus.Fields{
			"username": username,
		}).Debug("No user-profile entry found for user")
		return customAttributes
	}

	// Map profile attributes to LDAP attributes
	for _, field := range attributeFields {
		if field.LDAPAttribute == "" {
			continue // Skip if no LDAP mapping
		}

		// Look for the attribute value in the user's profile
		if value, exists := userAttributes[field.Name]; exists && value != "" {
			if len(value) > 64 {
				Log.WithFields(logrus.Fields{
					"username":   username,
					"field_name": field.Name,
					"value":      value,
					"length":     len(value),
				}).Error("User attribute exceeds 64 character limit")
				return nil
			}
			customAttributes[field.LDAPAttribute] = value
			Log.WithFields(logrus.Fields{
				"username":       username,
				"field_name":     field.Name,
				"ldap_attribute": field.LDAPAttribute,
				"value":          value,
			}).Debug("Mapped user profile attribute to LDAP")
		}
	}

	return customAttributes
}

// extractUserGroups extracts user groups from JSONL file
func (c *Client) extractUserGroups(jsonlPath string) ([]ldapPkg.LDAPGroup, error) {
	file, err := os.Open(jsonlPath)
	if err != nil {
		return nil, err
	}
	defer closeWithLog(file, "JSONL file")

	var groups []ldapPkg.LDAPGroup
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}

		// Process user-groups entries
		if entryType, ok := entry["type"].(string); ok && entryType == "user-groups" {
			group, err := parseGroupEntry(line)
			if err != nil {
				Log.WithFields(logrus.Fields{
					"error": err.Error(),
					"line":  line,
				}).Warn("⚠️ Failed to parse user-groups entry, skipping")
				continue
			}

			groups = append(groups, group)

			Log.WithFields(logrus.Fields{
				"group_name":      group.Name,
				"unique_id":       group.UniqueID,
				"member_count":    len(group.Members),
				"allow_reference": group.AllowReference,
			}).Debug("📋 Extracted user group from JSONL")
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading JSONL file: %w", err)
	}

	if len(groups) == 0 {
		Log.Info("📋 No user groups found in JSONL")
	} else {
		Log.WithFields(logrus.Fields{
			"group_count": len(groups),
		}).Info("📋 Successfully extracted user groups from JSONL")
	}

	return groups, nil
}

// setupLDAPGroups creates and configures LDAP groups from JSONL data
func (c *Client) setupLDAPGroups(config *LDAPConfig) error {
	Log.Info("👥 Setting up LDAP groups")

	// Extract groups from JSONL
	groups, err := c.extractUserGroups(c.BulkImportPath)
	if err != nil {
		return fmt.Errorf("failed to extract groups from JSONL: %w", err)
	}

	if len(groups) == 0 {
		Log.Info("📋 No groups found in JSONL, skipping group setup")
		return nil
	}

	// Connect to LDAP server
	ldapConn, err := ldap.DialURL(config.URL)
	if err != nil {
		return fmt.Errorf("failed to connect to LDAP server: %w", err)
	}
	defer func() {
		if err := ldapConn.Close(); err != nil {
			Log.WithError(err).Warn("Failed to close LDAP connection")
		}
	}()

	// Bind as admin
	if err := ldapConn.Bind(config.BindDN, config.BindPassword); err != nil {
		return fmt.Errorf("failed to bind to LDAP server: %w", err)
	}

	// Ensure schema is applied (including uniqueID attribute for groups)
	attributeFields, err := c.extractCustomAttributeDefinitions(c.BulkImportPath)
	if err != nil {
		return fmt.Errorf("failed to extract custom attribute definitions: %w", err)
	}

	if err := c.ensureCustomAttributeSchema(ldapConn, attributeFields, config); err != nil {
		return fmt.Errorf("failed to ensure custom attribute schema: %w", err)
	}

	// Create LDAP client for group operations
	ldapClient := ldapPkg.NewClient(config)

	// Create each group
	for _, group := range groups {
		if err := ldapClient.CreateGroup(ldapConn, group, config); err != nil {
			Log.WithFields(logrus.Fields{
				"group_name": group.Name,
				"error":      err.Error(),
			}).Error("Failed to create LDAP group")
			return fmt.Errorf("failed to create group %s: %w", group.Name, err)
		}
	}

	Log.WithFields(logrus.Fields{
		"group_count": len(groups),
	}).Info("✅ Successfully set up LDAP groups")

	return nil
}

// linkLDAPGroups links LDAP groups to Mattermost via API
func (c *Client) linkLDAPGroups() error {
	Log.Info("🔗 Linking LDAP groups to Mattermost")

	// Extract groups from JSONL to get group information
	groups, err := c.extractUserGroups(c.BulkImportPath)
	if err != nil {
		return fmt.Errorf("failed to extract groups from JSONL: %w", err)
	}

	if len(groups) == 0 {
		Log.Info("📋 No groups found, skipping group linking")
		return nil
	}

	linkedCount := 0
	for _, group := range groups {
		if err := c.linkSingleLDAPGroup(group); err != nil {
			Log.WithFields(logrus.Fields{
				"group_name": group.Name,
				"error":      err.Error(),
			}).Warn("Failed to link LDAP group to Mattermost")
			// Continue with other groups instead of failing completely
			continue
		}
		linkedCount++
	}

	Log.WithFields(logrus.Fields{
		"linked_count": linkedCount,
		"total_count":  len(groups),
	}).Info("✅ Linked LDAP groups to Mattermost")

	return nil
}

// linkSingleLDAPGroup links a single LDAP group to Mattermost and configures its properties
func (c *Client) linkSingleLDAPGroup(group ldapPkg.LDAPGroup) error {
	logFields := logrus.Fields{
		"group_name": group.Name,
		"unique_id":  group.UniqueID,
	}
	Log.WithFields(logFields).Debug("Linking LDAP group to Mattermost")

	// Link the LDAP group to Mattermost
	linkedGroup, _, err := c.API.LinkLdapGroup(context.Background(), group.Name)
	if err != nil {
		return fmt.Errorf("failed to link LDAP group '%s': %w", group.Name, err)
	}

	// Configure group properties if needed
	if group.AllowReference {
		if err := c.configureGroupProperties(linkedGroup.Id, group); err != nil {
			Log.WithFields(logrus.Fields{
				"group_name": group.Name,
				"group_id":   linkedGroup.Id,
				"error":      err.Error(),
			}).Warn("⚠️ Failed to configure group properties - group linked but not fully configured")
			// Continue - don't fail the entire operation for property configuration
		}
	}

	logFields["allow_reference"] = group.AllowReference
	Log.WithFields(logFields).Info("✅ Successfully linked and configured LDAP group")
	return nil
}

// parseGroupEntry parses a single group entry from JSONL format
func parseGroupEntry(line string) (ldapPkg.LDAPGroup, error) {
	var groupImport UserGroupImport
	if err := json.Unmarshal([]byte(line), &groupImport); err != nil {
		return ldapPkg.LDAPGroup{}, fmt.Errorf("failed to unmarshal group entry: %w", err)
	}

	// Validate required fields
	if groupImport.Group.Name == "" {
		return ldapPkg.LDAPGroup{}, fmt.Errorf("group name is required")
	}
	if groupImport.Group.ID == "" {
		return ldapPkg.LDAPGroup{}, fmt.Errorf("group ID is required")
	}

	return ldapPkg.LDAPGroup{
		Name:           groupImport.Group.Name,
		UniqueID:       groupImport.Group.ID,
		Members:        groupImport.Group.Members,
		AllowReference: groupImport.Group.AllowReference,
	}, nil
}

// configureGroupProperties configures group properties based on the group configuration
func (c *Client) configureGroupProperties(groupID string, group ldapPkg.LDAPGroup) error {
	// Create patch request with desired properties
	groupPatch := &model.GroupPatch{
		AllowReference: &group.AllowReference,
	}

	// Apply the configuration
	_, _, err := c.API.PatchGroup(context.Background(), groupID, groupPatch)
	if err != nil {
		return fmt.Errorf("failed to configure group properties: %w", err)
	}

	Log.WithFields(logrus.Fields{
		"group_id":        groupID,
		"group_name":      group.Name,
		"allow_reference": group.AllowReference,
	}).Debug("✅ Successfully configured group properties")

	return nil
}

// ensureCustomAttributeSchema ensures custom attributes are defined in the LDAP schema
func (c *Client) ensureCustomAttributeSchema(ldapConn *ldap.Conn, attributeFields []UserAttributeField, config *LDAPConfig) error {
	Log.WithFields(logrus.Fields{
		"attribute_count": len(attributeFields),
	}).Info("🔧 Ensuring custom attribute schema in LDAP")

	// Use the new schema extension system
	schemaConfig := ldapPkg.DefaultSchemaConfig()

	// Convert to LDAP package types
	ldapAttributes := convertToLDAPAttributes(attributeFields)

	// Apply schema extensions with proper attribute definitions and object classes
	ldapClient := ldapPkg.NewClient(config)
	ldapPkg.SetLogger(Log)
	if err := ldapClient.SetupSchema(ldapConn, ldapAttributes, schemaConfig, config); err != nil {
		return fmt.Errorf("failed to apply schema extensions: %w", err)
	}

	// Ensure custom object class
	if err := c.ensureCustomObjectClass(); err != nil {
		return fmt.Errorf("failed to ensure custom object class: %w", err)
	}

	// Log the custom attributes that will be used
	for _, field := range attributeFields {
		if field.LDAPAttribute != "" {
			Log.WithFields(logrus.Fields{
				"field_name":     field.Name,
				"ldap_attribute": field.LDAPAttribute,
				"display_name":   field.DisplayName,
			}).Debug("Custom attribute available for LDAP mapping")
		}
	}

	Log.Info("✅ Custom attribute schema verification completed")
	return nil
}

// convertToLDAPAttributes converts mattermost UserAttributeField to LDAP package format
func convertToLDAPAttributes(attributes []UserAttributeField) []ldapPkg.UserAttributeField {
	ldapAttrs := make([]ldapPkg.UserAttributeField, len(attributes))
	for i, attr := range attributes {
		ldapAttrs[i] = ldapPkg.UserAttributeField{
			Name:          attr.Name,
			DisplayName:   attr.DisplayName,
			Type:          attr.Type,
			LDAPAttribute: attr.LDAPAttribute,
			Required:      attr.Required,
		}
	}
	return ldapAttrs
}

// ensureCustomObjectClass ensures that the inetOrgPerson object class (which we use) supports our attributes
func (c *Client) ensureCustomObjectClass() error {
	// The rroemhild/test-openldap image includes support for many attributes through inetOrgPerson
	// and organizationalPerson object classes. Custom attributes are dynamically created based on
	// the 'ldap' field values in user-attribute definitions from the JSONL configuration.
	//
	// Note: For production LDAP servers, you may need to extend the schema to support
	// custom attributes that aren't part of the standard inetOrgPerson object class.

	Log.Debug("Using dynamic LDAP attributes from user-attribute configuration")
	return nil
}

// formatLDAPAttributesForDebug formats LDAP attributes for debug logging
func (c *Client) formatLDAPAttributesForDebug(addRequest *ldap.AddRequest) map[string]any {
	attributes := make(map[string]any)

	for _, attr := range addRequest.Attributes {
		// Don't log passwords in debug output
		if attr.Type == "userPassword" {
			attributes[attr.Type] = "[REDACTED]"
		} else {
			attributes[attr.Type] = attr.Vals
		}
	}

	return attributes
}

// GenerateLDIFContent generates LDIF content from user data with schema extensions
func (c *Client) GenerateLDIFContent(users []LDAPUser) (string, error) {
	var ldif strings.Builder

	// Check if we need schema extensions
	var attributeFields []UserAttributeField
	hasCustomAttributes := false
	for _, user := range users {
		if len(user.CustomAttributes) > 0 {
			hasCustomAttributes = true
			break
		}
	}

	// If we have custom attributes, extract attribute definitions for schema generation
	if hasCustomAttributes {
		var err error
		attributeFields, err = c.extractCustomAttributeDefinitions(c.BulkImportPath)
		if err != nil {
			Log.WithFields(logrus.Fields{"error": err.Error()}).Warn("⚠️ Failed to extract attribute definitions for LDIF schema")
		}
	}

	// Add schema extensions if needed
	if hasCustomAttributes && len(attributeFields) > 0 {
		schemaConfig := ldapPkg.DefaultSchemaConfig()
		ldapClient := ldapPkg.NewClient(&LDAPConfig{})
		ldapAttributes := convertToLDAPAttributes(attributeFields)
		schemaLDIF, err := ldapClient.BuildSchemaLDIF(ldapAttributes, schemaConfig)
		if err != nil {
			Log.WithFields(logrus.Fields{"error": err.Error()}).Warn("⚠️ Failed to generate schema LDIF")
		} else {
			ldif.WriteString(schemaLDIF)
			ldif.WriteString("\n")
		}
	}

	// Add base DN and organization
	ldif.WriteString("# Base DN\n")
	ldif.WriteString("dn: dc=planetexpress,dc=com\n")
	ldif.WriteString("objectClass: domain\n")
	ldif.WriteString("objectClass: top\n")
	ldif.WriteString("dc: planetexpress\n")
	ldif.WriteString("\n")

	// Add organizational unit for people
	ldif.WriteString("# Organizational Unit for People\n")
	ldif.WriteString("dn: ou=people,dc=planetexpress,dc=com\n")
	ldif.WriteString("objectClass: organizationalUnit\n")
	ldif.WriteString("objectClass: top\n")
	ldif.WriteString("ou: people\n")
	ldif.WriteString("\n")

	// Add users
	for _, user := range users {
		ldif.WriteString(fmt.Sprintf("# User: %s\n", user.Username))
		ldif.WriteString(fmt.Sprintf("dn: uid=%s,ou=people,dc=planetexpress,dc=com\n", user.Username))

		// Standard object classes
		ldif.WriteString("objectClass: inetOrgPerson\n")
		ldif.WriteString("objectClass: organizationalPerson\n")
		ldif.WriteString("objectClass: person\n")
		ldif.WriteString("objectClass: top\n")

		// Add custom object class if we have custom attributes
		if len(user.CustomAttributes) > 0 {
			schemaConfig := ldapPkg.DefaultSchemaConfig()
			ldif.WriteString(fmt.Sprintf("objectClass: %s\n", schemaConfig.ObjectClassName))
		}

		ldif.WriteString(fmt.Sprintf("uid: %s\n", user.Username))
		ldif.WriteString(fmt.Sprintf("cn: %s %s\n", user.FirstName, user.LastName))
		ldif.WriteString(fmt.Sprintf("sn: %s\n", user.LastName))
		ldif.WriteString(fmt.Sprintf("givenName: %s\n", user.FirstName))
		ldif.WriteString(fmt.Sprintf("mail: %s\n", user.Email))
		if user.Position != "" {
			ldif.WriteString(fmt.Sprintf("title: %s\n", user.Position))
		}

		// Add custom attributes
		for ldapAttr, value := range user.CustomAttributes {
			if value != "" {
				ldif.WriteString(fmt.Sprintf("%s: %s\n", ldapAttr, value))
			}
		}

		// Set password from user data or use default for demo purposes
		if user.Password != "" {
			ldif.WriteString(fmt.Sprintf("userPassword: %s\n", user.Password))
		} else {
			ldif.WriteString("userPassword: {SSHA}password123\n")
		}
		ldif.WriteString("\n")
	}

	return ldif.String(), nil
}

// importUsersToLDAPWithConfig connects directly to LDAP and creates users with custom configuration
func (c *Client) importUsersToLDAPWithConfig(users []LDAPUser, config *LDAPConfig) error {
	Log.WithFields(logrus.Fields{"user_count": len(users)}).Info("📥 Importing users directly to LDAP")

	// Connect to LDAP server
	ldapConn, err := ldap.DialURL(config.URL)
	if err != nil {
		return fmt.Errorf("failed to connect to LDAP server: %w", err)
	}
	defer func() {
		if err := ldapConn.Close(); err != nil {
			Log.WithFields(logrus.Fields{"error": err.Error()}).Warn("⚠️ Failed to close LDAP connection")
		}
	}()

	// Bind as admin
	err = ldapConn.Bind(config.BindDN, config.BindPassword)
	if err != nil {
		return fmt.Errorf("failed to bind to LDAP server: %w", err)
	}

	// Ensure base organizational structure exists
	if err := c.ensureLDAPStructureWithConfig(ldapConn, config); err != nil {
		return fmt.Errorf("failed to ensure LDAP structure: %w", err)
	}

	// Extract custom attribute definitions and ensure they exist in LDAP schema
	attributeFields, err := c.extractCustomAttributeDefinitions(c.BulkImportPath)
	if err != nil {
		return fmt.Errorf("failed to extract custom attribute definitions: %w", err)
	}

	if len(attributeFields) > 0 {
		if err := c.ensureCustomAttributeSchema(ldapConn, attributeFields, config); err != nil {
			return fmt.Errorf("failed to ensure custom attribute schema: %w", err)
		}
	}

	// Create users
	successCount := 0
	errorCount := 0

	for _, user := range users {
		if err := c.createLDAPUserWithConfig(ldapConn, user, config); err != nil {
			Log.WithFields(logrus.Fields{
				"username": user.Username,
				"error":    err.Error(),
			}).Warn("⚠️ Failed to create LDAP user")
			errorCount++
		} else {
			successCount++
		}
	}

	Log.WithFields(logrus.Fields{
		"success_count": successCount,
		"error_count":   errorCount,
	}).Info("✅ LDAP user import completed")

	if errorCount > 0 {
		return fmt.Errorf("failed to create %d out of %d users", errorCount, len(users))
	}

	return nil
}

// ensureLDAPStructureWithConfig ensures the base organizational structure exists in LDAP
func (c *Client) ensureLDAPStructureWithConfig(ldapConn *ldap.Conn, config *LDAPConfig) error {
	// Check if base DN exists
	searchRequest := ldap.NewSearchRequest(
		config.BaseDN,
		ldap.ScopeBaseObject,
		ldap.NeverDerefAliases,
		0, 0, false,
		"(objectClass=*)",
		[]string{"dn"},
		nil,
	)

	_, err := ldapConn.Search(searchRequest)
	if err != nil {
		// Base DN doesn't exist, create it
		Log.Info("🏢 Creating base LDAP organizational structure")

		// Create base domain
		addRequest := ldap.NewAddRequest(config.BaseDN, nil)
		addRequest.Attribute("objectClass", []string{"domain", "top"})
		// Extract the first DC component for the attribute
		dcValue := strings.Split(config.BaseDN, ",")[0]
		dcValue = strings.TrimPrefix(dcValue, "dc=")
		addRequest.Attribute("dc", []string{dcValue})

		if err := ldapConn.Add(addRequest); err != nil {
			Log.WithFields(logrus.Fields{"error": err.Error()}).Debug("Base DN might already exist")
		}
	}

	// Check if people OU exists
	peopleOU := fmt.Sprintf("ou=people,%s", config.BaseDN)
	searchRequest = ldap.NewSearchRequest(
		peopleOU,
		ldap.ScopeBaseObject,
		ldap.NeverDerefAliases,
		0, 0, false,
		"(objectClass=*)",
		[]string{"dn"},
		nil,
	)

	_, err = ldapConn.Search(searchRequest)
	if err != nil {
		// People OU doesn't exist, create it
		addRequest := ldap.NewAddRequest(peopleOU, nil)
		addRequest.Attribute("objectClass", []string{"organizationalUnit", "top"})
		addRequest.Attribute("ou", []string{"people"})

		if err := ldapConn.Add(addRequest); err != nil {
			return fmt.Errorf("failed to create people OU: %w", err)
		}
		Log.Info("✅ Created people organizational unit")
	}

	return nil
}

// createLDAPUserWithConfig creates a single user in LDAP
func (c *Client) createLDAPUserWithConfig(ldapConn *ldap.Conn, user LDAPUser, config *LDAPConfig) error {
	dn := fmt.Sprintf("uid=%s,ou=people,%s", user.Username, config.BaseDN)

	// Check if user already exists
	searchRequest := ldap.NewSearchRequest(
		dn,
		ldap.ScopeBaseObject,
		ldap.NeverDerefAliases,
		0, 0, false,
		"(objectClass=*)",
		[]string{"dn"},
		nil,
	)

	_, err := ldapConn.Search(searchRequest)
	if err == nil {
		// User already exists, skip
		Log.WithFields(logrus.Fields{"username": user.Username}).Debug("User already exists in LDAP, skipping")
		return nil
	}

	// Create user with appropriate object classes
	addRequest := ldap.NewAddRequest(dn, nil)

	// Standard object classes
	objectClasses := []string{"inetOrgPerson", "organizationalPerson", "person", "top"}

	// Add custom object class if we have custom attributes
	schemaConfig := ldapPkg.DefaultSchemaConfig()
	if len(user.CustomAttributes) > 0 {
		objectClasses = append(objectClasses, schemaConfig.ObjectClassName)
	}

	addRequest.Attribute("objectClass", objectClasses)
	addRequest.Attribute("uid", []string{user.Username})
	addRequest.Attribute("cn", []string{fmt.Sprintf("%s %s", user.FirstName, user.LastName)})
	addRequest.Attribute("sn", []string{user.LastName})
	addRequest.Attribute("givenName", []string{user.FirstName})
	addRequest.Attribute("mail", []string{user.Email})

	if user.Position != "" {
		addRequest.Attribute("title", []string{user.Position})
	}

	// Set password from JSONL or use default for demo purposes
	if user.Password != "" {
		addRequest.Attribute("userPassword", []string{user.Password})
	} else {
		addRequest.Attribute("userPassword", []string{"password123"})
	}

	// Add custom attributes if they exist
	for ldapAttr, value := range user.CustomAttributes {
		if value != "" {
			addRequest.Attribute(ldapAttr, []string{value})
			Log.WithFields(logrus.Fields{
				"username":  user.Username,
				"attribute": ldapAttr,
				"value":     value,
			}).Debug("Added custom LDAP attribute")
		}
	}

	// Debug: Log the complete LDAP add request
	Log.WithFields(logrus.Fields{
		"username":   user.Username,
		"dn":         dn,
		"attributes": c.formatLDAPAttributesForDebug(addRequest),
	}).Debug("Creating LDAP user with attributes")

	if err := ldapConn.Add(addRequest); err != nil {
		return fmt.Errorf("failed to add user %s: %w", user.Username, err)
	}

	Log.WithFields(logrus.Fields{"username": user.Username, "dn": dn}).Debug("Created LDAP user")
	return nil
}

// migrateUsersToLDAPAuth migrates existing Mattermost users from email authentication to LDAP authentication
func (c *Client) migrateUsersToLDAPAuth(users []LDAPUser) error {
	Log.WithFields(logrus.Fields{"user_count": len(users)}).Info("🔄 Migrating users from email auth to LDAP auth")

	successCount := 0
	errorCount := 0

	for _, user := range users {
		Log.WithFields(logrus.Fields{"username": user.Username}).Debug("Attempting to migrate user from JSONL to LDAP auth")

		if err := c.migrateUserToLDAP(user.Username); err != nil {
			Log.WithFields(logrus.Fields{
				"username": user.Username,
				"error":    err.Error(),
			}).Warn("⚠️ Failed to migrate user to LDAP auth")
			errorCount++
		} else {
			successCount++
		}
	}

	Log.WithFields(logrus.Fields{
		"success_count": successCount,
		"error_count":   errorCount,
	}).Info("✅ User auth migration completed")

	if errorCount > 0 {
		Log.WithFields(logrus.Fields{"error_count": errorCount}).Warn("Some users failed to migrate to LDAP auth")
	}

	return nil
}

// migrateUserToLDAP migrates a single user from email authentication to LDAP authentication
func (c *Client) migrateUserToLDAP(username string) error {
	// First, find the user by username to get their user ID
	user, resp, err := c.API.GetUserByUsername(context.Background(), username, "")
	if err != nil {
		if resp != nil && resp.StatusCode == 404 {
			Log.WithFields(logrus.Fields{"username": username}).Debug("User not found in Mattermost, skipping migration")
			return nil
		}
		return fmt.Errorf("failed to find user '%s': %w", username, err)
	}

	// Update user authentication to LDAP
	url := fmt.Sprintf("%s/api/v4/users/%s/auth", c.ServerURL, user.Id)

	payload := map[string]any{
		"auth_data":    username, // Use username as auth_data for LDAP
		"auth_service": "ldap",
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal auth update payload: %w", err)
	}

	Log.WithFields(logrus.Fields{
		"username": username,
		"user_id":  user.Id,
		"service":  "ldap",
	}).Debug("Updating user authentication method to LDAP")

	req, err := http.NewRequest("PUT", url, bytes.NewBuffer(jsonPayload))
	if err != nil {
		return fmt.Errorf("failed to create auth update request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.API.AuthToken)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	httpResp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to execute auth update request: %w", err)
	}
	defer func() { _ = httpResp.Body.Close() }()

	if httpResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(httpResp.Body)
		return fmt.Errorf("failed to update user auth, status %d: %s", httpResp.StatusCode, string(body))
	}

	Log.WithFields(logrus.Fields{"username": username}).Debug("Successfully updated user to LDAP auth")
	return nil
}
