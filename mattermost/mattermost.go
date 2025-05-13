// Package mattermost provides tools for setting up and configuring a Mattermost server
package mattermost

import (
	// Standard library imports
	"context"
	"fmt"
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

	// FlightAppURL is the webhook URL for the flight tracking app
	FlightAppURL string

	// WeatherAppURL is the webhook URL for the weather app
	WeatherAppURL string

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
			return fmt.Errorf("%s: %v, response status code: %v", operation, err, resp.StatusCode)
		}
		return fmt.Errorf("%s: %v", operation, err)
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
		API:           model.NewAPIv4Client(serverURL),
		ServerURL:     serverURL,
		AdminUser:     adminUser,
		AdminPass:     adminPass,
		TeamName:      teamName,
		FlightAppURL:  "http://flightaware-app:8086/webhook",
		WeatherAppURL: "http://weather-app:8085/webhook",
		ConfigPath:    configPath,
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
	c.logf("Waiting %d seconds for the server to start\n", MaxWaitSeconds)

	for i := 0; i < MaxWaitSeconds; i++ {
		_, resp, err := c.API.GetPing(context.Background())
		if err == nil && resp != nil && resp.StatusCode == 200 {
			c.log("Server started")
			return nil
		}
		c.logf(".")
		time.Sleep(1 * time.Second)
	}

	return fmt.Errorf("server didn't start in %d seconds", MaxWaitSeconds)
}

// ensureUserIsAdmin ensures a user has system_admin role
func (c *Client) ensureUserIsAdmin(user *model.User, username string) {
	if !strings.Contains(user.Roles, "system_admin") {
		// Use UpdateUserRoles API to directly assign system_admin role
		_, err := c.API.UpdateUserRoles(context.Background(), user.Id, "system_admin system_user")
		if err != nil {
			c.logf("❌ Failed to assign system_admin role to user '%s': %v\n", username, err)
		} else {
			c.logf("✅ Successfully assigned system_admin role to user '%s'\n", username)
		}
	}
}

// CreateUsers creates test users from the config file if they don't exist.
// If no config file is available, it falls back to creating default users.
func (c *Client) CreateUsers() error {
	// Get existing users from the server
	existingUsers, resp, err := c.API.GetUsers(context.Background(), 0, 1000, "")
	if err != nil {
		return handleAPIError("failed to get users", err, resp)
	}

	// Create a map of existing users for quick lookup
	existingUserMap := make(map[string]*model.User)
	for _, user := range existingUsers {
		existingUserMap[user.Username] = user
	}

	// If we have a config file, use it to create users
	if c.Config != nil && len(c.Config.Users) > 0 {
		c.log("Creating users from configuration file...")

		// Process each user from the config
		for _, userConfig := range c.Config.Users {
			// Check if user already exists
			if existingUser, exists := existingUserMap[userConfig.Username]; exists {
				c.logf("User '%s' already exists\n", userConfig.Username)

				// Ensure user has admin role if needed
				if userConfig.IsSystemAdmin {
					c.ensureUserIsAdmin(existingUser, userConfig.Username)
				}
				continue
			}

			// Create the user
			c.logf("Creating %s user...\n", userConfig.Username)

			// Set roles if system admin
			roles := "system_user" // Always include system_user
			if userConfig.IsSystemAdmin {
				roles = "system_admin system_user"
			}

			newUser := &model.User{
				Username: userConfig.Username,
				Email:    userConfig.Email,
				Password: userConfig.Password,
				Nickname: userConfig.Nickname,
				Roles:    roles,
			}

			createdUser, _, err := c.API.CreateUser(context.Background(), newUser)
			if err != nil {
				c.logf("❌ Failed to create user '%s': %v\n", userConfig.Username, err)
				continue
			}

			// Ensure the new user has admin role if needed
			if userConfig.IsSystemAdmin {
				c.ensureUserIsAdmin(createdUser, createdUser.Username)
			}

			c.logf("✅ Successfully created user '%s' (ID: %s)\n", createdUser.Username, createdUser.Id)

			// Add the user to the map of existing users
			existingUserMap[createdUser.Username] = createdUser
		}

		return nil
	}

	// No default users if no config is available - we rely on the systemadmin user
	c.log("No configuration file found. Using only default admin user.")

	return nil
}

// CreateTeam creates teams from config or a default team if no config available
func (c *Client) CreateTeam() error {
	// Get all existing teams
	existingTeams, resp, err := c.API.GetAllTeams(context.Background(), "", 0, 100)
	if err != nil {
		return handleAPIError("failed to get teams", err, resp)
	}

	// Create a map of existing teams for quick lookup
	existingTeamMap := make(map[string]*model.Team)
	for _, team := range existingTeams {
		existingTeamMap[team.Name] = team
	}

	// Store created teams for user assignment later
	createdTeams := make(map[string]*model.Team)

	// If we have a config file with teams, use it to create teams
	if c.Config != nil && len(c.Config.Teams) > 0 {
		c.log("Creating teams from configuration file...")

		// Process each team from the config
		for teamName, teamConfig := range c.Config.Teams {
			// Check if team already exists
			if existingTeam, exists := existingTeamMap[teamConfig.Name]; exists {
				c.logf("Team '%s' already exists\n", teamConfig.Name)
				createdTeams[teamName] = existingTeam
				continue
			}

			// Create the team
			c.logf("Creating %s team...\n", teamConfig.Name)

			// Default to Open type if not specified
			teamType := model.TeamOpen
			if teamConfig.Type != "" {
				teamType = teamConfig.Type
			}

			newTeam := &model.Team{
				Name:        teamConfig.Name,
				DisplayName: teamConfig.DisplayName,
				Description: teamConfig.Description,
				Type:        teamType,
			}

			createdTeam, _, err := c.API.CreateTeam(context.Background(), newTeam)
			if err != nil {
				c.logf("❌ Failed to create team '%s': %v\n", teamConfig.Name, err)
				continue
			}

			c.logf("✅ Successfully created team '%s' (ID: %s)\n", createdTeam.Name, createdTeam.Id)
			createdTeams[teamName] = createdTeam
			existingTeamMap[createdTeam.Name] = createdTeam
		}
	} else {
		// Fallback to creating the default team if it doesn't exist
		c.log("No team configuration found. Creating default team...")

		// Check if default team exists
		teamExists := false
		var team *model.Team

		for _, t := range existingTeams {
			if t.Name == c.TeamName {
				teamExists = true
				team = t
				break
			}
		}

		// Create team if needed
		if !teamExists {
			c.logf("Creating %s team...\n", c.TeamName)
			newTeam := &model.Team{
				Name:        c.TeamName,
				DisplayName: "Test Team",
				Type:        model.TeamOpen,
			}

			var createResp *model.Response
			team, createResp, err = c.API.CreateTeam(context.Background(), newTeam)
			if err != nil {
				return handleAPIError("failed to create team", err, createResp)
			}
			c.logf("✅ Successfully created team '%s' (ID: %s)\n", team.Name, team.Id)
		} else {
			c.logf("Team '%s' already exists\n", c.TeamName)
		}

		// Add default team to the created teams map
		createdTeams[c.TeamName] = team
	}

	// Return if we couldn't create any teams
	if len(createdTeams) == 0 && len(existingTeamMap) == 0 {
		return fmt.Errorf("no teams could be created or found")
	}

	// Add users to teams according to config
	return c.AddUsersToTeams(existingTeamMap)
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

				// Add user to team
				_, teamResp, err := c.API.AddTeamMember(context.Background(), team.Id, user.Id)
				if err != nil {
					// Check if the error is because the user is already a member
					if teamResp.StatusCode == 400 {
						c.logf("User '%s' is already a member of team '%s'\n", userConfig.Username, teamName)
						continue
					}
					c.logf("❌ Failed to add user '%s' to team '%s': %v\n", userConfig.Username, teamName, err)
					continue
				}

				c.logf("✅ Added user '%s' to team '%s'\n", userConfig.Username, teamName)
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

		_, teamResp, err := c.API.AddTeamMember(context.Background(), team.Id, user.Id)
		if err != nil {
			// Check if the error is because the user is already a member
			if teamResp.StatusCode == 400 {
				c.logf("Default admin user is already a member of team '%s'\n", c.TeamName)
				return nil
			}
			c.logf("❌ Failed to add default admin user to team '%s': %v\n", c.TeamName, err)
			return nil
		}

		c.logf("✅ Added default admin user to team '%s'\n", c.TeamName)
	}

	return nil
}

// CreateSlashCommand creates a single slash command
func (c *Client) CreateSlashCommand(teamID, trigger, url, displayName, description, username string) error {
	// Get existing commands
	commands, resp, err := c.API.ListCommands(context.Background(), teamID, true)
	if err != nil {
		return handleAPIError("failed to list commands", err, resp)
	}

	// Check if command already exists
	for _, cmd := range commands {
		if cmd.Trigger == trigger {
			c.logf("/%s command already exists\n", trigger)
			return nil
		}
	}

	// Create command with autocomplete enabled
	c.logf("Creating /%s slash command...\n", trigger)
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
		AutoCompleteHint: getAutoCompleteHint(trigger),
		Username:         username,
	}

	createdCmd, resp, err := c.API.CreateCommand(context.Background(), cmd)
	if err != nil {
		return handleAPIError(fmt.Sprintf("failed to create %s command", trigger), err, resp)
	}

	c.logf("✅ /%s command created successfully (ID: %s)\n", createdCmd.Trigger, createdCmd.Id)
	return nil
}

// getAutoCompleteHint provides appropriate command hints for slash command autocomplete
func getAutoCompleteHint(trigger string) string {
	switch trigger {
	case "flights":
		return "[departures/subscribe/unsubscribe/list] [--airport AIRPORT] [--frequency SECONDS]"
	case "weather":
		return "[location] [--subscribe] [--unsubscribe] [--update-frequency TIME]"
	default:
		return ""
	}
}

// CreateFlightCommand creates the flights slash command
func (c *Client) CreateFlightCommand(teamID string) error {
	return c.CreateSlashCommand(
		teamID,
		"flights",
		c.FlightAppURL,
		"Flight Departures",
		"Get flight departures or subscribe to airport updates",
		"flight-bot",
	)
}

// CreateWeatherCommand creates the weather slash command
func (c *Client) CreateWeatherCommand(teamID string) error {
	return c.CreateSlashCommand(
		teamID,
		"weather",
		c.WeatherAppURL,
		"Weather Information",
		"Get current weather information for a location",
		"weather-bot",
	)
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

// CreateWebhook creates a single incoming webhook
func (c *Client) CreateWebhook(channelID, displayName, description, username string) (*model.IncomingWebhook, error) {
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

// CreateAppWebhook creates an incoming webhook for an app and updates its configuration
func (c *Client) CreateAppWebhook(channelID, appName, displayName, description, username, envVarName, containerName string) error {
	// Create the webhook
	newHook, err := c.CreateWebhook(channelID, displayName, description, username)
	if err != nil {
		return fmt.Errorf("failed to create %s webhook: %w", appName, err)
	}

	// If webhook already existed but we didn't get its ID, we can't update the config
	if newHook == nil || newHook.Id == "" {
		return fmt.Errorf("webhook created but no ID returned")
	}

	// Update the webhook configuration using the function reference
	if err := c.UpdateWebhookConfig(newHook.Id, appName, envVarName, containerName); err != nil {
		return fmt.Errorf("failed to update webhook config for %s: %w", appName, err)
	}

	return nil
}

// AppConfig holds the configuration for an app integration with Mattermost.
// It contains all parameters needed to create webhooks and commands for an application.
type AppConfig struct {
	// AppName is the display name of the application (e.g., "Weather app")
	AppName string

	// WebhookName is the name for the webhook in Mattermost
	WebhookName string

	// WebhookDesc is the description for the webhook
	WebhookDesc string

	// EnvVarName is the environment variable name to store the webhook URL
	// (e.g., "WEATHER_MATTERMOST_WEBHOOK_URL")
	EnvVarName string

	// ContainerName is the Docker container name to restart after config update
	ContainerName string

	// CommandCreator is a function that creates the slash command for this app
	// It takes a teamID and returns an error if creation fails
	CommandCreator func(string) error
}

// CreateApp sets up an app with webhook and slash command
func (c *Client) CreateApp(channelID, teamID string, config AppConfig) error {
	// Create webhook
	err := c.CreateAppWebhook(
		channelID,
		config.AppName,
		config.WebhookName,
		config.WebhookDesc,
		"professor", // Common username for app integrations
		config.EnvVarName,
		config.ContainerName,
	)
	if err != nil {
		return fmt.Errorf("failed to create %s webhook: %v", config.AppName, err)
	}

	// Create slash command
	if err := config.CommandCreator(teamID); err != nil {
		return fmt.Errorf("failed to create %s command: %v", config.AppName, err)
	}

	return nil
}

// CreateWeatherApp sets up the weather app (webhook and slash command)
func (c *Client) CreateWeatherApp(channelID, teamID string) error {
	return c.CreateApp(channelID, teamID, AppConfig{
		AppName:        "Weather app",
		WebhookName:    "weather",
		WebhookDesc:    "Weather responses",
		EnvVarName:     "WEATHER_MATTERMOST_WEBHOOK_URL",
		ContainerName:  "weather-app",
		CommandCreator: c.CreateWeatherCommand,
	})
}

// CreateFlightApp sets up the flight app (webhook and slash command)
func (c *Client) CreateFlightApp(channelID, teamID string) error {
	return c.CreateApp(channelID, teamID, AppConfig{
		AppName:        "Flight app",
		WebhookName:    "flight-app",
		WebhookDesc:    "Flight departures",
		EnvVarName:     "FLIGHTS_MATTERMOST_WEBHOOK_URL",
		ContainerName:  "flightaware-app",
		CommandCreator: c.CreateFlightCommand,
	})
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

// SetupWebhooks sets up webhooks and slash commands for all teams
func (c *Client) SetupWebhooks() error {
	// Get all existing teams
	teams, resp, err := c.API.GetAllTeams(context.Background(), "", 0, 100)
	if err != nil {
		return handleAPIError("failed to get teams", err, resp)
	}

	// Create a map of team name to team ID for quick lookup
	teamMap := make(map[string]*model.Team)
	for _, team := range teams {
		teamMap[team.Name] = team
	}

	// If we have team-specific configurations, use them
	if c.Config != nil && len(c.Config.Teams) > 0 {
		c.log("Setting up team-specific webhooks and slash commands from configuration")

		// Process each team from the config
		for _, teamConfig := range c.Config.Teams {
			// Verify the team exists
			team, exists := teamMap[teamConfig.Name]
			if !exists {
				c.logf("❌ Team '%s' not found, skipping webhook/command setup\n", teamConfig.Name)
				continue
			}

			teamID := team.Id
			c.logf("Setting up webhooks and slash commands for team '%s' (ID: %s)\n", teamConfig.Name, teamID)

			// Get the channels for this team
			channels, _, err := c.API.GetPublicChannelsForTeam(context.Background(), teamID, 0, 100, "")
			if err != nil {
				c.logf("❌ Failed to get channels for team '%s': %v\n", teamConfig.Name, err)
				continue
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
						webhook.ChannelName, teamConfig.Name, webhook.DisplayName)
					continue
				}

				c.logf("Creating webhook '%s' in channel '%s'\n", webhook.DisplayName, webhook.ChannelName)

				// Create the webhook
				err := c.CreateAppWebhook(
					channel.Id,
					webhook.DisplayName, // Use as both app name and webhook name
					webhook.DisplayName,
					webhook.Description,
					webhook.Username,
					webhook.EnvVariable,
					webhook.ContainerName,
				)

				if err != nil {
					c.logf("❌ Failed to create webhook '%s': %v\n", webhook.DisplayName, err)
					continue
				}

				c.logf("✅ Successfully created webhook '%s' in channel '%s'\n",
					webhook.DisplayName, webhook.ChannelName)
			}

			// Set up slash commands for this team
			for _, cmd := range teamConfig.SlashCommands {
				c.logf("Creating /%s slash command for team '%s'\n", cmd.Trigger, teamConfig.Name)

				// Determine autocomplete hint if not specified
				autoCompleteHint := cmd.AutoCompleteHint
				if autoCompleteHint == "" && cmd.AutoComplete {
					autoCompleteHint = getAutoCompleteHint(cmd.Trigger)
				}

				// Determine autocomplete description if not specified
				autoCompleteDesc := cmd.AutoCompleteDesc
				if autoCompleteDesc == "" && cmd.AutoComplete {
					autoCompleteDesc = cmd.Description
				}

				// Create the slash command
				slashCmd := &model.Command{
					TeamId:           teamID,
					Trigger:          cmd.Trigger,
					Method:           "P",
					URL:              cmd.URL,
					CreatorId:        "", // Will be set to current user
					DisplayName:      cmd.DisplayName,
					Description:      cmd.Description,
					AutoComplete:     cmd.AutoComplete,
					AutoCompleteDesc: autoCompleteDesc,
					AutoCompleteHint: autoCompleteHint,
					Username:         cmd.Username,
				}

				createdCmd, _, err := c.API.CreateCommand(context.Background(), slashCmd)
				if err != nil {
					c.logf("❌ Failed to create /%s command: %v\n", cmd.Trigger, err)
					continue
				}

				c.logf("✅ /%s command created successfully (ID: %s)\n", createdCmd.Trigger, createdCmd.Id)
			}
		}

		return nil
	}

	// Fallback to creating default webhooks if no config is available
	c.log("No team-specific configuration found. Setting up default apps...")

	// Find the default team
	team, exists := teamMap[c.TeamName]
	if !exists {
		return fmt.Errorf("default team '%s' not found", c.TeamName)
	}

	teamID := team.Id

	// Get the channels for the team
	channels, _, err := c.API.GetPublicChannelsForTeam(context.Background(), teamID, 0, 100, "")
	if err != nil {
		return handleAPIError("failed to get channels", err, resp)
	}

	// Find the off-topic channel
	var channelID string
	for _, channel := range channels {
		if channel.Name == "off-topic" {
			channelID = channel.Id
			break
		}
	}

	if channelID == "" {
		return fmt.Errorf("off-topic channel not found in team %s", c.TeamName)
	}

	c.logf("Found off-topic channel ID: %s in team %s\n", channelID, c.TeamName)

	c.log("Setting up Weather app...")
	if err := c.CreateWeatherApp(channelID, teamID); err != nil {
		c.logf("❌ Failed to setup weather app: %v\n", err)
	}

	c.log("Setting up Flight app...")
	if err := c.CreateFlightApp(channelID, teamID); err != nil {
		c.logf("❌ Failed to setup flight app: %v\n", err)
	}

	return nil
}

// SetupTestData sets up test data in Mattermost
func (c *Client) SetupTestData() error {
	c.log("===========================================")
	c.log("Setting up test Data for Mattermost")
	c.log("===========================================")

	if err := c.CreateUsers(); err != nil {
		return err
	}

	if err := c.CreateTeam(); err != nil {
		return err
	}

	if err := c.SetupWebhooks(); err != nil {
		return err
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

// Setup performs the main setup
func (c *Client) Setup() error {
	// Safety check - make sure the client and API are properly initialized
	if c == nil || c.API == nil {
		return fmt.Errorf("client not properly initialized")
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
