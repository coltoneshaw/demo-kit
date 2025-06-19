package mattermost

import (
	"encoding/json"
)

// BulkImportLine represents a single line in the bulk import JSONL file
type BulkImportLine struct {
	Type    string          `json:"type"`
	Version int             `json:"version,omitempty"`
	Raw     json.RawMessage `json:"-"` // Store original JSON for writing
}

// ChannelCategoryImport represents a channel category import entry
type ChannelCategoryImport struct {
	Type     string   `json:"type"`
	Category string   `json:"category"`
	Team     string   `json:"team"`
	Channels []string `json:"channels"`
}

// CommandImport represents a command import entry
type CommandImport struct {
	Type    string `json:"type"`
	Command struct {
		Team    string `json:"team"`
		Channel string `json:"channel"`
		Text    string `json:"text"`
	} `json:"command"`
}

// ChannelBannerImport represents a channel banner import entry
type ChannelBannerImport struct {
	Type   string `json:"type"`
	Banner struct {
		Team            string `json:"team"`
		Channel         string `json:"channel"`
		Text            string `json:"text"`
		BackgroundColor string `json:"background_color"`
		Enabled         bool   `json:"enabled"`
	} `json:"banner"`
}

// PluginImport represents a plugin import entry
type PluginImport struct {
	Type   string `json:"type"`
	Plugin struct {
		Source       string `json:"source"`        // "github" or "local"
		GithubRepo   string `json:"github_repo"`   // For GitHub plugins: "owner/repo"
		Path         string `json:"path"`          // For local plugins: "../apps/plugin-name"
		PluginID     string `json:"plugin_id"`     // Plugin ID
		Name         string `json:"name"`          // Human readable name
		ForceInstall bool   `json:"force_install"` // Whether to force reinstall
	} `json:"plugin"`
}

// UserRank represents military rank information
type UserRank struct {
	Username string
	Rank     string
	Level    int // For comparison (higher = senior)
	Unit     string
}

// ChannelContext represents conversation context for a channel
type ChannelContext struct {
	Name        string
	Topics      []string
	Formality   string // "formal", "operational", "informal"
	MessageType string // "briefing", "status", "coordination", "intel"
}

// UserAttributeImport represents a single user attribute definition import entry
type UserAttributeImport struct {
	Type      string             `json:"type"`
	Attribute UserAttributeField `json:"attribute"`
}

// UserAttributeField represents a custom profile field definition
type UserAttributeField struct {
	Name          string `json:"name"`
	DisplayName   string `json:"display_name"`
	Type          string `json:"type"`
	HideWhenEmpty bool   `json:"hide_when_empty,omitempty"`
	Required      bool   `json:"required,omitempty"`
	// Extended configuration fields
	LDAPAttribute string   `json:"ldap,omitempty"`       // LDAP attribute mapping
	SAMLAttribute string   `json:"saml,omitempty"`       // SAML attribute mapping
	Options       []string `json:"options,omitempty"`    // Options for select fields
	SortOrder     int      `json:"sort_order,omitempty"` // Display order
	ValueType     string   `json:"value_type,omitempty"` // Value type constraint
	Visibility    string   `json:"visibility,omitempty"` // Visibility setting
}

// UserProfileImport represents a user profile assignment entry
type UserProfileImport struct {
	Type       string            `json:"type"`
	User       string            `json:"user"`       // Username
	Attributes map[string]string `json:"attributes"` // Map of attribute name to value
}

// GroupConfig represents group configuration from import data
type GroupConfig struct {
	Name           string   `json:"name"`            // Group name
	ID             string   `json:"id"`              // Unique group identifier
	Members        []string `json:"members"`         // Array of usernames
	AllowReference bool     `json:"allow_reference"` // Whether the group can be referenced (@mentions)
}

// UserGroupImport represents a user group import entry
type UserGroupImport struct {
	Type  string      `json:"type"`
	Group GroupConfig `json:"group"`
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