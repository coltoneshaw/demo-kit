package mattermost

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/sirupsen/logrus"
)

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

// EchoLogins prints login information - always shown regardless of test mode
func (c *Client) EchoLogins() {
	Log.Info("===========================================")
	Log.Info("🔑 Mattermost logins")
	Log.Info("===========================================")

	Log.Info("- System admin")
	Log.WithFields(logrus.Fields{"username": c.AdminUser}).Info("     - username")
	Log.WithFields(logrus.Fields{"password": c.AdminPass}).Info("     - password")

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