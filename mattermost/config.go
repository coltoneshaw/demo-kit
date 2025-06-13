package mattermost

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Default configuration file paths
const (
	DefaultConfigPath = "./config.json"
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

	// Plugins is an optional array of plugin configurations to download from GitHub
	Plugins []PluginConfig `json:"plugins,omitempty"`
}

// ChannelConfig represents the configuration for a Mattermost channel
type ChannelConfig struct {
	// Name is the channel name (no spaces, lowercase)
	Name string `json:"name"`

	// DisplayName is the human-readable channel name
	DisplayName string `json:"displayName"`

	// Purpose is an optional channel purpose description
	Purpose string `json:"purpose,omitempty"`

	// Header is an optional channel header text
	Header string `json:"header,omitempty"`

	// Type is the channel type: "O" for public, "P" for private
	Type string `json:"type,omitempty"`

	// Members is a list of usernames to add to this channel
	Members []string `json:"members,omitempty"`

	// Commands is a list of slash commands to execute in this channel
	Commands []string `json:"commands,omitempty"`

	// Category is an optional category to add the channel to
	Category string `json:"category,omitempty"`
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

	// Channels defines the channels to create for this team
	Channels []ChannelConfig `json:"channels,omitempty"`
}

// PluginConfig represents the configuration for a plugin to download from GitHub
type PluginConfig struct {
	// Name is the human-readable plugin name
	Name string `json:"name"`

	// Repo is the GitHub repository in format "owner/repo"
	Repo string `json:"github_repo"`
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

		// Validate channels
		for i, channel := range team.Channels {
			if channel.Name == "" {
				return fmt.Errorf("channel at index %d for team '%s' is missing name", i, name)
			}
			if channel.DisplayName == "" {
				return fmt.Errorf("channel '%s' for team '%s' is missing displayName", channel.Name, name)
			}

			// Validate channel type if provided
			if channel.Type != "" && channel.Type != "O" && channel.Type != "P" {
				return fmt.Errorf("channel '%s' for team '%s' has invalid type '%s', must be 'O' for public or 'P' for private",
					channel.Name, name, channel.Type)
			}

			// Validate members exist in users
			for _, member := range channel.Members {
				userFound := false
				for _, user := range config.Users {
					if user.Username == member {
						userFound = true
						break
					}
				}

				if !userFound {
					// This is just a warning, not an error
					fmt.Printf("Warning: Member '%s' in channel '%s' is not defined in users section\n",
						member, channel.Name)
				}
			}

			// Validate commands
			for i, command := range channel.Commands {
				if command == "" {
					return fmt.Errorf("command at index %d for channel '%s' in team '%s' is empty",
						i, channel.Name, name)
				}

				// Verify the command starts with a slash
				if !strings.HasPrefix(command, "/") {
					return fmt.Errorf("command '%s' for channel '%s' in team '%s' must start with /",
						command, channel.Name, name)
				}

				// Extract the command name for validation
				parts := strings.Fields(command)
				if len(parts) == 0 {
					return fmt.Errorf("command '%s' for channel '%s' in team '%s' is invalid",
						command, channel.Name, name)
				}

				cmdName := strings.TrimPrefix(parts[0], "/")

				// Validate command is one of the supported types
				validType := false
				supportedTypes := []string{"weather", "flights", "mission"}
				for _, t := range supportedTypes {
					if cmdName == t {
						validType = true
						break
					}
				}

				if !validType {
					// Just a warning, not an error
					fmt.Printf("Warning: Command type '%s' for channel '%s' may not be supported\n",
						cmdName, channel.Name)
				}
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
