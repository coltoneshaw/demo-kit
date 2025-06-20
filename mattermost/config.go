package mattermost

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sirupsen/logrus"
	
	ldapPkg "github.com/coltoneshaw/demokit/mattermost/ldap"
)

// Default configuration file paths
const (
	DefaultConfigPath = "config.json"
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

	// FirstName is the user's first name
	FirstName string `json:"firstname,omitempty"`

	// LastName is the user's last name
	LastName string `json:"lastname,omitempty"`

	// Position is the user's job title/position
	Position string `json:"position,omitempty"`

	// IsSystemAdmin indicates if the user should have system admin privileges
	IsSystemAdmin bool `json:"isSystemAdmin"`

	// Teams is a list of team names the user should belong to
	Teams []string `json:"teams"`
}

// LDAPConfigFile represents LDAP configuration from config.json
type LDAPConfigFile = ldapPkg.LDAPConfig

// Config represents the main configuration structure
type Config struct {
	// Environment specifies the deployment environment (e.g., "local", "staging", "production")
	Environment string `json:"environment,omitempty"`

	// Server is the Mattermost server URL
	Server string `json:"server"`

	// AdminUsername is the admin account username
	AdminUsername string `json:"admin_username"`

	// AdminPassword is the admin account password
	AdminPassword string `json:"admin_password"`

	// DefaultTeam is the default team name for operations
	DefaultTeam string `json:"default_team,omitempty"`

	// Users is an array of user configurations
	Users []UserConfig `json:"users,omitempty"`

	// Teams is an optional map of team configurations
	Teams map[string]TeamConfig `json:"teams,omitempty"`

	// Plugins is an optional array of plugin configurations to download from GitHub
	Plugins []PluginConfig `json:"plugins,omitempty"`

	// LDAP contains LDAP server configuration
	LDAP LDAPConfigFile `json:"ldap,omitempty"`
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

	// PluginID is the prefix of the tar file (e.g., "mattermost-plugin-playbooks")
	PluginID string `json:"plugin-id"`
}

// LoadConfig loads the configuration from the specified file path
// If path is empty, it uses the default path and checks multiple locations
func LoadConfig(path string) (*Config, error) {
	if path == "" {
		// Try multiple possible locations for the config file
		possiblePaths := []string{
			"config.json",          // Current directory (when run from root)
			"../config.json",       // Parent directory (when run from mattermost/)
		}
		
		for _, possiblePath := range possiblePaths {
			if _, err := os.Stat(possiblePath); err == nil {
				path = possiblePath
				break
			}
		}
		
		if path == "" {
			return nil, fmt.Errorf("config file not found. Tried: %v", possiblePaths)
		}
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
	// Check required admin fields
	if config.Server == "" {
		return fmt.Errorf("server URL is required in config")
	}
	if config.AdminUsername == "" {
		return fmt.Errorf("admin_username is required in config")
	}
	if config.AdminPassword == "" {
		return fmt.Errorf("admin_password is required in config")
	}

	// Validate each user has required fields (if users are defined)
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
					Log.WithFields(logrus.Fields{
						"member": member,
						"channel": channel.Name,
						"team": name,
					}).Warn("⚠️ Member in channel is not defined in users section")
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
					Log.WithFields(logrus.Fields{
						"command_type": cmdName,
						"channel": channel.Name,
						"team": name,
						"supported_types": supportedTypes,
					}).Warn("⚠️ Command type may not be supported")
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
