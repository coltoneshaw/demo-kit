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

	"github.com/sirupsen/logrus"

	ldapPkg "github.com/coltoneshaw/demokit/mattermost/ldap"
)

// LDAPConfig is an alias for the LDAP package configuration
type LDAPConfig = ldapPkg.LDAPConfig

// SetupLDAP extracts users from JSONL and imports them directly into LDAP using default configuration
func (c *Client) SetupLDAP() error {
	defaultConfig := &LDAPConfig{
		URL:          "ldap://localhost:10389",
		BindDN:       "cn=admin,dc=planetexpress,dc=com",
		BindPassword: "GoodNewsEveryone",
		BaseDN:       "dc=planetexpress,dc=com",
	}
	return c.SetupLDAPWithConfig(defaultConfig)
}

// ShowLDAPSchemaExtensions displays the LDAP schema extensions that would be applied
func (c *Client) ShowLDAPSchemaExtensions() error {
	Log.Info("🔍 Showing LDAP schema extensions for custom attributes")

	// Extract custom attribute definitions from JSONL (Mattermost-specific logic)
	attributeFields, err := c.extractCustomAttributeDefinitions(c.BulkImportPath)
	if err != nil {
		return fmt.Errorf("failed to extract custom attribute definitions: %w", err)
	}

	// Convert to LDAP package types and delegate to LDAP package
	schemaConfig := ldapPkg.DefaultSchemaConfig()
	ldapClient := ldapPkg.NewClient(&LDAPConfig{})

	return ldapClient.ShowSchemaExtensions(attributeFields, schemaConfig)
}

// SetupLDAPWithConfig extracts users from JSONL and imports them directly into LDAP with custom configuration
func (c *Client) SetupLDAPWithConfig(config *LDAPConfig) error {
	Log.WithFields(logrus.Fields{
		"ldap_url": config.URL,
		"bind_dn":  config.BindDN,
		"base_dn":  config.BaseDN,
	}).Info("🔐 Starting LDAP setup")

	// Extract users from JSONL
	users, err := c.ExtractUsersFromJSONL(c.BulkImportPath)
	if err != nil {
		return fmt.Errorf("failed to extract users from JSONL: %w", err)
	}

	// Import users directly into LDAP
	if err := c.importUsersToLDAPWithConfig(users, config); err != nil {
		return fmt.Errorf("failed to import users to LDAP: %w", err)
	}

	// Setup LDAP groups
	if err := c.setupLDAPGroups(config); err != nil {
		return fmt.Errorf("failed to setup LDAP groups: %w", err)
	}

	// Migrate existing Mattermost users from email auth to LDAP auth
	if err := c.migrateUsersToLDAPAuth(users); err != nil {
		return fmt.Errorf("failed to migrate users to LDAP auth: %w", err)
	}

	// Link LDAP groups to Mattermost (optional - API may not be available)
	if err := c.linkLDAPGroups(); err != nil {
		Log.WithFields(logrus.Fields{
			"error": err.Error(),
		}).Warn("Failed to link LDAP groups to Mattermost API (groups still created in LDAP)")
		// Don't fail the entire setup if API linking fails
	}

	// Trigger LDAP sync to ensure Mattermost picks up all LDAP attributes and groups
	Log.Info("🔄 Triggering LDAP sync to update user attributes and groups")
	if err := c.syncLDAP(); err != nil {
		return fmt.Errorf("failed to sync LDAP: %w", err)
	}

	Log.Info("✅ LDAP setup completed successfully")
	return nil
}

// syncLDAP triggers an LDAP sync to ensure Mattermost picks up all LDAP attributes
func (c *Client) syncLDAP() error {
	Log.Info("🔄 Starting LDAP sync...")

	// SyncLdap requires a boolean parameter for includeRemovedMembers
	includeRemovedMembers := false
	resp, err := c.API.SyncLdap(context.Background(), &includeRemovedMembers)
	if err != nil {
		return handleAPIError("failed to trigger LDAP sync", err, resp)
	}

	Log.Info("✅ LDAP sync completed successfully")
	return nil
}

// ExtractUsersFromJSONL extracts user data from the JSONL file
func (c *Client) ExtractUsersFromJSONL(jsonlPath string) ([]LDAPUser, error) {
	// First, extract custom attribute definitions to know what attributes exist
	attributeFields, err := c.extractCustomAttributeDefinitions(jsonlPath)
	if err != nil {
		return nil, fmt.Errorf("failed to extract custom attribute definitions: %w", err)
	}

	// Extract user profile assignments
	userProfiles, err := c.extractUserProfiles(jsonlPath)
	if err != nil {
		return nil, fmt.Errorf("failed to extract user profiles: %w", err)
	}

	file, err := os.Open(jsonlPath)
	if err != nil {
		return nil, err
	}
	defer closeWithLog(file, "JSONL file")

	var users []LDAPUser
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}

		if entryType, ok := entry["type"].(string); ok && entryType == "user" {
			if userData, ok := entry["user"].(map[string]any); ok {
				username := ldapPkg.GetStringFromMap(userData, "username")
				user := LDAPUser{
					Username:         username,
					Email:            ldapPkg.GetStringFromMap(userData, "email"),
					FirstName:        ldapPkg.GetStringFromMap(userData, "first_name"),
					LastName:         ldapPkg.GetStringFromMap(userData, "last_name"),
					Password:         ldapPkg.GetStringFromMap(userData, "password"),
					Position:         ldapPkg.GetStringFromMap(userData, "position"),
					CustomAttributes: make(map[string]string),
				}

				// Extract custom attributes from user-profile entries
				user.CustomAttributes = c.extractCustomAttributesFromProfiles(username, userProfiles, attributeFields)

				users = append(users, user)
			}
		}
	}

	return users, scanner.Err()
}


// extractCustomAttributeDefinitions extracts custom attribute definitions from JSONL
func (c *Client) extractCustomAttributeDefinitions(jsonlPath string) ([]UserAttributeField, error) {
	file, err := os.Open(jsonlPath)
	if err != nil {
		return nil, err
	}
	defer closeWithLog(file, "JSONL file for attribute definitions")

	var attributeFields []UserAttributeField
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

	return attributeFields, scanner.Err()
}

// extractUserProfiles extracts user profile assignments from JSONL
func (c *Client) extractUserProfiles(jsonlPath string) (map[string]map[string]string, error) {
	file, err := os.Open(jsonlPath)
	if err != nil {
		return nil, err
	}
	defer closeWithLog(file, "JSONL file for user profiles")

	userProfiles := make(map[string]map[string]string)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}

		if entryType, ok := entry["type"].(string); ok && entryType == "user-profile" {
			username := ldapPkg.GetStringFromInterface(entry["user"])
			if username == "" {
				continue
			}

			if attributesInterface, ok := entry["attributes"]; ok {
				if attributesMap, ok := attributesInterface.(map[string]any); ok {
					attributes := make(map[string]string)
					for key, value := range attributesMap {
						if strValue := ldapPkg.GetStringFromInterface(value); strValue != "" {
							if len(strValue) > 64 {
								return nil, fmt.Errorf("user attribute '%s' for user '%s' exceeds 64 character limit (length: %d): %s", key, username, len(strValue), strValue)
							}
							attributes[key] = strValue
						}
					}
					userProfiles[username] = attributes
				}
			}
		}
	}

	return userProfiles, scanner.Err()
}

// extractCustomAttributesFromProfiles extracts custom attribute values from user-profile entries
func (c *Client) extractCustomAttributesFromProfiles(username string, userProfiles map[string]map[string]string, attributeFields []UserAttributeField) map[string]string {
	customAttributes := make(map[string]string)

	// Get the user's profile attributes if they exist
	userAttributes, exists := userProfiles[username]
	if !exists {
		Log.WithFields(logrus.Fields{
			"username": username,
		}).Debug("No user-profile entry found for user")
		return customAttributes
	}

	// Map profile attributes to LDAP attributes
	for _, field := range attributeFields {
		if field.LDAPAttribute == "" {
			continue // Skip if no LDAP mapping
		}

		// Look for the attribute value in the user's profile
		if value, exists := userAttributes[field.Name]; exists && value != "" {
			if len(value) > 64 {
				Log.WithFields(logrus.Fields{
					"username":   username,
					"field_name": field.Name,
					"value":      value,
					"length":     len(value),
				}).Error("User attribute exceeds 64 character limit")
				return nil
			}
			customAttributes[field.LDAPAttribute] = value
			Log.WithFields(logrus.Fields{
				"username":       username,
				"field_name":     field.Name,
				"ldap_attribute": field.LDAPAttribute,
				"value":          value,
			}).Debug("Mapped user profile attribute to LDAP")
		}
	}

	return customAttributes
}

// extractUserGroups extracts user groups from JSONL file
func (c *Client) extractUserGroups(jsonlPath string) ([]ldapPkg.LDAPGroup, error) {
	file, err := os.Open(jsonlPath)
	if err != nil {
		return nil, err
	}
	defer closeWithLog(file, "JSONL file")

	var groups []ldapPkg.LDAPGroup
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}

		// Process user-groups entries
		if entryType, ok := entry["type"].(string); ok && entryType == "user-groups" {
			group, err := ldapPkg.ParseGroupEntry(entry)
			if err != nil {
				Log.WithFields(logrus.Fields{
					"error": err.Error(),
					"line":  line,
				}).Warn("⚠️ Failed to parse user-groups entry, skipping")
				continue
			}

			groups = append(groups, group)

			Log.WithFields(logrus.Fields{
				"group_name":      group.Name,
				"unique_id":       group.UniqueID,
				"member_count":    len(group.Members),
				"allow_reference": group.AllowReference,
			}).Debug("📋 Extracted user group from JSONL")
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading JSONL file: %w", err)
	}

	if len(groups) == 0 {
		Log.Info("📋 No user groups found in JSONL")
	} else {
		Log.WithFields(logrus.Fields{
			"group_count": len(groups),
		}).Info("📋 Successfully extracted user groups from JSONL")
	}

	return groups, nil
}

// setupLDAPGroups creates and configures LDAP groups from JSONL data
func (c *Client) setupLDAPGroups(config *LDAPConfig) error {
	// Extract groups from JSONL
	groups, err := c.extractUserGroups(c.BulkImportPath)
	if err != nil {
		return fmt.Errorf("failed to extract groups from JSONL: %w", err)
	}

	// Extract attribute fields for schema
	attributeFields, err := c.extractCustomAttributeDefinitions(c.BulkImportPath)
	if err != nil {
		return fmt.Errorf("failed to extract custom attribute definitions: %w", err)
	}

	// Delegate to LDAP package
	ldapClient := ldapPkg.NewClient(config)
	return ldapClient.SetupLDAPGroups(groups, attributeFields, config)
}

// linkLDAPGroups links LDAP groups to Mattermost via API
func (c *Client) linkLDAPGroups() error {
	Log.Info("🔗 Linking LDAP groups to Mattermost")

	// Extract groups from JSONL to get group information
	groups, err := c.extractUserGroups(c.BulkImportPath)
	if err != nil {
		return fmt.Errorf("failed to extract groups from JSONL: %w", err)
	}

	if len(groups) == 0 {
		Log.Info("📋 No groups found, skipping group linking")
		return nil
	}

	linkedCount := 0
	for _, group := range groups {
		if err := c.linkSingleLDAPGroup(group); err != nil {
			Log.WithFields(logrus.Fields{
				"group_name": group.Name,
				"error":      err.Error(),
			}).Warn("Failed to link LDAP group to Mattermost")
			// Continue with other groups instead of failing completely
			continue
		}
		linkedCount++
	}

	Log.WithFields(logrus.Fields{
		"linked_count": linkedCount,
		"total_count":  len(groups),
	}).Info("✅ Linked LDAP groups to Mattermost")

	return nil
}

// linkSingleLDAPGroup links a single LDAP group to Mattermost and configures its properties
func (c *Client) linkSingleLDAPGroup(group ldapPkg.LDAPGroup) error {
	logFields := logrus.Fields{
		"group_name": group.Name,
		"unique_id":  group.UniqueID,
	}
	Log.WithFields(logFields).Debug("Linking LDAP group to Mattermost")

	// Link the LDAP group to Mattermost
	linkedGroup, _, err := c.API.LinkLdapGroup(context.Background(), group.Name)
	if err != nil {
		return fmt.Errorf("failed to link LDAP group '%s': %w", group.Name, err)
	}

	// Configure group properties if needed
	if group.AllowReference {
		if err := c.configureGroupProperties(linkedGroup.Id, group); err != nil {
			Log.WithFields(logrus.Fields{
				"group_name": group.Name,
				"group_id":   linkedGroup.Id,
				"error":      err.Error(),
			}).Warn("⚠️ Failed to configure group properties - group linked but not fully configured")
			// Continue - don't fail the entire operation for property configuration
		}
	}

	logFields["allow_reference"] = group.AllowReference
	Log.WithFields(logFields).Info("✅ Successfully linked and configured LDAP group")
	return nil
}







// importUsersToLDAPWithConfig connects directly to LDAP and creates users with custom configuration
func (c *Client) importUsersToLDAPWithConfig(users []LDAPUser, config *LDAPConfig) error {
	// Extract custom attribute definitions
	attributeFields, err := c.extractCustomAttributeDefinitions(c.BulkImportPath)
	if err != nil {
		return fmt.Errorf("failed to extract custom attribute definitions: %w", err)
	}

	// Delegate to LDAP package
	ldapClient := ldapPkg.NewClient(config)
	return ldapClient.ImportUsersToLDAP(users, attributeFields, config)
}



// migrateUsersToLDAPAuth migrates existing Mattermost users from email authentication to LDAP authentication
func (c *Client) migrateUsersToLDAPAuth(users []LDAPUser) error {
	Log.WithFields(logrus.Fields{"user_count": len(users)}).Info("🔄 Migrating users from email auth to LDAP auth")

	successCount := 0
	errorCount := 0

	for _, user := range users {
		Log.WithFields(logrus.Fields{"username": user.Username}).Debug("Attempting to migrate user from JSONL to LDAP auth")

		if err := c.migrateUserToLDAP(user.Username); err != nil {
			Log.WithFields(logrus.Fields{
				"username": user.Username,
				"error":    err.Error(),
			}).Warn("⚠️ Failed to migrate user to LDAP auth")
			errorCount++
		} else {
			successCount++
		}
	}

	Log.WithFields(logrus.Fields{
		"success_count": successCount,
		"error_count":   errorCount,
	}).Info("✅ User auth migration completed")

	if errorCount > 0 {
		Log.WithFields(logrus.Fields{"error_count": errorCount}).Warn("Some users failed to migrate to LDAP auth")
	}

	return nil
}

// migrateUserToLDAP migrates a single user from email authentication to LDAP authentication
func (c *Client) migrateUserToLDAP(username string) error {
	// First, find the user by username to get their user ID
	user, resp, err := c.API.GetUserByUsername(context.Background(), username, "")
	if err != nil {
		if resp != nil && resp.StatusCode == 404 {
			Log.WithFields(logrus.Fields{"username": username}).Debug("User not found in Mattermost, skipping migration")
			return nil
		}
		return fmt.Errorf("failed to find user '%s': %w", username, err)
	}

	// Update user authentication to LDAP
	url := fmt.Sprintf("%s/api/v4/users/%s/auth", c.ServerURL, user.Id)

	payload := map[string]any{
		"auth_data":    username, // Use username as auth_data for LDAP
		"auth_service": "ldap",
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal auth update payload: %w", err)
	}

	Log.WithFields(logrus.Fields{
		"username": username,
		"user_id":  user.Id,
		"service":  "ldap",
	}).Debug("Updating user authentication method to LDAP")

	req, err := http.NewRequest("PUT", url, bytes.NewBuffer(jsonPayload))
	if err != nil {
		return fmt.Errorf("failed to create auth update request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.API.AuthToken)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	httpResp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to execute auth update request: %w", err)
	}
	defer func() { _ = httpResp.Body.Close() }()

	if httpResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(httpResp.Body)
		return fmt.Errorf("failed to update user auth, status %d: %s", httpResp.StatusCode, string(body))
	}

	Log.WithFields(logrus.Fields{"username": username}).Debug("Successfully updated user to LDAP auth")
	return nil
}

// GenerateLDIFContent generates LDIF content from user data with schema extensions
func (c *Client) GenerateLDIFContent(users []LDAPUser) (string, error) {
	// Extract custom attribute definitions if needed
	var attributeFields []UserAttributeField
	hasCustomAttributes := false
	for _, user := range users {
		if len(user.CustomAttributes) > 0 {
			hasCustomAttributes = true
			break
		}
	}

	// If we have custom attributes, extract attribute definitions for schema generation
	if hasCustomAttributes {
		var err error
		attributeFields, err = c.extractCustomAttributeDefinitions(c.BulkImportPath)
		if err != nil {
			Log.WithFields(logrus.Fields{"error": err.Error()}).Warn("⚠️ Failed to extract attribute definitions for LDIF schema")
		}
	}

	// Delegate to LDAP package
	ldapClient := ldapPkg.NewClient(&LDAPConfig{})
	return ldapClient.GenerateLDIFContent(users, attributeFields)
}