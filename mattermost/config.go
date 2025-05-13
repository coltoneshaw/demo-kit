package mattermost

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Default configuration file paths
const (
	DefaultConfigPath = "../config.json"
)

// UserConfig represents the configuration for a Mattermost user
type UserConfig struct {
	// Username is the login name for the user
	Username string `json:"username"`

	// Email is the user's email address
	Email string `json:"email"`

	// Password is the user's password
	Password string `json:"password"`

	// Nickname is an optional display name
	Nickname string `json:"nickname,omitempty"`

	// IsSystemAdmin indicates if the user should have system admin privileges
	IsSystemAdmin bool `json:"isSystemAdmin"`

	// Teams is a list of team names the user should belong to
	Teams []string `json:"teams"`
}

// Config represents the main configuration structure
type Config struct {
	// Users is an array of user configurations
	Users []UserConfig `json:"users"`

	// Teams is an optional map of team configurations
	Teams map[string]TeamConfig `json:"teams,omitempty"`
}

// WebhookConfig represents the configuration for a Mattermost webhook
type WebhookConfig struct {
	// DisplayName is the webhook display name
	DisplayName string `json:"displayName"`

	// Description is the webhook description
	Description string `json:"description"`

	// ChannelName is the channel where the webhook should be added
	ChannelName string `json:"channelName"`

	// Username is the webhook username
	Username string `json:"username"`

	// IconURL is an optional URL for the webhook icon
	IconURL string `json:"iconUrl,omitempty"`

	// EnvVariable is the environment variable to set with the webhook URL
	EnvVariable string `json:"envVariable"`

	// ContainerName is the container to restart after webhook creation
	ContainerName string `json:"containerName"`
}

// SlashCommandConfig represents the configuration for a Mattermost slash command
type SlashCommandConfig struct {
	// Trigger is the command trigger word (e.g., "weather" for /weather)
	Trigger string `json:"trigger"`

	// URL is the endpoint that the command will call
	URL string `json:"url"`

	// DisplayName is the command display name
	DisplayName string `json:"displayName"`

	// Description is the command description
	Description string `json:"description"`

	// Username is the command bot username
	Username string `json:"username"`

	// AutoComplete enables command autocomplete
	AutoComplete bool `json:"autoComplete"`

	// AutoCompleteDesc is the description shown in autocomplete
	AutoCompleteDesc string `json:"autoCompleteDesc,omitempty"`

	// AutoCompleteHint is the hint shown in autocomplete
	AutoCompleteHint string `json:"autoCompleteHint,omitempty"`
}

// TeamConfig represents the configuration for a Mattermost team
type TeamConfig struct {
	// Name is the team name
	Name string `json:"name"`

	// DisplayName is the human-readable team name
	DisplayName string `json:"displayName"`

	// Description is an optional team description
	Description string `json:"description,omitempty"`

	// Type can be "O" (open), "I" (invite only), or other valid Mattermost team types
	Type string `json:"type,omitempty"`

	// Webhooks defines the webhooks to create for this team
	Webhooks []WebhookConfig `json:"webhooks,omitempty"`

	// SlashCommands defines the slash commands to create for this team
	SlashCommands []SlashCommandConfig `json:"slashCommands,omitempty"`
}

// LoadConfig loads the configuration from the specified file path
// If path is empty, it uses the default path
func LoadConfig(path string) (*Config, error) {
	if path == "" {
		path = DefaultConfigPath
	}

	// Make sure the file exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, fmt.Errorf("config file not found at %s: %w", path, err)
	}

	// Read the file
	file, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Parse the JSON
	var config Config
	if err := json.Unmarshal(file, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config JSON: %w", err)
	}

	// Validate the config
	if err := validateConfig(&config); err != nil {
		return nil, err
	}

	return &config, nil
}

// validateConfig performs basic validation on the configuration
func validateConfig(config *Config) error {
	// Check if we have any users configured
	if len(config.Users) == 0 {
		return fmt.Errorf("no users defined in config")
	}

	// Validate each user has required fields
	for i, user := range config.Users {
		if user.Username == "" {
			return fmt.Errorf("user at index %d is missing username", i)
		}
		if user.Email == "" {
			return fmt.Errorf("user '%s' is missing email", user.Username)
		}
		if user.Password == "" {
			return fmt.Errorf("user '%s' is missing password", user.Username)
		}
	}

	// Validate each team in the teams map has required fields
	for name, team := range config.Teams {
		if team.Name == "" {
			return fmt.Errorf("team '%s' is missing name field", name)
		}
		if team.DisplayName == "" {
			return fmt.Errorf("team '%s' is missing displayName", name)
		}

		// Validate webhooks
		for i, webhook := range team.Webhooks {
			if webhook.DisplayName == "" {
				return fmt.Errorf("webhook at index %d for team '%s' is missing displayName", i, name)
			}
			if webhook.ChannelName == "" {
				return fmt.Errorf("webhook '%s' for team '%s' is missing channelName", webhook.DisplayName, name)
			}
			if webhook.EnvVariable == "" {
				return fmt.Errorf("webhook '%s' for team '%s' is missing envVariable", webhook.DisplayName, name)
			}
			if webhook.ContainerName == "" {
				return fmt.Errorf("webhook '%s' for team '%s' is missing containerName", webhook.DisplayName, name)
			}
		}

		// Validate slash commands
		for i, cmd := range team.SlashCommands {
			if cmd.Trigger == "" {
				return fmt.Errorf("slash command at index %d for team '%s' is missing trigger", i, name)
			}
			if cmd.URL == "" {
				return fmt.Errorf("slash command '%s' for team '%s' is missing URL", cmd.Trigger, name)
			}
			if cmd.DisplayName == "" {
				return fmt.Errorf("slash command '%s' for team '%s' is missing displayName", cmd.Trigger, name)
			}
			if cmd.Description == "" {
				return fmt.Errorf("slash command '%s' for team '%s' is missing description", cmd.Trigger, name)
			}
		}
	}

	return nil
}

// SaveConfig saves the configuration to the specified file path
// If path is empty, it uses the default path
func SaveConfig(config *Config, path string) error {
	if path == "" {
		path = DefaultConfigPath
	}

	// Create directory if it doesn't exist
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Validate the config before saving
	if err := validateConfig(config); err != nil {
		return err
	}

	// Marshal the JSON with indentation for readability
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config to JSON: %w", err)
	}

	// Write the file
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}
