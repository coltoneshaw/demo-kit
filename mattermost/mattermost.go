package mattermost

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/mattermost/mattermost/server/public/model"
)

// Constants
const (
	MaxWaitSeconds = 120
	EnvFile        = "./files/env_vars.env"
)

// Client represents a Mattermost API client
type Client struct {
	API           *model.Client4
	ServerURL     string
	AdminUser     string
	AdminPass     string
	TeamName      string
	FlightAppURL  string
	WeatherAppURL string
}

// NewClient creates a new Mattermost client
func NewClient(serverURL, adminUser, adminPass, teamName string) *Client {
	return &Client{
		API:           model.NewAPIv4Client(serverURL),
		ServerURL:     serverURL,
		AdminUser:     adminUser,
		AdminPass:     adminPass,
		TeamName:      teamName,
		FlightAppURL:  "http://flightaware-app:8086/webhook",
		WeatherAppURL: "http://weather-app:8085/webhook",
	}
}

// Login authenticates with the Mattermost server
func (c *Client) Login() error {
	_, resp, err := c.API.Login(context.Background(), c.AdminUser, c.AdminPass)
	if err != nil {
		if resp != nil {
			return fmt.Errorf("login failed: %v, response status code: %v", err, resp.StatusCode)
		}
		return fmt.Errorf("login failed: %v", err)
	}
	return nil
}

// WaitForStart waits for the Mattermost server to start
func (c *Client) WaitForStart() error {
	fmt.Printf("Waiting %d seconds for the server to start\n", MaxWaitSeconds)

	for i := 0; i < MaxWaitSeconds; i++ {
		_, resp, _ := c.API.GetPing(context.Background())
		if resp.StatusCode == 200 {
			fmt.Println("Server started")
			return nil
		}
		fmt.Print(".")
		time.Sleep(1 * time.Second)
	}

	return fmt.Errorf("server didn't start in %d seconds", MaxWaitSeconds)
}

// CreateUsers creates test users if they don't exist
func (c *Client) CreateUsers() error {
	// Check if sysadmin user exists
	users, resp, err := c.API.GetUsers(context.Background(), 0, 100, "")
	if err != nil {
		if resp != nil {
			return fmt.Errorf("failed to get users: %v, response status code: %v", err, resp.StatusCode)
		}
		return fmt.Errorf("failed to get users: %v", err)
	}

	sysadminExists := false
	user1Exists := false

	for _, user := range users {
		if user.Username == "sysadmin" {
			sysadminExists = true
		}
		if user.Username == "user-1" {
			user1Exists = true
		}
	}

	// Create sysadmin if needed
	if !sysadminExists {
		fmt.Println("Creating sysadmin user...")
		sysadmin := &model.User{
			Username: "sysadmin",
			Email:    "sysadmin@example.com",
			Password: "Testpassword123!",
			Roles:    "system_admin system_user",
		}

		createdUser, resp, err := c.API.CreateUser(context.Background(), sysadmin)
		if err != nil {
			return fmt.Errorf("failed to create sysadmin: %v, response status code: %v", err, resp.StatusCode)
		}
		fmt.Printf("✅ Successfully created system admin user '%s' (ID: %s)\n", createdUser.Username, createdUser.Id)

	} else {
		fmt.Println("User 'sysadmin' already exists")
	}

	// Create user-1 if needed
	if !user1Exists {
		fmt.Println("Creating user-1 user...")
		user1 := &model.User{
			Username: "user-1",
			Email:    "user-1@example.com",
			Password: "Testpassword123!",
		}

		createdUser1, resp, err := c.API.CreateUser(context.Background(), user1)
		if err != nil {
			return fmt.Errorf("failed to create user-1: %v, response status code: %v", err, resp.StatusCode)
		}
		fmt.Printf("✅ Successfully created regular user '%s' (ID: %s)\n", createdUser1.Username, createdUser1.Id)
	} else {
		fmt.Println("User 'user-1' already exists")
	}

	return nil
}

// CreateTeam creates a test team if it doesn't exist
func (c *Client) CreateTeam() error {
	// Check if team exists
	teams, resp, err := c.API.GetAllTeams(context.Background(), "", 0, 100)
	if err != nil {
		if resp != nil {
			return fmt.Errorf("failed to get teams: %v, response status code: %v", err, resp.StatusCode)
		}
		return fmt.Errorf("failed to get teams: %v", err)
	}

	teamExists := false
	var team *model.Team

	for _, t := range teams {
		if t.Name == c.TeamName {
			teamExists = true
			team = t
			break
		}
	}

	// Create team if needed
	if !teamExists {
		fmt.Printf("Creating %s team...\n", c.TeamName)
		newTeam := &model.Team{
			Name:        c.TeamName,
			DisplayName: "Test Team",
			Type:        model.TeamOpen,
		}

		var createResp *model.Response
		team, createResp, err = c.API.CreateTeam(context.Background(), newTeam)
		if err != nil {
			return fmt.Errorf("failed to create team: %v, response status code: %v", err, createResp.StatusCode)
		}
		fmt.Printf("✅ Successfully created team '%s' (ID: %s)\n", team.Name, team.Id)
	} else {
		fmt.Printf("Team '%s' already exists\n", c.TeamName)
	}

	// Add users to the team
	users, resp, err := c.API.GetUsers(context.Background(), 0, 100, "")
	if err != nil {
		if resp != nil {
			return fmt.Errorf("failed to get users: %v, response status code: %v", err, resp.StatusCode)
		}
		return fmt.Errorf("failed to get users: %v", err)
	}

	for _, user := range users {
		if user.Username == "sysadmin" || user.Username == "user-1" {
			_, resp, err := c.API.AddTeamMember(context.Background(), team.Id, user.Id)
			if err != nil {
				// Check if the error is because the user is already a member
				if resp.StatusCode == 400 {
					continue
				}
				return fmt.Errorf("failed to add user to team: %v", err)
			}
			fmt.Printf("✅ Added user '%s' to team '%s'\n", user.Username, c.TeamName)
		}
	}

	return nil
}

// CreateSlashCommand creates a single slash command
func (c *Client) CreateSlashCommand(teamID, trigger, url, displayName, description, username string) error {
	// Get existing commands
	commands, resp, err := c.API.ListCommands(context.Background(), teamID, true)
	if err != nil {
		if resp != nil {
			return fmt.Errorf("failed to list commands: %v, response status code: %v", err, resp.StatusCode)
		}
		return fmt.Errorf("failed to list commands: %v", err)
	}

	// Check if command already exists
	for _, cmd := range commands {
		if cmd.Trigger == trigger {
			fmt.Printf("/%s command already exists\n", trigger)
			return nil
		}
	}

	// Create command
	fmt.Printf("Creating /%s slash command...\n", trigger)
	cmd := &model.Command{
		TeamId:       teamID,
		Trigger:      trigger,
		Method:       "P",
		URL:          url,
		CreatorId:    "", // Will be set to current user
		DisplayName:  displayName,
		Description:  description,
		AutoComplete: true,
		Username:     username,
	}

	createdCmd, resp, err := c.API.CreateCommand(context.Background(), cmd)
	if err != nil {
		if resp != nil {
			return fmt.Errorf("failed to create %s command: %v, response status code: %v", trigger, err, resp.StatusCode)
		}
		return fmt.Errorf("failed to create %s command: %v", trigger, err)
	}
	
	fmt.Printf("✅ /%s command created successfully (ID: %s)\n", createdCmd.Trigger, createdCmd.Id)
	return nil
}

// CreateFlightCommand creates the flights slash command
func (c *Client) CreateFlightCommand(teamID string) error {
	return c.CreateSlashCommand(
		teamID,
		"flights",
		c.FlightAppURL,
		"Flight Departures",
		"Get flight departures",
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
		"Get weather information",
		"weather-bot",
	)
}


// UpdateWebhookConfig updates the webhook configuration in the env file
func (c *Client) UpdateWebhookConfig(webhookID, appName, envVarName, containerName string) error {
	fmt.Printf("✅ Created webhook with ID: %s for %s\n", webhookID, appName)

	// Update env_vars.env file with the webhook URL
	webhookURL := fmt.Sprintf("http://mattermost:8065/hooks/%s", webhookID)
	fmt.Printf("Setting webhook URL: %s for %s\n", webhookURL, envVarName)

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

	fmt.Printf("Updated env_vars.env with webhook URL for %s\n", appName)

	// Restart the app container
	fmt.Printf("Restarting %s container...\n", containerName)
	cmd := exec.Command("docker", "restart", containerName)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to restart container: %v", err)
	}
	fmt.Printf("✅ %s restarted successfully\n", appName)

	return nil
}

// CreateWebhook creates a single incoming webhook
func (c *Client) CreateWebhook(channelID, displayName, description, username string) (*model.IncomingWebhook, error) {
	// Check if webhook already exists
	hooks, resp, err := c.API.GetIncomingWebhooks(context.Background(), 0, 1000, "")
	if err != nil {
		if resp != nil {
			return nil, fmt.Errorf("failed to get webhooks: %v, response status code: %v", err, resp.StatusCode)
		}
		return nil, fmt.Errorf("failed to get webhooks: %v", err)
	}

	for _, hook := range hooks {
		if hook.DisplayName == displayName {
			fmt.Printf("Webhook '%s' already exists\n", displayName)
			return hook, nil
		}
	}

	// Create the webhook
	fmt.Printf("Creating incoming webhook '%s'...\n", displayName)
	hook := &model.IncomingWebhook{
		ChannelId:   channelID,
		DisplayName: displayName,
		Description: description,
		Username:    username,
	}

	newHook, resp, err := c.API.CreateIncomingWebhook(context.Background(), hook)
	if err != nil {
		if resp != nil {
			return nil, fmt.Errorf("failed to create webhook: %v, response status code: %v", err, resp.StatusCode)
		}
		return nil, fmt.Errorf("failed to create webhook: %v", err)
	}

	return newHook, nil
}

// CreateAppWebhook creates an incoming webhook for an app and updates its configuration
func (c *Client) CreateAppWebhook(channelID, appName, displayName, description, username, envVarName, containerName string) error {
	// Create the webhook
	newHook, err := c.CreateWebhook(channelID, displayName, description, username)
	if err != nil {
		return err
	}

	// If webhook already existed but we didn't get its ID, we can't update the config
	if newHook == nil || newHook.Id == "" {
		return fmt.Errorf("webhook created but no ID returned")
	}

	// Update the webhook configuration
	return c.UpdateWebhookConfig(newHook.Id, appName, envVarName, containerName)
}

// CreateWeatherApp sets up the weather app (webhook and slash command)
func (c *Client) CreateWeatherApp(channelID, teamID string) error {
	// Create webhook
	err := c.CreateAppWebhook(
		channelID,
		"Weather app",
		"weather",
		"Weather responses",
		"professor",
		"WEATHER_MATTERMOST_WEBHOOK_URL",
		"weather-app",
	)
	if err != nil {
		return fmt.Errorf("failed to create weather webhook: %v", err)
	}

	// Create slash command
	if err := c.CreateWeatherCommand(teamID); err != nil {
		return fmt.Errorf("failed to create weather command: %v", err)
	}

	return nil
}

// CreateFlightApp sets up the flight app (webhook and slash command)
func (c *Client) CreateFlightApp(channelID, teamID string) error {
	// Create webhook
	err := c.CreateAppWebhook(
		channelID,
		"Flight app",
		"flight-app",
		"Flight departures",
		"professor",
		"FLIGHTS_MATTERMOST_WEBHOOK_URL",
		"flightaware-app",
	)
	if err != nil {
		return fmt.Errorf("failed to create flight webhook: %v", err)
	}

	// Create slash command
	if err := c.CreateFlightCommand(teamID); err != nil {
		return fmt.Errorf("failed to create flight command: %v", err)
	}

	return nil
}

// SetupWebhooks sets up webhooks for the apps
func (c *Client) SetupWebhooks() error {
	// Get the channel ID for off-topic in the test team
	teams, resp, err := c.API.GetAllTeams(context.Background(), "", 0, 100)
	if err != nil {
		if resp != nil {
			return fmt.Errorf("failed to get teams: %v, response status code: %v", err, resp.StatusCode)
		}
		return fmt.Errorf("failed to get teams: %v", err)
	}

	var teamID string
	for _, team := range teams {
		if team.Name == c.TeamName {
			teamID = team.Id
			break
		}
	}

	if teamID == "" {
		return fmt.Errorf("team '%s' not found", c.TeamName)
	}

	channels, resp, err := c.API.GetPublicChannelsForTeam(context.Background(), teamID, 0, 100, "")
	if err != nil {
		if resp != nil {
			return fmt.Errorf("failed to get channels: %v, response status code: %v", err, resp.StatusCode)
		}
		return fmt.Errorf("failed to get channels: %v", err)
	}

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

	fmt.Printf("Found off-topic channel ID: %s\n", channelID)

	fmt.Println("Setting up Weather app...")
	if err := c.CreateWeatherApp(channelID, teamID); err != nil {
		fmt.Printf("Warning: Failed to setup weather app: %v\n", err)
	}

	fmt.Println("Setting up Flight app...")
	if err := c.CreateFlightApp(channelID, teamID); err != nil {
		fmt.Printf("Warning: Failed to setup flight app: %v\n", err)
	}

	return nil
}

// SetupTestData sets up test data in Mattermost
func (c *Client) SetupTestData() error {
	fmt.Println("===========================================")
	fmt.Println("Setting up test Data for Mattermost")
	fmt.Println("===========================================")

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

// Setup performs the main setup
func (c *Client) Setup() error {
	if err := c.WaitForStart(); err != nil {
		return err
	}

	if err := c.Login(); err != nil {
		return err
	}

	if err := c.SetupTestData(); err != nil {
		return err
	}

	fmt.Println("Alright, everything seems to be setup and running. Enjoy.")
	return nil
}

// EchoLogins prints login information
func (c *Client) EchoLogins() {
	fmt.Println("===========================================")
	fmt.Println("Mattermost logins")
	fmt.Println("===========================================")

	fmt.Println("- System admin")
	fmt.Println("     - username: sysadmin")
	fmt.Println("     - password: Testpassword123!")
	fmt.Println("- Regular account:")
	fmt.Println("     - username: user-1")
	fmt.Println("     - password: Testpassword123!")
	fmt.Println("- LDAP or SAML account:")
	fmt.Println("     - username: professor")
	fmt.Println("     - password: professor")
	fmt.Println()
	fmt.Println("For more logins check out https://github.com/coltoneshaw/mattermost#accounts")
	fmt.Println()
	fmt.Println("===========================================")
}
