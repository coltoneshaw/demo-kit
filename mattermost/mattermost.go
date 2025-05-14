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
	"strings"
	"time"

	// Third party imports
	"github.com/mattermost/mattermost/server/public/model"
)

// Constants for configuration and behaviors
const (
	// MaxWaitSeconds is the maximum time to wait for server startup
	MaxWaitSeconds = 120

	// EnvFile is the path to the environment variables file
	EnvFile = "../files/env_vars.env"

	// TestUrlPattern is used to detect when we're running in test mode
	TestUrlPattern = "mattermost-kit.ngrok.io"

	// Default admin user credentials
	DefaultAdminUsername = "systemadmin"
	DefaultAdminPassword = "Password123!"
)

// WebhookConfigFunction is a function type for updating webhook configurations.
// It allows mocking the webhook update functionality during testing.
type WebhookConfigFunction func(webhookID, appName, envVarName, containerName string) error

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

	// UpdateWebhookConfig is a function that updates environment variables and restarts
	// containers when webhook configurations change
	UpdateWebhookConfig WebhookConfigFunction

	// Config is the loaded configuration from config.json
	Config *Config

	// ConfigPath is the path to the configuration file
	ConfigPath string
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

// log conditionally logs a message based on test detection.
// Messages are suppressed when running tests against the ngrok URL.
func (c *Client) log(message string) {
	if !strings.Contains(c.ServerURL, TestUrlPattern) {
		fmt.Println(message)
	}
}

// logf conditionally logs a formatted message based on test detection.
// Messages are suppressed when running tests against the ngrok URL.
func (c *Client) logf(format string, args ...interface{}) {
	if !strings.Contains(c.ServerURL, TestUrlPattern) {
		fmt.Printf(format, args...)
	}
}

// NewClient creates a new Mattermost client with the specified connection parameters.
// It initializes the API client and sets default webhook URLs for integrated applications.
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

	// Set up the default webhook configuration implementation
	client.UpdateWebhookConfig = client.updateWebhookConfigImpl

	// Load the configuration if possible
	config, err := LoadConfig(configPath)
	if err == nil {
		client.Config = config
	} else {
		// Log the error but continue - we'll use defaults
		client.logf("❌ Failed to load config file: %v\n", err)
	}

	return client
}

// Login authenticates with the Mattermost server
func (c *Client) Login() error {
	_, resp, err := c.API.Login(context.Background(), c.AdminUser, c.AdminPass)
	return handleAPIError("login failed", err, resp)
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

// ensureUserIsAdmin ensures a user has system_admin role
func (c *Client) ensureUserIsAdmin(user *model.User) error {
	if !strings.Contains(user.Roles, "system_admin") {
		// Use UpdateUserRoles API to directly assign system_admin role
		_, err := c.API.UpdateUserRoles(context.Background(), user.Id, "system_admin system_user")
		if err != nil {
			c.logf("❌ Failed to assign system_admin role to user '%s': %v\n", user.Username, err)
			return err
		}
		c.logf("✅ Successfully assigned system_admin role to user '%s'\n", user.Username)
	}
	return nil
}

// createOrGetUser creates a new user or returns an existing one
func (c *Client) createOrGetUser(username, email, password, nickname string, isAdmin bool) (*model.User, error) {
	// Check if user already exists
	existingUsers, resp, err := c.API.GetUsers(context.Background(), 0, 1000, "")
	if err != nil {
		return nil, handleAPIError("failed to get users", err, resp)
	}

	// Look for existing user
	for _, user := range existingUsers {
		if user.Username == username {
			c.logf("User '%s' already exists\n", username)

			// Ensure admin status if needed
			if isAdmin {
				if err := c.ensureUserIsAdmin(user); err != nil {
					return user, err // Still return the user even if admin role update fails
				}
			}

			return user, nil
		}
	}

	// Create the user
	c.logf("Creating %s user...\n", username)

	// Set roles if system admin
	roles := "system_user" // Always include system_user
	if isAdmin {
		roles = "system_admin system_user"
	}

	newUser := &model.User{
		Username: username,
		Email:    email,
		Password: password,
		Nickname: nickname,
		Roles:    roles,
	}

	createdUser, resp, err := c.API.CreateUser(context.Background(), newUser)
	if err != nil {
		return nil, handleAPIError(fmt.Sprintf("failed to create user '%s'", username), err, resp)
	}

	c.logf("✅ Successfully created user '%s' (ID: %s)\n", createdUser.Username, createdUser.Id)
	return createdUser, nil
}

// CreateUsers creates test users from the config file if they don't exist.
// If no config file is available, it falls back to creating default users.
func (c *Client) CreateUsers() error {
	// If we have a config file, use it to create users
	if c.Config != nil && len(c.Config.Users) > 0 {
		c.log("Creating users from configuration file...")

		// Process each user from the config
		for _, userConfig := range c.Config.Users {
			_, err := c.createOrGetUser(
				userConfig.Username,
				userConfig.Email,
				userConfig.Password,
				userConfig.Nickname,
				userConfig.IsSystemAdmin,
			)
			if err != nil {
				c.logf("❌ Error with user '%s': %v\n", userConfig.Username, err)
			}
		}

		return nil
	}

	// No default users if no config is available - we rely on the systemadmin user
	c.log("No configuration file found. Using only default admin user.")
	return nil
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
			c.logf("Team '%s' already exists\n", teamName)
			return team, nil
		}
	}

	// Create the team
	c.logf("Creating '%s' team...\n", teamName)

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

	c.logf("✅ Successfully created team '%s' (ID: %s)\n", createdTeam.Name, createdTeam.Id)
	return createdTeam, nil
}

// CreateTeam creates teams from config or a default team if no config available
// It also sets up webhooks and slash commands for each team as it's created
func (c *Client) CreateTeam() error {
	// Get all existing teams for user assignments
	existingTeams, resp, err := c.API.GetAllTeams(context.Background(), "", 0, 100)
	if err != nil {
		return handleAPIError("failed to get teams", err, resp)
	}

	// Create a map of existing teams for quick lookup
	existingTeamMap := make(map[string]*model.Team)
	for _, team := range existingTeams {
		existingTeamMap[team.Name] = team
	}

	// If we have a config file with teams, use it to create teams
	if c.Config != nil && len(c.Config.Teams) > 0 {
		c.log("Creating teams from configuration file...")

		// Process each team from the config
		for _, teamConfig := range c.Config.Teams {
			team, err := c.createOrGetTeam(
				teamConfig.Name,
				teamConfig.DisplayName,
				teamConfig.Description,
				teamConfig.Type,
			)
			if err != nil {
				c.logf("❌ Error with team '%s': %v\n", teamConfig.Name, err)
				continue
			}

			// Update the team maps
			existingTeamMap[team.Name] = team

			// Set up webhooks and slash commands for this team
			if err := c.setupTeamResources(team, teamConfig); err != nil {
				c.logf("⚠️ Warning: Error setting up resources for team '%s': %v\n", teamConfig.Name, err)
			}
		}
	} else {
		// Fallback to creating the default team if it doesn't exist
		c.log("No team configuration found. Creating default team...")

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

	// Add users to teams according to config
	if err := c.AddUsersToTeams(existingTeamMap); err != nil {
		return err
	}

	return nil
}

// addUserToTeam adds a user to a team and handles common error cases
func (c *Client) addUserToTeam(user *model.User, team *model.Team) error {
	_, teamResp, err := c.API.AddTeamMember(context.Background(), team.Id, user.Id)
	if err != nil {
		// Check if the error is because the user is already a member
		if teamResp != nil && teamResp.StatusCode == 400 {
			c.logf("User '%s' is already a member of team '%s'\n", user.Username, team.Name)
			return nil
		}
		return fmt.Errorf("failed to add user '%s' to team '%s': %w", user.Username, team.Name, err)
	}

	c.logf("✅ Added user '%s' to team '%s'\n", user.Username, team.Name)
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
		c.log("Adding users to teams from configuration...")

		// Loop through each user in config
		for _, userConfig := range c.Config.Users {
			// Skip if user doesn't exist
			user, exists := userMap[userConfig.Username]
			if !exists {
				c.logf("❌ User '%s' not found, can't add to teams\n", userConfig.Username)
				continue
			}

			// Add user to each of their teams
			for _, teamName := range userConfig.Teams {
				// Skip if team doesn't exist
				team, exists := teamMap[teamName]
				if !exists {
					c.logf("❌ Team '%s' not found, can't add user '%s'\n", teamName, userConfig.Username)
					continue
				}

				// Add user to team with error handling
				if err := c.addUserToTeam(user, team); err != nil {
					c.logf("❌ %v\n", err)
				}
			}
		}
	} else {
		// Fallback to adding only default admin user to default team
		c.log("No user configuration found. Adding only default admin to default team...")

		// Get default team
		team, exists := teamMap[c.TeamName]
		if !exists {
			return fmt.Errorf("default team '%s' not found", c.TeamName)
		}

		// Add default admin user to the default team
		user, exists := userMap[DefaultAdminUsername]
		if !exists {
			c.logf("❌ Default admin user '%s' not found\n", DefaultAdminUsername)
			return nil
		}

		// Add admin user to team with error handling
		if err := c.addUserToTeam(user, team); err != nil {
			c.logf("❌ %v\n", err)
		}
	}

	return nil
}

// createOrGetSlashCommand creates a slash command or returns if it already exists
func (c *Client) createOrGetSlashCommand(teamID string, cmd *model.Command) (*model.Command, error) {
	// Get existing commands
	commands, resp, err := c.API.ListCommands(context.Background(), teamID, true)
	if err != nil {
		return nil, handleAPIError("failed to list commands", err, resp)
	}

	// Check if command already exists
	for _, existingCmd := range commands {
		if existingCmd.Trigger == cmd.Trigger {
			c.logf("/%s command already exists\n", cmd.Trigger)
			return existingCmd, nil
		}
	}

	// Create the command
	c.logf("Creating /%s slash command...\n", cmd.Trigger)

	createdCmd, resp, err := c.API.CreateCommand(context.Background(), cmd)
	if err != nil {
		return nil, handleAPIError(fmt.Sprintf("failed to create /%s command", cmd.Trigger), err, resp)
	}

	c.logf("✅ /%s command created successfully (ID: %s)\n", createdCmd.Trigger, createdCmd.Id)
	return createdCmd, nil
}

// CreateSlashCommand creates a single slash command
func (c *Client) CreateSlashCommand(teamID, trigger, url, displayName, description, username, autoCompleteHint string) error {
	// Create command with autocomplete enabled
	cmd := &model.Command{
		TeamId:           teamID,
		Trigger:          trigger,
		Method:           "P",
		URL:              url,
		CreatorId:        "", // Will be set to current user
		DisplayName:      displayName,
		Description:      description,
		AutoComplete:     true,
		AutoCompleteDesc: description,
		AutoCompleteHint: autoCompleteHint,
		Username:         username,
	}

	_, err := c.createOrGetSlashCommand(teamID, cmd)
	return err
}

// CreateSlashCommandFromConfig creates a slash command from configuration
// It takes command configuration from the config and applies it to create a slash command
func (c *Client) CreateSlashCommandFromConfig(teamID string, cmdConfig SlashCommandConfig) error {
	// Set default values for optional fields
	autoCompleteDesc := cmdConfig.Description // Default to the description
	if cmdConfig.AutoCompleteDesc != "" {
		autoCompleteDesc = cmdConfig.AutoCompleteDesc
	}

	// Create the command with configuration values
	cmd := &model.Command{
		TeamId:           teamID,
		Trigger:          cmdConfig.Trigger,
		Method:           "P",
		URL:              cmdConfig.URL,
		CreatorId:        "", // Will be set to current user
		DisplayName:      cmdConfig.DisplayName,
		Description:      cmdConfig.Description,
		AutoComplete:     cmdConfig.AutoComplete,
		AutoCompleteDesc: autoCompleteDesc,
		AutoCompleteHint: cmdConfig.AutoCompleteHint,
		Username:         cmdConfig.Username,
	}

	_, err := c.createOrGetSlashCommand(teamID, cmd)
	return err
}

// updateWebhookConfigImpl updates an app's webhook configuration in the environment.
// This method:
// 1. Updates the webhook URL in the env_vars.env file
// 2. Restarts the associated Docker container so it picks up the new URL
//
// Parameters:
//   - webhookID: The ID of the created webhook in Mattermost
//   - appName: Display name of the app for logging purposes
//   - envVarName: Name of the environment variable to update
//   - containerName: Name of the Docker container to restart
//
// Returns an error if any step in the process fails.
func (c *Client) updateWebhookConfigImpl(webhookID, appName, envVarName, containerName string) error {
	c.logf("✅ Created webhook with ID: %s for %s\n", webhookID, appName)

	// Update env_vars.env file with the webhook URL
	webhookURL := fmt.Sprintf("http://mattermost:8065/hooks/%s", webhookID)
	c.logf("Setting webhook URL: %s for %s\n", webhookURL, envVarName)

	// Read the env file
	data, err := os.ReadFile(EnvFile)
	if err != nil {
		return fmt.Errorf("failed to read env file: %v", err)
	}

	// Replace the line with the new webhook URL
	lines := strings.Split(string(data), "\n")
	for i, line := range lines {
		if strings.HasPrefix(line, envVarName+"=") {
			lines[i] = fmt.Sprintf("%s=%s", envVarName, webhookURL)
			break
		}
	}

	// Write the updated content back to the file
	err = os.WriteFile(EnvFile, []byte(strings.Join(lines, "\n")), 0644)
	if err != nil {
		return fmt.Errorf("failed to write env file: %v", err)
	}

	c.logf("Updated env_vars.env with webhook URL for %s\n", appName)

	// Check if container exists and is running before trying to restart
	checkCmd := exec.Command("docker", "ps", "-q", "-f", "name="+containerName)
	output, err := checkCmd.Output()
	if err != nil || len(output) == 0 {
		c.logf("❌ Container %s not found or not running, skipping restart\n", containerName)
		return nil
	}

	// Restart the app container
	c.logf("Restarting %s container...\n", containerName)
	cmd := exec.Command("docker", "restart", containerName)
	if err := cmd.Run(); err != nil {
		c.logf("❌ Failed to restart container %s: %v\n", containerName, err)
		// Don't return error here, just log the warning
	} else {
		c.logf("✅ %s restarted successfully\n", appName)
	}

	return nil
}

// createOrGetWebhook creates a webhook or returns an existing one with the same name
func (c *Client) createOrGetWebhook(channelID, displayName, description, username string) (*model.IncomingWebhook, error) {
	// Check if webhook already exists
	hooks, resp, err := c.API.GetIncomingWebhooks(context.Background(), 0, 1000, "")
	if err != nil {
		return nil, handleAPIError("failed to get webhooks", err, resp)
	}

	for _, hook := range hooks {
		if hook.DisplayName == displayName {
			c.logf("Webhook '%s' already exists\n", displayName)
			return hook, nil
		}
	}

	// Create the webhook
	c.logf("Creating incoming webhook '%s'...\n", displayName)
	hook := &model.IncomingWebhook{
		ChannelId:   channelID,
		DisplayName: displayName,
		Description: description,
		Username:    username,
	}

	newHook, resp, err := c.API.CreateIncomingWebhook(context.Background(), hook)
	if err != nil {
		return nil, handleAPIError("failed to create webhook", err, resp)
	}

	return newHook, nil
}

// CreateWebhookFromConfig creates an incoming webhook from configuration
// It takes webhook configuration from the config and applies it to create a webhook
// Additionally, it updates environment variables and restarts containers as needed
func (c *Client) CreateWebhookFromConfig(channelID string, webhookConfig WebhookConfig) error {
	// Create the webhook
	newHook, err := c.createOrGetWebhook(
		channelID,
		webhookConfig.DisplayName,
		webhookConfig.Description,
		webhookConfig.Username,
	)
	if err != nil {
		return fmt.Errorf("failed to create '%s' webhook: %w", webhookConfig.DisplayName, err)
	}

	// If webhook already existed but we didn't get its ID, we can't update the config
	if newHook == nil || newHook.Id == "" {
		return fmt.Errorf("webhook created but no ID returned")
	}

	// Update the webhook configuration using the function reference
	if err := c.UpdateWebhookConfig(
		newHook.Id,
		webhookConfig.DisplayName,
		webhookConfig.EnvVariable,
		webhookConfig.ContainerName,
	); err != nil {
		return fmt.Errorf("failed to update webhook config for '%s': %w", webhookConfig.DisplayName, err)
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

// setupTeamResources sets up channels, webhooks and slash commands for a specific team
// This is called immediately after creating or finding a team to ensure resources are set up
func (c *Client) setupTeamResources(team *model.Team, teamConfig TeamConfig) error {
	if team == nil {
		return fmt.Errorf("team is nil, cannot set up resources")
	}

	teamID := team.Id
	c.logf("Setting up resources for team '%s' (ID: %s)\n", team.Name, teamID)

	// Set up channels for this team first (webhooks need channels to exist)
	if err := c.setupTeamChannels(team, teamConfig); err != nil {
		return fmt.Errorf("failed to set up channels for team '%s': %w", team.Name, err)
	}

	// Set up webhooks for this team
	if err := c.setupTeamWebhooks(team, teamConfig); err != nil {
		return fmt.Errorf("failed to set up webhooks for team '%s': %w", team.Name, err)
	}

	// Set up slash commands for this team
	if err := c.setupTeamSlashCommands(team, teamConfig); err != nil {
		return fmt.Errorf("failed to set up slash commands for team '%s': %w", team.Name, err)
	}

	return nil
}

// setupTeamWebhooks creates webhooks for a specific team
func (c *Client) setupTeamWebhooks(team *model.Team, teamConfig TeamConfig) error {
	teamID := team.Id
	c.logf("Setting up webhooks for team '%s' (ID: %s)\n", team.Name, teamID)

	// Get the channels for this team
	channels, _, err := c.API.GetPublicChannelsForTeam(context.Background(), teamID, 0, 100, "")
	if err != nil {
		return fmt.Errorf("failed to get channels for team '%s': %w", team.Name, err)
	}

	// Create a map of channel name to channel ID for quick lookup
	channelMap := make(map[string]*model.Channel)
	for _, channel := range channels {
		channelMap[channel.Name] = channel
	}

	// Set up webhooks for this team
	for _, webhook := range teamConfig.Webhooks {
		// Verify the channel exists
		channel, exists := channelMap[webhook.ChannelName]
		if !exists {
			c.logf("❌ Channel '%s' not found in team '%s', skipping webhook '%s'\n",
				webhook.ChannelName, team.Name, webhook.DisplayName)
			continue
		}

		c.logf("Creating webhook '%s' in channel '%s'\n", webhook.DisplayName, webhook.ChannelName)

		// Create the webhook using the configuration
		err := c.CreateWebhookFromConfig(channel.Id, webhook)
		if err != nil {
			c.logf("❌ Failed to create webhook '%s': %v\n", webhook.DisplayName, err)
			continue
		}

		c.logf("✅ Successfully created webhook '%s' in channel '%s'\n",
			webhook.DisplayName, webhook.ChannelName)
	}

	return nil
}

// setupTeamSlashCommands creates slash commands for a specific team
func (c *Client) setupTeamSlashCommands(team *model.Team, teamConfig TeamConfig) error {
	teamID := team.Id
	c.logf("Setting up slash commands for team '%s' (ID: %s)\n", team.Name, teamID)

	// Set up slash commands for this team
	for _, cmd := range teamConfig.SlashCommands {
		c.logf("Creating /%s slash command for team '%s'\n", cmd.Trigger, team.Name)

		// Create the slash command using the configuration
		err := c.CreateSlashCommandFromConfig(teamID, cmd)
		if err != nil {
			c.logf("❌ Failed to create /%s command: %v\n", cmd.Trigger, err)
			continue
		}
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
			c.logf("❌ Warning: Failed to get private channels: %v\n", privateErr)
		}
	}

	// Check if channel already exists
	for _, channel := range channels {
		if channel.Name == name {
			c.logf("Channel '%s' already exists in team\n", name)
			return channel, nil
		}
	}

	// Default to Open if not specified
	if channelType == "" {
		channelType = "O"
	}

	// Create the channel
	c.logf("Creating '%s' channel...\n", name)

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

	c.logf("✅ Successfully created channel '%s' (ID: %s)\n", createdChannel.Name, createdChannel.Id)
	return createdChannel, nil
}

// addUserToChannel adds a user to a channel and handles common error cases
func (c *Client) addUserToChannel(userID, channelID string) error {
	_, resp, err := c.API.AddChannelMember(context.Background(), channelID, userID)
	if err != nil {
		// Check if the error is because the user is already a member
		if resp != nil && resp.StatusCode == 400 {
			// Look for the "already a member" message
			c.logf("User is already a member of the channel\n")
			return nil
		}
		return fmt.Errorf("failed to add user to channel: %w", err)
	}

	c.logf("✅ Added user to channel successfully\n")
	return nil
}

// categorizeChannelAPI implements channel categorization using the Playbooks API
func (c *Client) categorizeChannelAPI(channelID string, categoryName string) error {
	if channelID == "" || categoryName == "" {
		return fmt.Errorf("channel ID and category name are required")
	}

	c.logf("Categorizing channel %s in category '%s' using Playbooks API...\n", channelID, categoryName)

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
	defer resp.Body.Close()

	// Check response
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("categorize request failed with status %d: %s", resp.StatusCode, string(body))
	}

	c.logf("✅ Successfully categorized channel in '%s' using Playbooks API\n", categoryName)
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
	c.logf("Setting up channels for team '%s' (ID: %s)\n", team.Name, teamID)

	// Skip if no channels are configured
	if len(teamConfig.Channels) == 0 {
		c.logf("No channels configured for team '%s'\n", team.Name)
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
			c.logf("❌ Failed to create channel '%s': %v\n", channelConfig.Name, err)
			continue
		}

		// If a category is specified, categorize the channel before adding members
		if channelConfig.Category != "" {
			if err := c.categorizeChannel(channel.Id, channelConfig.Category); err != nil {
				c.logf("⚠️ Warning: Failed to categorize channel '%s' in category '%s': %v\n",
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

	c.log("Adding members to channels...")

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
			c.logf("❌ Team '%s' not found, can't add channel members\n", teamName)
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
				c.logf("❌ Failed to get channels for team '%s': %v\n", teamName, err)
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
				c.logf("❌ Channel '%s' not found in team '%s'\n", channelConfig.Name, teamName)
				continue
			}

			// Add members to the channel
			c.logf("Adding %d members to channel '%s'\n", len(channelConfig.Members), channelConfig.Name)

			for _, username := range channelConfig.Members {
				user, exists := userMap[username]
				if !exists {
					c.logf("❌ User '%s' not found, can't add to channel '%s'\n", username, channelConfig.Name)
					continue
				}

				if err := c.addUserToChannel(user.Id, channel.Id); err != nil {
					c.logf("❌ Failed to add user '%s' to channel '%s': %v\n", username, channelConfig.Name, err)
				} else {
					c.logf("✅ Added user '%s' to channel '%s'\n", username, channelConfig.Name)
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

	c.log("Setting up channel commands...")

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
			c.logf("❌ Team '%s' not found, can't execute commands\n", teamName)
			return fmt.Errorf("team '%s' not found", teamName)
		}

		// Get all channels for this team
		channels, _, err := c.API.GetPublicChannelsForTeam(context.Background(), team.Id, 0, 1000, "")
		if err != nil {
			c.logf("❌ Failed to get channels for team '%s': %v\n", teamName, err)
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
				c.logf("❌ Channel '%s' not found in team '%s'\n", channelConfig.Name, teamName)
				return fmt.Errorf("channel '%s' not found in team '%s'", channelConfig.Name, teamName)
			}

			c.logf("Executing %d commands for channel '%s' (sequentially)...\n",
				len(channelConfig.Commands), channelConfig.Name)

			// Execute each command in order, waiting for each to complete
			for i, command := range channelConfig.Commands {
				// Check if the command has been loaded and trimmed
				commandText := strings.TrimSpace(command)

				if !strings.HasPrefix(commandText, "/") {
					c.logf("❌ Invalid command '%s' for channel '%s' - must start with /\n",
						commandText, channelConfig.Name)
					return fmt.Errorf("invalid command '%s' - must start with /", commandText)
				}

				// Remove the leading slash for the API

				c.logf("Executing command %d/%d in channel '%s': %s\n",
					i+1, len(channelConfig.Commands), channelConfig.Name, commandText)

				// Execute the command using the commands/execute API
				_, resp, err := c.API.ExecuteCommand(context.Background(), channel.Id, commandText)

				// Check for any errors or non-200 response
				if err != nil {
					c.logf("❌ Failed to execute command '%s': %v\n", commandText, err)
					return fmt.Errorf("failed to execute command '%s': %w", commandText, err)
				}

				if resp.StatusCode != 200 {
					c.logf("❌ Command '%s' returned non-200 status code: %d\n",
						commandText, resp.StatusCode)
					return fmt.Errorf("command '%s' returned status code %d",
						commandText, resp.StatusCode)
				}

				c.logf("✅ Successfully executed command %d/%d: '%s' in channel '%s'\n",
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
	c.log("===========================================")
	c.log("Setting up test Data for Mattermost")
	c.log("===========================================")

	// Load configuration if not already loaded
	if c.Config == nil {
		config, err := LoadConfig(c.ConfigPath)
		if err != nil {
			c.logf("❌ Failed to load config: %v, using defaults\n", err)
		} else {
			c.Config = config
		}
	}

	if err := c.CreateUsers(); err != nil {
		return err
	}

	// Create teams and set up webhooks and slash commands
	// The CreateTeam function now handles webhook and slash command setup
	if err := c.CreateTeam(); err != nil {
		return err
	}

	// Now that teams are created and users added to teams,
	// we can add users to channels
	if err := c.AddChannelMembers(); err != nil {
		c.logf("❌ Warning: Error adding channel members: %v\n", err)
		// Don't return error here, continue with setup
	}

	// Now that channels are fully set up with members,
	// we can execute the channel commands
	if err := c.SetupChannelCommands(); err != nil {
		c.logf("❌ Error executing channel commands: %v\n", err)
		// Return error here to abort the setup process
		return fmt.Errorf("failed to execute channel commands: %w", err)
	}

	return nil
}

// CreateDefaultAdminUser creates the single default admin user using mmctl
func (c *Client) CreateDefaultAdminUser() error {
	c.log("Creating default system admin user with mmctl...")

	// Use Docker exec to run mmctl command
	cmd := exec.Command("docker", "exec", "-i", "mattermost", "mmctl", "user", "create",
		"--email", DefaultAdminUsername+"@example.com",
		"--username", DefaultAdminUsername,
		"--password", DefaultAdminPassword,
		"--system-admin",
		"--local")

	output, err := cmd.CombinedOutput()
	if err != nil {
		// Check if error is because user already exists (which is fine)
		if strings.Contains(string(output), "already exists") {
			c.log("Default admin user already exists, continuing with setup...")
			return nil
		}
		return fmt.Errorf("failed to create default admin user with mmctl: %v, output: %s", err, output)
	}

	c.log("✅ Successfully created default admin user with mmctl")
	return nil
}

// Setup performs the main setup based on configuration
func (c *Client) Setup() error {
	// Safety check - make sure the client and API are properly initialized
	if c == nil || c.API == nil {
		return fmt.Errorf("client not properly initialized")
	}

	// Load configuration if not already loaded
	if c.Config == nil && c.ConfigPath != "" {
		config, err := LoadConfig(c.ConfigPath)
		if err != nil {
			c.logf("Failed to load config from %s: %v\n", c.ConfigPath, err)
			// Try default path if specific path fails
			config, err = LoadConfig(DefaultConfigPath)
			if err != nil {
				c.logf("Failed to load config from default path: %v\n", err)
			} else {
				c.Config = config
			}
		} else {
			c.Config = config
		}
	}

	if err := c.WaitForStart(); err != nil {
		return err
	}

	// Create default admin user with mmctl before attempting login
	if err := c.CreateDefaultAdminUser(); err != nil {
		return err
	}

	if err := c.Login(); err != nil {
		return err
	}

	if err := c.SetupTestData(); err != nil {
		return err
	}

	c.log("Alright, everything seems to be setup and running. Enjoy.")
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
