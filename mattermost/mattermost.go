// Package mattermost provides tools for setting up and configuring a Mattermost server
package mattermost

import (
	// Standard library imports
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

	// Managers for different operations
	UserManager    *UserManager
	TeamManager    *TeamManager
	ChannelManager *ChannelManager
	PluginManager  *PluginManager
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

	// Initialize managers
	client.UserManager = NewUserManager(client)
	client.TeamManager = NewTeamManager(client)
	client.ChannelManager = NewChannelManager(client)
	client.PluginManager = NewPluginManager(client)

	// Load the configuration if possible
	config, err := LoadConfig(configPath)
	if err == nil {
		client.Config = config
	} else {
		fmt.Printf("❌ Failed to load config file: %v\n", err)
	}

	return client
}

// Login authenticates with the Mattermost server
func (c *Client) Login() error {
	user, resp, err := c.API.Login(context.Background(), c.AdminUser, c.AdminPass)
	if err != nil {
		return handleAPIError(fmt.Sprintf("login failed for user '%s' with password '%s'", c.AdminUser, c.AdminPass), err, resp)
	}

	// Ensure the logged-in user has admin privileges
	if err := c.UserManager.EnsureUserIsAdmin(user); err != nil {
		return fmt.Errorf("failed to ensure admin role for user '%s': %w", c.AdminUser, err)
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

// CreateUsers creates test users from the config file if they don't exist.
// If no config file is available, it falls back to creating default users.
func (c *Client) CreateUsers() error {
	return c.UserManager.CreateUsersFromConfig(c.Config, c.TeamManager)
}

// createOrGetTeam creates a new team or returns an existing one
func (c *Client) createOrGetTeam(teamName, displayName, description, teamType string) (*model.Team, error) {
	// Get all existing teams
	existingTeams, resp, err := c.API.GetAllTeams(context.Background(), "", 0, 100)
	if err != nil {
		return nil, handleAPIError("failed to get teams", err, resp)
	}

	// Check if team already exists
	for _, team := range existingTeams {
		if team.Name == teamName {
			fmt.Printf("Team '%s' already exists\n", teamName)
			return team, nil
		}
	}

	// Create the team
	fmt.Printf("Creating '%s' team...\n", teamName)

	// Default to Open type if not specified
	if teamType == "" {
		teamType = model.TeamOpen
	}

	newTeam := &model.Team{
		Name:        teamName,
		DisplayName: displayName,
		Description: description,
		Type:        teamType,
	}

	createdTeam, createResp, err := c.API.CreateTeam(context.Background(), newTeam)
	if err != nil {
		return nil, handleAPIError("failed to create team", err, createResp)
	}

	fmt.Printf("✅ Successfully created team '%s' (ID: %s)\n", createdTeam.Name, createdTeam.Id)
	return createdTeam, nil
}

// CreateTeam creates teams from config or a default team if no config available
// It creates teams as needed
func (c *Client) CreateTeam() error {
	// Get all existing teams (should include teams created by CreateUsers)
	existingTeams, resp, err := c.API.GetAllTeams(context.Background(), "", 0, 100)
	if err != nil {
		return handleAPIError("failed to get teams", err, resp)
	}

	// Create a map of existing teams for quick lookup
	existingTeamMap := make(map[string]*model.Team)
	for _, team := range existingTeams {
		existingTeamMap[team.Name] = team
	}

	// If we have a config file with teams, set up channels and resources
	// (teams should already be created by CreateUsers)
	if c.Config != nil && len(c.Config.Teams) > 0 {
		fmt.Println("Setting up team resources (channels, etc.)...")

		// Process each team from the config
		for _, teamConfig := range c.Config.Teams {
			team, exists := existingTeamMap[teamConfig.Name]
			if !exists {
				fmt.Printf("❌ Team '%s' not found, skipping resource setup\n", teamConfig.Name)
				continue
			}

			// Set up channels for this team
			if err := c.setupTeamResources(team, teamConfig); err != nil {
				fmt.Printf("⚠️ Warning: Error setting up resources for team '%s': %v\n", teamConfig.Name, err)
			}
		}
	} else {
		// Fallback to creating the default team if it doesn't exist
		fmt.Println("No team configuration found. Creating default team...")

		team, err := c.createOrGetTeam(
			c.TeamName,
			"Test Team",
			"",
			model.TeamOpen,
		)
		if err != nil {
			return fmt.Errorf("failed to create default team: %w", err)
		}

		// Add default team to the map
		existingTeamMap[team.Name] = team
	}

	// Return if we couldn't create any teams
	if len(existingTeamMap) == 0 {
		return fmt.Errorf("no teams could be created or found")
	}

	// Note: User-team assignments are now handled in CreateUsers(),
	// but we keep AddUsersToTeams for any legacy or fallback scenarios
	if err := c.AddUsersToTeams(existingTeamMap); err != nil {
		return err
	}

	return nil
}

// addUserToTeam adds a user to a team and handles common error cases
func (c *Client) addUserToTeam(user *model.User, team *model.Team) error {
	// Check if user is already a team member
	_, resp, err := c.API.GetTeamMember(context.Background(), team.Id, user.Id, "")
	if err == nil && resp.StatusCode == 200 {
		fmt.Printf("User '%s' is already a member of team '%s'\n", user.Username, team.Name)
		return nil
	}

	_, teamResp, err := c.API.AddTeamMember(context.Background(), team.Id, user.Id)
	if err != nil {
		// Check if the error is because the user is already a member
		if teamResp != nil && teamResp.StatusCode == 400 {
			fmt.Printf("User '%s' is already a member of team '%s'\n", user.Username, team.Name)
			return nil
		}
		return fmt.Errorf("failed to add user '%s' to team '%s': %w", user.Username, team.Name, err)
	}

	fmt.Printf("✅ Added user '%s' to team '%s'\n", user.Username, team.Name)
	return nil
}

// AddUsersToTeams adds users to teams according to the configuration
func (c *Client) AddUsersToTeams(teamMap map[string]*model.Team) error {
	// Get all existing users
	existingUsers, resp, err := c.API.GetUsers(context.Background(), 0, 1000, "")
	if err != nil {
		return handleAPIError("failed to get users", err, resp)
	}

	// Create a map of existing users for quick lookup
	userMap := make(map[string]*model.User)
	for _, user := range existingUsers {
		userMap[user.Username] = user
	}

	// If we have a config file with users, use their team assignments
	if c.Config != nil && len(c.Config.Users) > 0 {
		fmt.Println("Adding users to teams from configuration...")

		// Loop through each user in config
		for _, userConfig := range c.Config.Users {
			// Skip if user doesn't exist
			user, exists := userMap[userConfig.Username]
			if !exists {
				fmt.Printf("❌ User '%s' not found, can't add to teams\n", userConfig.Username)
				continue
			}

			// Add user to each of their teams
			for _, teamName := range userConfig.Teams {
				// Skip if team doesn't exist
				team, exists := teamMap[teamName]
				if !exists {
					fmt.Printf("❌ Team '%s' not found, can't add user '%s'\n", teamName, userConfig.Username)
					continue
				}

				// Add user to team with error handling
				if err := c.addUserToTeam(user, team); err != nil {
					fmt.Printf("❌ %v\n", err)
				}
			}
		}
	} else {
		// Fallback to adding only default admin user to default team
		fmt.Println("No user configuration found. Adding only default admin to default team...")

		// Get default team
		team, exists := teamMap[c.TeamName]
		if !exists {
			return fmt.Errorf("default team '%s' not found", c.TeamName)
		}

		// Add default admin user to the default team
		user, exists := userMap[DefaultAdminUsername]
		if !exists {
			fmt.Printf("❌ Default admin user '%s' not found\n", DefaultAdminUsername)
			return nil
		}

		// Add admin user to team with error handling
		if err := c.addUserToTeam(user, team); err != nil {
			fmt.Printf("❌ %v\n", err)
		}
	}

	return nil
}

// GetTeamAndChannel finds a team and channel by name
func (c *Client) GetTeamAndChannel(channelName string) (teamID, channelID string, err error) {
	// Get team ID by name
	teams, resp, err := c.API.GetAllTeams(context.Background(), "", 0, 100)
	if err != nil {
		return "", "", handleAPIError("failed to get teams", err, resp)
	}

	for _, team := range teams {
		if team.Name == c.TeamName {
			teamID = team.Id
			break
		}
	}

	if teamID == "" {
		return "", "", fmt.Errorf("team '%s' not found", c.TeamName)
	}

	// Get channel ID by name
	channels, _, err := c.API.GetPublicChannelsForTeam(context.Background(), teamID, 0, 100, "")
	if err != nil {
		return "", "", handleAPIError("failed to get channels", err, nil)
	}

	for _, channel := range channels {
		if channel.Name == channelName {
			channelID = channel.Id
			break
		}
	}

	if channelID == "" {
		return "", "", fmt.Errorf("channel '%s' not found in team %s", channelName, c.TeamName)
	}

	return teamID, channelID, nil
}

// setupTeamResources sets up channels for a specific team
// This is called immediately after creating or finding a team to ensure resources are set up
func (c *Client) setupTeamResources(team *model.Team, teamConfig TeamConfig) error {
	if team == nil {
		return fmt.Errorf("team is nil, cannot set up resources")
	}

	teamID := team.Id
	fmt.Printf("Setting up resources for team '%s' (ID: %s)\n", team.Name, teamID)

	// Set up channels for this team first
	if err := c.setupTeamChannels(team, teamConfig); err != nil {
		return fmt.Errorf("failed to set up channels for team '%s': %w", team.Name, err)
	}

	return nil
}

// createOrGetChannel creates a new channel or returns an existing one
func (c *Client) createOrGetChannel(teamID, name, displayName, purpose, header, channelType string) (*model.Channel, error) {
	// Get existing channels
	channels, resp, err := c.API.GetPublicChannelsForTeam(context.Background(), teamID, 0, 1000, "")
	if err != nil {
		return nil, handleAPIError("failed to get public channels", err, resp)
	}

	// Also check private channels if we're looking for one
	if channelType == "P" {
		privateChannels, _, privateErr := c.API.GetPrivateChannelsForTeam(context.Background(), teamID, 0, 1000, "")
		if privateErr == nil {
			channels = append(channels, privateChannels...)
		} else {
			fmt.Printf("❌ Warning: Failed to get private channels: %v\n", privateErr)
		}
	}

	// Check if channel already exists
	for _, channel := range channels {
		if channel.Name == name {
			fmt.Printf("Channel '%s' already exists in team\n", name)
			return channel, nil
		}
	}

	// Default to Open if not specified
	if channelType == "" {
		channelType = "O"
	}

	// Create the channel
	fmt.Printf("Creating '%s' channel...\n", name)

	newChannel := &model.Channel{
		TeamId:      teamID,
		Name:        name,
		DisplayName: displayName,
		Purpose:     purpose,
		Header:      header,
		Type:        model.ChannelType(channelType),
	}

	createdChannel, createResp, err := c.API.CreateChannel(context.Background(), newChannel)
	if err != nil {
		return nil, handleAPIError("failed to create channel", err, createResp)
	}

	fmt.Printf("✅ Successfully created channel '%s' (ID: %s)\n", createdChannel.Name, createdChannel.Id)
	return createdChannel, nil
}

// addUserToChannel adds a user to a channel and handles common error cases
func (c *Client) addUserToChannel(userID, channelID, username, channelName string) error {
	// Check if user is already a channel member
	_, resp, err := c.API.GetChannelMember(context.Background(), channelID, userID, "")
	if err == nil && resp.StatusCode == 200 {
		fmt.Printf("User '%s' is already a member of channel '%s'\n", username, channelName)
		return nil
	}

	_, resp, err = c.API.AddChannelMember(context.Background(), channelID, userID)
	if err != nil {
		// Check if the error is because the user is already a member
		if resp != nil && resp.StatusCode == 400 {
			// Look for the "already a member" message
			fmt.Printf("User '%s' is already a member of channel '%s'\n", username, channelName)
			return nil
		}
		return fmt.Errorf("failed to add user '%s' to channel '%s': %w", username, channelName, err)
	}

	fmt.Printf("✅ Added user '%s' to channel '%s'\n", username, channelName)
	return nil
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
	if err := resp.Body.Close(); err != nil {
		fmt.Printf("failed to close response body: %v", err)
	}

	// Check response
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("categorize request failed with status %d: %s", resp.StatusCode, string(body))
	}

	fmt.Printf("✅ Successfully categorized channel in '%s' using Playbooks API\n", categoryName)
	return nil
}

// categorizeChannel adds a channel to a category
func (c *Client) categorizeChannel(channelID string, categoryName string) error {
	if channelID == "" || categoryName == "" {
		return fmt.Errorf("channel ID and category name are required")
	}

	return c.categorizeChannelAPI(channelID, categoryName)

}

// setupTeamChannels creates channels for a specific team and adds members
func (c *Client) setupTeamChannels(team *model.Team, teamConfig TeamConfig) error {
	teamID := team.Id
	fmt.Printf("Setting up channels for team '%s' (ID: %s)\n", team.Name, teamID)

	// Skip if no channels are configured
	if len(teamConfig.Channels) == 0 {
		fmt.Printf("No channels configured for team '%s'\n", team.Name)
		return nil
	}

	// Create each channel first (without adding members)
	for _, channelConfig := range teamConfig.Channels {
		// Create or get the channel
		channel, err := c.createOrGetChannel(
			teamID,
			channelConfig.Name,
			channelConfig.DisplayName,
			channelConfig.Purpose,
			channelConfig.Header,
			channelConfig.Type,
		)
		if err != nil {
			fmt.Printf("❌ Failed to create channel '%s': %v\n", channelConfig.Name, err)
			continue
		}

		// If a category is specified, categorize the channel before adding members
		if channelConfig.Category != "" {
			if err := c.categorizeChannel(channel.Id, channelConfig.Category); err != nil {
				fmt.Printf("⚠️ Warning: Failed to categorize channel '%s' in category '%s': %v\n",
					channelConfig.Name, channelConfig.Category, err)
				// Don't return error here, continue with other operations
			}
		}
	}

	// We'll add members to channels after users have been added to the team
	// This will be done in a separate function below

	return nil
}

// AddChannelMembers adds members to channels after teams and users are fully set up
func (c *Client) AddChannelMembers() error {
	// Only proceed if we have a config
	if c.Config == nil || len(c.Config.Teams) == 0 {
		return nil
	}

	fmt.Println("Adding members to channels...")

	// Get all teams
	teams, resp, err := c.API.GetAllTeams(context.Background(), "", 0, 100)
	if err != nil {
		return handleAPIError("failed to get teams", err, resp)
	}

	// Create a map of team names to team IDs
	teamMap := make(map[string]*model.Team)
	for _, team := range teams {
		teamMap[team.Name] = team
	}

	// Get all users
	users, resp, err := c.API.GetUsers(context.Background(), 0, 1000, "")
	if err != nil {
		return handleAPIError("failed to get users", err, resp)
	}

	// Create a map of usernames to user IDs
	userMap := make(map[string]*model.User)
	for _, user := range users {
		userMap[user.Username] = user
	}

	// For each team in the config
	for teamName, teamConfig := range c.Config.Teams {
		team, exists := teamMap[teamName]
		if !exists {
			fmt.Printf("❌ Team '%s' not found, can't add channel members\n", teamName)
			continue
		}

		// For each channel in the team
		for _, channelConfig := range teamConfig.Channels {
			// Skip if no members are specified
			if len(channelConfig.Members) == 0 {
				continue
			}

			// Get the channel
			channels, _, err := c.API.GetPublicChannelsForTeam(context.Background(), team.Id, 0, 1000, "")
			if err != nil {
				fmt.Printf("❌ Failed to get channels for team '%s': %v\n", teamName, err)
				continue
			}

			// If it's a private channel, get those too
			if channelConfig.Type == "P" {
				privateChannels, _, err := c.API.GetPrivateChannelsForTeam(context.Background(), team.Id, 0, 1000, "")
				if err == nil {
					channels = append(channels, privateChannels...)
				}
			}

			// Find the channel by name
			var channel *model.Channel
			for _, ch := range channels {
				if ch.Name == channelConfig.Name {
					channel = ch
					break
				}
			}

			if channel == nil {
				fmt.Printf("❌ Channel '%s' not found in team '%s'\n", channelConfig.Name, teamName)
				continue
			}

			// Add members to the channel
			fmt.Printf("Adding %d members to channel '%s'\n", len(channelConfig.Members), channelConfig.Name)

			for _, username := range channelConfig.Members {
				user, exists := userMap[username]
				if !exists {
					fmt.Printf("❌ User '%s' not found, can't add to channel '%s'\n", username, channelConfig.Name)
					continue
				}

				if err := c.addUserToChannel(user.Id, channel.Id, username, channelConfig.Name); err != nil {
					fmt.Printf("❌ Failed to add user '%s' to channel '%s': %v\n", username, channelConfig.Name, err)
				}
			}
		}
	}

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

// SetupTestData sets up test data in Mattermost based on configuration
func (c *Client) SetupTestData() error {
	fmt.Println("===========================================")
	fmt.Println("Setting up test Data for Mattermost")
	fmt.Println("===========================================")

	// Load configuration if not already loaded
	if c.Config == nil {
		config, err := LoadConfig(c.ConfigPath)
		if err != nil {
			fmt.Printf("❌ Failed to load config: %v, using defaults\n", err)
		} else {
			c.Config = config
		}
	}

	// CreateUsers now handles both user creation and team assignments from config
	if err := c.CreateUsers(); err != nil {
		return err
	}

	// Only run CreateTeam if we need to handle legacy setup or channels
	// (since CreateUsers now handles teams from config)
	if err := c.CreateTeam(); err != nil {
		return err
	}

	// Now that teams are created and users added to teams,
	// we can add users to channels
	if err := c.AddChannelMembers(); err != nil {
		fmt.Printf("❌ Warning: Error adding channel members: %v\n", err)
		// Don't return error here, continue with setup
	}

	// Now that channels are fully set up with members and plugins are installed,
	// we can execute the channel commands
	if err := c.SetupChannelCommands(); err != nil {
		fmt.Printf("❌ Error executing channel commands: %v\n", err)
		// Return error here to abort the setup process
		return fmt.Errorf("failed to execute channel commands: %w", err)
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

// Setup performs the main setup based on configuration
func (c *Client) Setup() error {
	// Safety check - make sure the client and API are properly initialized
	if c == nil || c.API == nil {
		return fmt.Errorf("client not properly initialized")
	}

	// Load configuration if not already loaded
	if c.Config == nil {
		var configPath string
		if c.ConfigPath != "" {
			configPath = c.ConfigPath
		} else {
			configPath = DefaultConfigPath
		}

		config, err := LoadConfig(configPath)
		if err != nil {
			return fmt.Errorf("failed to load config file: %w", err)
		}
		c.Config = config
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

	// Download and install latest plugins from config and files directory
	if err := c.PluginManager.SetupLatestPlugins(c.Config); err != nil {
		return fmt.Errorf("failed to setup plugins: %w", err)
	}

	if err := c.SetupTestData(); err != nil {
		return err
	}

	fmt.Println("Alright, everything seems to be setup and running. Enjoy.")
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
