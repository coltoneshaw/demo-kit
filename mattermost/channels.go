package mattermost

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/mattermost/mattermost/server/public/model"
)

// ChannelManager handles all channel-related operations
type ChannelManager struct {
	client *Client
}

// NewChannelManager creates a new ChannelManager instance
func NewChannelManager(client *Client) *ChannelManager {
	return &ChannelManager{client: client}
}

// GetChannelsForTeam retrieves all channels for a team
func (cm *ChannelManager) GetChannelsForTeam(teamID string, includePrivate bool) ([]*model.Channel, error) {
	// Get public channels
	channels, resp, err := cm.client.API.GetPublicChannelsForTeam(context.Background(), teamID, 0, 1000, "")
	if err != nil {
		return nil, handleAPIError("failed to get public channels", err, resp)
	}

	// Also check private channels if requested
	if includePrivate {
		privateChannels, _, privateErr := cm.client.API.GetPrivateChannelsForTeam(context.Background(), teamID, 0, 1000, "")
		if privateErr == nil {
			channels = append(channels, privateChannels...)
		} else {
			fmt.Printf("❌ Warning: Failed to get private channels: %v\n", privateErr)
		}
	}

	return channels, nil
}

// GetChannelByName retrieves a channel by name in a team
func (cm *ChannelManager) GetChannelByName(teamID, channelName string) (*model.Channel, error) {
	channels, err := cm.GetChannelsForTeam(teamID, true) // Include private channels
	if err != nil {
		return nil, err
	}

	for _, channel := range channels {
		if channel.Name == channelName {
			return channel, nil
		}
	}

	return nil, fmt.Errorf("channel '%s' not found in team", channelName)
}

// ChannelExists checks if a channel exists by name in a team
func (cm *ChannelManager) ChannelExists(teamID, channelName string) (bool, *model.Channel, error) {
	channel, err := cm.GetChannelByName(teamID, channelName)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return false, nil, nil
		}
		return false, nil, err
	}
	return true, channel, nil
}

// CreateChannel creates a new channel
func (cm *ChannelManager) CreateChannel(teamID, name, displayName, purpose, header, channelType string) (*model.Channel, error) {
	// Default to Open if not specified
	if channelType == "" {
		channelType = "O"
	}

	fmt.Printf("Creating '%s' channel...\n", name)

	newChannel := &model.Channel{
		TeamId:      teamID,
		Name:        name,
		DisplayName: displayName,
		Purpose:     purpose,
		Header:      header,
		Type:        model.ChannelType(channelType),
	}

	createdChannel, createResp, err := cm.client.API.CreateChannel(context.Background(), newChannel)
	if err != nil {
		return nil, handleAPIError("failed to create channel", err, createResp)
	}

	fmt.Printf("✅ Successfully created channel '%s' (ID: %s)\n", createdChannel.Name, createdChannel.Id)
	return createdChannel, nil
}

// CreateOrGetChannel creates a new channel or returns an existing one
func (cm *ChannelManager) CreateOrGetChannel(teamID, name, displayName, purpose, header, channelType string) (*model.Channel, error) {
	// Check if channel already exists
	exists, channel, err := cm.ChannelExists(teamID, name)
	if err != nil {
		return nil, fmt.Errorf("failed to check if channel exists: %w", err)
	}

	if exists {
		fmt.Printf("Channel '%s' already exists in team\n", name)
		return channel, nil
	}

	// Create the channel
	return cm.CreateChannel(teamID, name, displayName, purpose, header, channelType)
}

// IsUserChannelMember checks if a user is a member of a channel
func (cm *ChannelManager) IsUserChannelMember(channelID, userID string) (bool, error) {
	_, resp, err := cm.client.API.GetChannelMember(context.Background(), channelID, userID, "")
	if err != nil {
		if resp != nil && resp.StatusCode == 404 {
			return false, nil
		}
		return false, handleAPIError("failed to check channel membership", err, resp)
	}
	return true, nil
}

// AddUserToChannel adds a user to a channel
func (cm *ChannelManager) AddUserToChannel(userID, channelID, username, channelName string) error {
	// Check if user is already a channel member
	isMember, err := cm.IsUserChannelMember(channelID, userID)
	if err != nil {
		return fmt.Errorf("failed to check channel membership: %w", err)
	}

	if isMember {
		fmt.Printf("User '%s' is already a member of channel '%s'\n", username, channelName)
		return nil
	}

	_, resp, err := cm.client.API.AddChannelMember(context.Background(), channelID, userID)
	if err != nil {
		// Check if the error is because the user is already a member
		if resp != nil && resp.StatusCode == 400 {
			fmt.Printf("User '%s' is already a member of channel '%s'\n", username, channelName)
			return nil
		}
		return fmt.Errorf("failed to add user '%s' to channel '%s': %w", username, channelName, err)
	}

	fmt.Printf("✅ Added user '%s' to channel '%s'\n", username, channelName)
	return nil
}

// CategorizeChannel adds a channel to a category using the Playbooks API
func (cm *ChannelManager) CategorizeChannel(channelID string, categoryName string) error {
	if channelID == "" || categoryName == "" {
		return fmt.Errorf("channel ID and category name are required")
	}

	fmt.Printf("Categorizing channel %s in category '%s' using Playbooks API...\n", channelID, categoryName)

	// Construct the URL for the categorize channel API
	url := fmt.Sprintf("%s/plugins/playbooks/api/v0/actions/channels/%s",
		cm.client.ServerURL, channelID)

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
	req.Header.Set("Authorization", "Bearer "+cm.client.API.AuthToken)

	// Send the request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send categorize request: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			fmt.Printf("failed to close response body: %v", closeErr)
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

// SetupTeamChannels creates channels for a specific team
func (cm *ChannelManager) SetupTeamChannels(team *model.Team, teamConfig TeamConfig) error {
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
		channel, err := cm.CreateOrGetChannel(
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

		// If a category is specified, categorize the channel after creation
		if channelConfig.Category != "" {
			if err := cm.CategorizeChannel(channel.Id, channelConfig.Category); err != nil {
				fmt.Printf("⚠️ Warning: Failed to categorize channel '%s' in category '%s': %v\n",
					channelConfig.Name, channelConfig.Category, err)
				// Don't return error here, continue with other operations
			}
		}
	}

	return nil
}

// AddChannelMembers adds members to channels after teams and users are fully set up
func (cm *ChannelManager) AddChannelMembers(config *Config, userManager *UserManager) error {
	// Only proceed if we have a config
	if config == nil || len(config.Teams) == 0 {
		return nil
	}

	fmt.Println("Adding members to channels...")

	// Get all teams
	teamManager := NewTeamManager(cm.client)
	teams, err := teamManager.GetAllTeams()
	if err != nil {
		return fmt.Errorf("failed to get teams: %w", err)
	}

	// Create a map of team names to team objects
	teamMap := make(map[string]*model.Team)
	for _, team := range teams {
		teamMap[team.Name] = team
	}

	// For each team in the config
	for teamName, teamConfig := range config.Teams {
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
			channel, err := cm.GetChannelByName(team.Id, channelConfig.Name)
			if err != nil {
				fmt.Printf("❌ Channel '%s' not found in team '%s': %v\n", channelConfig.Name, teamName, err)
				continue
			}

			// Add members to the channel
			fmt.Printf("Adding %d members to channel '%s'\n", len(channelConfig.Members), channelConfig.Name)

			for _, username := range channelConfig.Members {
				user, err := userManager.GetUserByUsername(username)
				if err != nil {
					fmt.Printf("❌ User '%s' not found, can't add to channel '%s'\n", username, channelConfig.Name)
					continue
				}

				if err := cm.AddUserToChannel(user.Id, channel.Id, username, channelConfig.Name); err != nil {
					fmt.Printf("❌ Failed to add user '%s' to channel '%s': %v\n", username, channelConfig.Name, err)
				}
			}
		}
	}

	return nil
}

// ExecuteChannelCommands executes specified slash commands in channels sequentially
func (cm *ChannelManager) ExecuteChannelCommands(config *Config) error {
	// Only proceed if we have a config
	if config == nil || len(config.Teams) == 0 {
		return nil
	}

	fmt.Println("Setting up channel commands...")

	// Get all teams
	teamManager := NewTeamManager(cm.client)
	teams, err := teamManager.GetAllTeams()
	if err != nil {
		return fmt.Errorf("failed to get teams: %w", err)
	}

	// Create a map of team names to team objects
	teamMap := make(map[string]*model.Team)
	for _, team := range teams {
		teamMap[team.Name] = team
	}

	// For each team in the config
	for teamName, teamConfig := range config.Teams {
		team, exists := teamMap[teamName]
		if !exists {
			fmt.Printf("❌ Team '%s' not found, can't execute commands\n", teamName)
			return fmt.Errorf("team '%s' not found", teamName)
		}

		// For each channel with commands
		for _, channelConfig := range teamConfig.Channels {
			// Skip if no commands are configured
			if len(channelConfig.Commands) == 0 {
				continue
			}

			// Find the channel
			channel, err := cm.GetChannelByName(team.Id, channelConfig.Name)
			if err != nil {
				fmt.Printf("❌ Channel '%s' not found in team '%s': %v\n", channelConfig.Name, teamName, err)
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

				fmt.Printf("Executing command %d/%d in channel '%s': %s\n",
					i+1, len(channelConfig.Commands), channelConfig.Name, commandText)

				// Execute the command using the commands/execute API
				_, resp, err := cm.client.API.ExecuteCommand(context.Background(), channel.Id, commandText)

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