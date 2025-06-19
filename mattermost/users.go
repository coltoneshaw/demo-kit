package mattermost

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/sirupsen/logrus"

	ldapPkg "github.com/coltoneshaw/demokit/mattermost/ldap"
)

// LoadBulkImportData loads and parses the bulk_import.jsonl file
func (c *Client) LoadBulkImportData() (*BulkImportData, error) {
	// Try multiple possible locations for the bulk import file
	possiblePaths := []string{
		"bulk_import.jsonl",    // Current directory (when run from root)
		"../bulk_import.jsonl", // Parent directory (when run from mattermost/)
	}

	var bulkImportPath string
	for _, path := range possiblePaths {
		if _, err := os.Stat(path); err == nil {
			bulkImportPath = path
			break
		}
	}

	if bulkImportPath == "" {
		return nil, fmt.Errorf("bulk_import.jsonl not found. Tried: %v", possiblePaths)
	}

	file, err := os.Open(bulkImportPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open bulk import file: %w", err)
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			Log.WithFields(logrus.Fields{"error": closeErr.Error()}).Warn("⚠️ Failed to close file")
		}
	}()

	var teams []BulkTeam
	var users []BulkUser

	// Define custom types that should be skipped during bulk import parsing
	customTypes := map[string]bool{
		"channel-category": true,
		"command":          true,
	}

	// Read the JSONL file line by line
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}

		// First, extract just the type field to check if it's a custom type
		var typeCheck struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal([]byte(line), &typeCheck); err != nil {
			// If we can't even parse the type, skip with warning
			Log.WithFields(logrus.Fields{"line": line}).Warn("⚠️ Failed to parse type from line, skipping")
			continue
		}

		// Skip custom types silently (no warning)
		if customTypes[typeCheck.Type] {
			continue
		}

		// For standard types, try to unmarshal as ResetImportLine
		var importLine ResetImportLine
		if err := json.Unmarshal([]byte(line), &importLine); err != nil {
			// Only warn for non-custom types that fail to parse
			Log.WithFields(logrus.Fields{"line": line}).Warn("⚠️ Failed to parse standard import line, skipping")
			continue
		}

		switch importLine.Type {
		case "team":
			if importLine.Team != nil {
				teams = append(teams, BulkTeam{
					Name:        importLine.Team.Name,
					DisplayName: importLine.Team.DisplayName,
					Type:        importLine.Team.Type,
					Description: importLine.Team.Description,
				})
			}
		case "user":
			if importLine.User != nil {
				users = append(users, BulkUser{
					Username:  importLine.User.Username,
					Email:     importLine.User.Email,
					FirstName: importLine.User.FirstName,
					LastName:  importLine.User.LastName,
					Nickname:  importLine.User.Nickname,
					Position:  importLine.User.Position,
				})
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading bulk import file: %w", err)
	}

	Log.WithFields(logrus.Fields{"teams_count": len(teams), "users_count": len(users)}).Info("📋 Loaded data from bulk import")

	return &BulkImportData{
		Teams: teams,
		Users: users,
	}, nil
}

// DeleteBulkUsers permanently deletes users from bulk import data
func (c *Client) DeleteBulkUsers(users []BulkUser) error {
	if len(users) == 0 {
		Log.Info("📋 No users found in bulk import to delete")
		return nil
	}

	Log.WithFields(logrus.Fields{"user_count": len(users)}).Info("🗑️ Deleting users from bulk import")

	for _, userInfo := range users {
		// Find the user by username
		user, resp, err := c.API.GetUserByUsername(context.Background(), userInfo.Username, "")
		if err != nil {
			if resp != nil && resp.StatusCode == 404 {
				Log.WithFields(logrus.Fields{"user_name": userInfo.Username}).Warn("⚠️ User not found, skipping")
				continue
			}
			return handleAPIError(fmt.Sprintf("failed to find user '%s'", userInfo.Username), err, resp)
		}

		// Permanently delete the user
		_, err = c.API.PermanentDeleteUser(context.Background(), user.Id)
		if err != nil {
			return handleAPIError(fmt.Sprintf("failed to delete user '%s'", userInfo.Username), err, nil)
		}

		Log.WithFields(logrus.Fields{"user_name": userInfo.Username}).Info("✅ Permanently deleted user")
	}

	return nil
}

// DeleteBulkTeams permanently deletes teams from bulk import data
func (c *Client) DeleteBulkTeams(teams []BulkTeam) error {
	if len(teams) == 0 {
		Log.Info("📋 No teams found in bulk import to delete")
		return nil
	}

	Log.WithFields(logrus.Fields{"team_count": len(teams)}).Info("🗑️ Deleting teams from bulk import")

	for _, teamInfo := range teams {
		// Find the team by name
		team, resp, err := c.API.GetTeamByName(context.Background(), teamInfo.Name, "")
		if err != nil {
			if resp != nil && resp.StatusCode == 404 {
				Log.WithFields(logrus.Fields{"team_name": teamInfo.Name}).Warn("⚠️ Team not found, skipping")
				continue
			}
			return handleAPIError(fmt.Sprintf("failed to find team '%s'", teamInfo.Name), err, resp)
		}

		// Permanently delete the team
		_, err = c.API.PermanentDeleteTeam(context.Background(), team.Id)
		if err != nil {
			return handleAPIError(fmt.Sprintf("failed to delete team '%s'", teamInfo.Name), err, nil)
		}

		Log.WithFields(logrus.Fields{"team_name": teamInfo.Name}).Info("✅ Permanently deleted team")
	}

	return nil
}

// Reset permanently deletes all teams and users from the bulk import file
func (c *Client) Reset() error {
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

	// Load bulk import data
	bulkData, err := c.LoadBulkImportData()
	if err != nil {
		return fmt.Errorf("failed to load bulk import data: %w", err)
	}

	// Check that deletion APIs are enabled
	if err := c.CheckDeletionSettings(); err != nil {
		return err
	}

	Log.Warn("🚨 WARNING: This will permanently delete all teams and users that are configured in the bulk import file.")
	Log.Warn("⚠️ This operation is irreversible.")

	// Delete users first (they need to be removed from teams before teams can be deleted)
	if err := c.DeleteBulkUsers(bulkData.Users); err != nil {
		return fmt.Errorf("failed to delete users: %w", err)
	}

	// Then delete teams
	if err := c.DeleteBulkTeams(bulkData.Teams); err != nil {
		return fmt.Errorf("failed to delete teams: %w", err)
	}

	Log.Info("✅ Reset completed successfully")
	return nil
}

// DeleteConfigUsers permanently deletes all users from the configuration
func (c *Client) DeleteConfigUsers() error {
	if c.Config == nil || len(c.Config.Users) == 0 {
		Log.Info("📋 No users found in configuration to delete")
		return nil
	}

	Log.WithFields(logrus.Fields{"user_count": len(c.Config.Users)}).Info("🗑️ Deleting users from configuration")

	for _, userConfig := range c.Config.Users {
		// Find the user by username
		user, resp, err := c.API.GetUserByUsername(context.Background(), userConfig.Username, "")
		if err != nil {
			if resp != nil && resp.StatusCode == 404 {
				Log.WithFields(logrus.Fields{"user_name": userConfig.Username}).Warn("⚠️ User not found, skipping")
				continue
			}
			return handleAPIError(fmt.Sprintf("failed to find user '%s'", userConfig.Username), err, resp)
		}

		// Permanently delete the user
		_, err = c.API.PermanentDeleteUser(context.Background(), user.Id)
		if err != nil {
			return handleAPIError(fmt.Sprintf("failed to delete user '%s'", userConfig.Username), err, nil)
		}

		Log.WithFields(logrus.Fields{"user_name": userConfig.Username}).Info("✅ Permanently deleted user")
	}

	return nil
}

// DeleteConfigTeams permanently deletes all teams from the configuration
func (c *Client) DeleteConfigTeams() error {
	if c.Config == nil || len(c.Config.Teams) == 0 {
		Log.Info("📋 No teams found in configuration to delete")
		return nil
	}

	Log.WithFields(logrus.Fields{"team_count": len(c.Config.Teams)}).Info("🗑️ Deleting teams from configuration")

	for teamName := range c.Config.Teams {
		// Find the team by name
		team, resp, err := c.API.GetTeamByName(context.Background(), teamName, "")
		if err != nil {
			if resp != nil && resp.StatusCode == 404 {
				Log.WithFields(logrus.Fields{"team_name": teamName}).Warn("⚠️ Team not found, skipping")
				continue
			}
			return handleAPIError(fmt.Sprintf("failed to find team '%s'", teamName), err, resp)
		}

		// Permanently delete the team
		_, err = c.API.PermanentDeleteTeam(context.Background(), team.Id)
		if err != nil {
			return handleAPIError(fmt.Sprintf("failed to delete team '%s'", teamName), err, nil)
		}

		Log.WithFields(logrus.Fields{"team_name": teamName}).Info("✅ Permanently deleted team")
	}

	return nil
}

// ListCustomProfileFields retrieves all custom profile fields from the server
func (c *Client) ListCustomProfileFields() ([]CustomProfileField, error) {
	url := fmt.Sprintf("%s/api/v4/custom_profile_attributes/fields", c.ServerURL)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.API.AuthToken)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get custom fields: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to get custom fields, status %d: %s", resp.StatusCode, string(body))
	}

	var fields []CustomProfileField
	if err := json.NewDecoder(resp.Body).Decode(&fields); err != nil {
		return nil, fmt.Errorf("failed to decode custom fields: %w", err)
	}

	return fields, nil
}

// CreateCustomProfileField creates a new custom profile field with extended configuration
func (c *Client) CreateCustomProfileField(name, displayName, fieldType string, options []string) (*CustomProfileField, error) {
	url := fmt.Sprintf("%s/api/v4/custom_profile_attributes/fields", c.ServerURL)

	payload := map[string]any{
		"name":         name,
		"display_name": displayName,
		"type":         fieldType,
	}

	if len(options) > 0 {
		payload["options"] = options
	}

	return c.createCustomProfileFieldWithPayload(url, payload)
}

// CreateCustomProfileFieldExtended creates a new custom profile field with full extended configuration
func (c *Client) CreateCustomProfileFieldExtended(field UserAttributeField) (*CustomProfileField, error) {
	url := fmt.Sprintf("%s/api/v4/custom_profile_attributes/fields", c.ServerURL)

	payload := map[string]any{
		"name":         field.Name,
		"display_name": field.DisplayName,
		"type":         field.Type,
	}

	// Add extended attributes based on the new JSON structure
	attrs := map[string]any{}

	if field.LDAPAttribute != "" {
		attrs["ldap"] = field.LDAPAttribute
	}
	if field.SAMLAttribute != "" {
		attrs["saml"] = field.SAMLAttribute
	}
	if len(field.Options) > 0 {
		attrs["options"] = field.Options
	}
	if field.SortOrder > 0 {
		attrs["sort_order"] = field.SortOrder
	}
	if field.ValueType != "" {
		attrs["value_type"] = field.ValueType
	}
	if field.Visibility != "" {
		attrs["visibility"] = field.Visibility
	}

	// Set attrs if we have any extended configuration
	if len(attrs) > 0 {
		payload["attrs"] = attrs
	}

	// Add basic options for backward compatibility
	if len(field.Options) > 0 {
		payload["options"] = field.Options
	}

	return c.createCustomProfileFieldWithPayload(url, payload)
}

// createCustomProfileFieldWithPayload handles the actual API call
func (c *Client) createCustomProfileFieldWithPayload(url string, payload map[string]any) (*CustomProfileField, error) {

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonPayload))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.API.AuthToken)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to create custom field: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to create custom field, status %d: %s", resp.StatusCode, string(body))
	}

	var field CustomProfileField
	if err := json.NewDecoder(resp.Body).Decode(&field); err != nil {
		return nil, fmt.Errorf("failed to decode created field: %w", err)
	}

	return &field, nil
}

// processUserAttributes processes user attribute definitions from JSONL
func (c *Client) processUserAttributes(bulkImportPath string) error {
	Log.WithFields(logrus.Fields{"file_path": bulkImportPath}).Info("📋 Processing user attributes")

	file, err := os.Open(bulkImportPath)
	if err != nil {
		return err
	}
	defer closeWithLog(file, "bulk import file")

	var attributeFields []UserAttributeField
	createdCount := 0
	errorCount := 0

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var attributeImport UserAttributeImport
		if err := json.Unmarshal([]byte(line), &attributeImport); err != nil {
			continue
		}

		if attributeImport.Type == "user-attribute" {
			attributeFields = append(attributeFields, attributeImport.Attribute)
		}
	}

	if len(attributeFields) == 0 {
		Log.Info("📋 No user attributes found, skipping")
		return nil
	}

	Log.WithFields(logrus.Fields{
		"field_count": len(attributeFields),
	}).Info("📋 Found user attributes")

	// Ensure all custom fields exist
	for _, field := range attributeFields {
		if err := c.ensureCustomFieldExists(field); err != nil {
			Log.WithFields(logrus.Fields{
				"field_name": field.Name,
				"error":      err.Error(),
			}).Warn("⚠️ Failed to ensure custom field exists")
			errorCount++
		} else {
			createdCount++
		}
	}

	Log.WithFields(logrus.Fields{
		"created_count": createdCount,
		"error_count":   errorCount,
	}).Info("✅ User attributes processing complete")

	return scanner.Err()
}

// ensureCustomFieldExists creates a custom field if it doesn't exist
func (c *Client) ensureCustomFieldExists(field UserAttributeField) error {
	// Get existing fields
	existingFields, err := c.ListCustomProfileFields()
	if err != nil {
		return fmt.Errorf("failed to list existing custom fields: %w", err)
	}

	// Check if field already exists
	for _, existingField := range existingFields {
		if existingField.Name == field.Name {
			Log.WithFields(logrus.Fields{
				"field_name": field.Name,
			}).Debug("🔍 Custom field already exists")
			return nil
		}
	}

	// Create the field with extended configuration
	Log.WithFields(logrus.Fields{
		"field_name":     field.Name,
		"display_name":   field.DisplayName,
		"field_type":     field.Type,
		"ldap_attribute": field.LDAPAttribute,
		"saml_attribute": field.SAMLAttribute,
		"options_count":  len(field.Options),
		"sort_order":     field.SortOrder,
		"value_type":     field.ValueType,
		"visibility":     field.Visibility,
	}).Info("📝 Creating custom profile field with extended configuration")

	_, err = c.CreateCustomProfileFieldExtended(field)
	if err != nil {
		return fmt.Errorf("failed to create custom field '%s': %w", field.Name, err)
	}

	Log.WithFields(logrus.Fields{
		"field_name": field.Name,
	}).Info("✅ Successfully created custom profile field")

	return nil
}

// configureGroupProperties configures group properties based on the group configuration
func (c *Client) configureGroupProperties(groupID string, group ldapPkg.LDAPGroup) error {
	// Create patch request with desired properties
	groupPatch := &model.GroupPatch{
		AllowReference: &group.AllowReference,
	}

	// Apply the configuration
	g, _, err := c.API.PatchGroup(context.Background(), groupID, groupPatch)
	if err != nil {
		return fmt.Errorf("failed to configure group properties: %w", err)
	}

	Log.WithFields(logrus.Fields{
		"group_id":        g.Id,
		"allow_reference": g.AllowReference,
	}).Debug("✅ Successfully configured group properties")

	return nil
}
