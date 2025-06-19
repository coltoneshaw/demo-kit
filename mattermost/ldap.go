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

	"github.com/go-ldap/ldap/v3"
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
	ldapAttributes := convertToLDAPAttributes(attributeFields)

	return ldapClient.ShowSchemaExtensions(ldapAttributes, schemaConfig)
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
				username := getStringFromMap(userData, "username")
				user := LDAPUser{
					Username:         username,
					Email:            getStringFromMap(userData, "email"),
					FirstName:        getStringFromMap(userData, "first_name"),
					LastName:         getStringFromMap(userData, "last_name"),
					Password:         getStringFromMap(userData, "password"),
					Position:         getStringFromMap(userData, "position"),
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

// getStringFromMap safely extracts a string value from a map
func getStringFromMap(m map[string]any, key string) string {
	if value, ok := m[key].(string); ok {
		return value
	}
	return ""
}

// getStringFromInterface safely extracts a string value from an interface
func getStringFromInterface(value any) string {
	if str, ok := value.(string); ok {
		return str
	}
	return ""
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
			username := getStringFromInterface(entry["user"])
			if username == "" {
				continue
			}

			if attributesInterface, ok := entry["attributes"]; ok {
				if attributesMap, ok := attributesInterface.(map[string]any); ok {
					attributes := make(map[string]string)
					for key, value := range attributesMap {
						if strValue := getStringFromInterface(value); strValue != "" {
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
			group, err := parseGroupEntry(line)
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
	Log.Info("👥 Setting up LDAP groups")

	// Extract groups from JSONL
	groups, err := c.extractUserGroups(c.BulkImportPath)
	if err != nil {
		return fmt.Errorf("failed to extract groups from JSONL: %w", err)
	}

	if len(groups) == 0 {
		Log.Info("📋 No groups found in JSONL, skipping group setup")
		return nil
	}

	// Connect to LDAP server
	ldapConn, err := ldap.DialURL(config.URL)
	if err != nil {
		return fmt.Errorf("failed to connect to LDAP server: %w", err)
	}
	defer func() {
		if err := ldapConn.Close(); err != nil {
			Log.WithError(err).Warn("Failed to close LDAP connection")
		}
	}()

	// Bind as admin
	if err := ldapConn.Bind(config.BindDN, config.BindPassword); err != nil {
		return fmt.Errorf("failed to bind to LDAP server: %w", err)
	}

	// Ensure schema is applied (including uniqueID attribute for groups)
	attributeFields, err := c.extractCustomAttributeDefinitions(c.BulkImportPath)
	if err != nil {
		return fmt.Errorf("failed to extract custom attribute definitions: %w", err)
	}

	if err := c.ensureCustomAttributeSchema(ldapConn, attributeFields, config); err != nil {
		return fmt.Errorf("failed to ensure custom attribute schema: %w", err)
	}

	// Create LDAP client for group operations
	ldapClient := ldapPkg.NewClient(config)

	// Create each group
	for _, group := range groups {
		if err := ldapClient.CreateGroup(ldapConn, group, config); err != nil {
			Log.WithFields(logrus.Fields{
				"group_name": group.Name,
				"error":      err.Error(),
			}).Error("Failed to create LDAP group")
			return fmt.Errorf("failed to create group %s: %w", group.Name, err)
		}
	}

	Log.WithFields(logrus.Fields{
		"group_count": len(groups),
	}).Info("✅ Successfully set up LDAP groups")

	return nil
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

// parseGroupEntry parses a single group entry from JSONL format
func parseGroupEntry(line string) (ldapPkg.LDAPGroup, error) {
	var groupImport UserGroupImport
	if err := json.Unmarshal([]byte(line), &groupImport); err != nil {
		return ldapPkg.LDAPGroup{}, fmt.Errorf("failed to unmarshal group entry: %w", err)
	}

	// Validate required fields
	if groupImport.Group.Name == "" {
		return ldapPkg.LDAPGroup{}, fmt.Errorf("group name is required")
	}
	if groupImport.Group.ID == "" {
		return ldapPkg.LDAPGroup{}, fmt.Errorf("group ID is required")
	}

	return ldapPkg.LDAPGroup{
		Name:           groupImport.Group.Name,
		UniqueID:       groupImport.Group.ID,
		Members:        groupImport.Group.Members,
		AllowReference: groupImport.Group.AllowReference,
	}, nil
}

// ensureCustomAttributeSchema ensures custom attributes are defined in the LDAP schema
func (c *Client) ensureCustomAttributeSchema(ldapConn *ldap.Conn, attributeFields []UserAttributeField, config *LDAPConfig) error {
	Log.WithFields(logrus.Fields{
		"attribute_count": len(attributeFields),
	}).Info("🔧 Ensuring custom attribute schema in LDAP")

	// Use the new schema extension system
	schemaConfig := ldapPkg.DefaultSchemaConfig()

	// Convert to LDAP package types
	ldapAttributes := convertToLDAPAttributes(attributeFields)

	// Apply schema extensions with proper attribute definitions and object classes
	ldapClient := ldapPkg.NewClient(config)
	ldapPkg.SetLogger(Log)
	if err := ldapClient.SetupSchema(ldapConn, ldapAttributes, schemaConfig, config); err != nil {
		return fmt.Errorf("failed to apply schema extensions: %w", err)
	}

	// Ensure custom object class
	if err := c.ensureCustomObjectClass(); err != nil {
		return fmt.Errorf("failed to ensure custom object class: %w", err)
	}

	// Log the custom attributes that will be used
	for _, field := range attributeFields {
		if field.LDAPAttribute != "" {
			Log.WithFields(logrus.Fields{
				"field_name":     field.Name,
				"ldap_attribute": field.LDAPAttribute,
				"display_name":   field.DisplayName,
			}).Debug("Custom attribute available for LDAP mapping")
		}
	}

	Log.Info("✅ Custom attribute schema verification completed")
	return nil
}

// convertToLDAPAttributes converts mattermost UserAttributeField to LDAP package format
func convertToLDAPAttributes(attributes []UserAttributeField) []ldapPkg.UserAttributeField {
	ldapAttrs := make([]ldapPkg.UserAttributeField, len(attributes))
	for i, attr := range attributes {
		ldapAttrs[i] = ldapPkg.UserAttributeField{
			Name:          attr.Name,
			DisplayName:   attr.DisplayName,
			Type:          attr.Type,
			LDAPAttribute: attr.LDAPAttribute,
			Required:      attr.Required,
		}
	}
	return ldapAttrs
}

// ensureCustomObjectClass ensures that the inetOrgPerson object class (which we use) supports our attributes
func (c *Client) ensureCustomObjectClass() error {
	// The rroemhild/test-openldap image includes support for many attributes through inetOrgPerson
	// and organizationalPerson object classes. Custom attributes are dynamically created based on
	// the 'ldap' field values in user-attribute definitions from the JSONL configuration.
	//
	// Note: For production LDAP servers, you may need to extend the schema to support
	// custom attributes that aren't part of the standard inetOrgPerson object class.

	Log.Debug("Using dynamic LDAP attributes from user-attribute configuration")
	return nil
}

// formatLDAPAttributesForDebug formats LDAP attributes for debug logging
func (c *Client) formatLDAPAttributesForDebug(addRequest *ldap.AddRequest) map[string]any {
	attributes := make(map[string]any)

	for _, attr := range addRequest.Attributes {
		// Don't log passwords in debug output
		if attr.Type == "userPassword" {
			attributes[attr.Type] = "[REDACTED]"
		} else {
			attributes[attr.Type] = attr.Vals
		}
	}

	return attributes
}

// GenerateLDIFContent generates LDIF content from user data with schema extensions
func (c *Client) GenerateLDIFContent(users []LDAPUser) (string, error) {
	var ldif strings.Builder

	// Check if we need schema extensions
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

	// Add schema extensions if needed
	if hasCustomAttributes && len(attributeFields) > 0 {
		schemaConfig := ldapPkg.DefaultSchemaConfig()
		ldapClient := ldapPkg.NewClient(&LDAPConfig{})
		ldapAttributes := convertToLDAPAttributes(attributeFields)
		schemaLDIF, err := ldapClient.BuildSchemaLDIF(ldapAttributes, schemaConfig)
		if err != nil {
			Log.WithFields(logrus.Fields{"error": err.Error()}).Warn("⚠️ Failed to generate schema LDIF")
		} else {
			ldif.WriteString(schemaLDIF)
			ldif.WriteString("\n")
		}
	}

	// Add base DN and organization
	ldif.WriteString("# Base DN\n")
	ldif.WriteString("dn: dc=planetexpress,dc=com\n")
	ldif.WriteString("objectClass: domain\n")
	ldif.WriteString("objectClass: top\n")
	ldif.WriteString("dc: planetexpress\n")
	ldif.WriteString("\n")

	// Add organizational unit for people
	ldif.WriteString("# Organizational Unit for People\n")
	ldif.WriteString("dn: ou=people,dc=planetexpress,dc=com\n")
	ldif.WriteString("objectClass: organizationalUnit\n")
	ldif.WriteString("objectClass: top\n")
	ldif.WriteString("ou: people\n")
	ldif.WriteString("\n")

	// Add users
	for _, user := range users {
		ldif.WriteString(fmt.Sprintf("# User: %s\n", user.Username))
		ldif.WriteString(fmt.Sprintf("dn: uid=%s,ou=people,dc=planetexpress,dc=com\n", user.Username))

		// Standard object classes
		ldif.WriteString("objectClass: inetOrgPerson\n")
		ldif.WriteString("objectClass: organizationalPerson\n")
		ldif.WriteString("objectClass: person\n")
		ldif.WriteString("objectClass: top\n")

		// Add custom object class if we have custom attributes
		if len(user.CustomAttributes) > 0 {
			schemaConfig := ldapPkg.DefaultSchemaConfig()
			ldif.WriteString(fmt.Sprintf("objectClass: %s\n", schemaConfig.ObjectClassName))
		}

		ldif.WriteString(fmt.Sprintf("uid: %s\n", user.Username))
		ldif.WriteString(fmt.Sprintf("cn: %s %s\n", user.FirstName, user.LastName))
		ldif.WriteString(fmt.Sprintf("sn: %s\n", user.LastName))
		ldif.WriteString(fmt.Sprintf("givenName: %s\n", user.FirstName))
		ldif.WriteString(fmt.Sprintf("mail: %s\n", user.Email))
		if user.Position != "" {
			ldif.WriteString(fmt.Sprintf("title: %s\n", user.Position))
		}

		// Add custom attributes
		for ldapAttr, value := range user.CustomAttributes {
			if value != "" {
				ldif.WriteString(fmt.Sprintf("%s: %s\n", ldapAttr, value))
			}
		}

		// Set password from user data or use default for demo purposes
		if user.Password != "" {
			ldif.WriteString(fmt.Sprintf("userPassword: %s\n", user.Password))
		} else {
			ldif.WriteString("userPassword: {SSHA}password123\n")
		}
		ldif.WriteString("\n")
	}

	return ldif.String(), nil
}

// importUsersToLDAPWithConfig connects directly to LDAP and creates users with custom configuration
func (c *Client) importUsersToLDAPWithConfig(users []LDAPUser, config *LDAPConfig) error {
	Log.WithFields(logrus.Fields{"user_count": len(users)}).Info("📥 Importing users directly to LDAP")

	// Connect to LDAP server
	ldapConn, err := ldap.DialURL(config.URL)
	if err != nil {
		return fmt.Errorf("failed to connect to LDAP server: %w", err)
	}
	defer func() {
		if err := ldapConn.Close(); err != nil {
			Log.WithFields(logrus.Fields{"error": err.Error()}).Warn("⚠️ Failed to close LDAP connection")
		}
	}()

	// Bind as admin
	err = ldapConn.Bind(config.BindDN, config.BindPassword)
	if err != nil {
		return fmt.Errorf("failed to bind to LDAP server: %w", err)
	}

	// Ensure base organizational structure exists
	if err := c.ensureLDAPStructureWithConfig(ldapConn, config); err != nil {
		return fmt.Errorf("failed to ensure LDAP structure: %w", err)
	}

	// Extract custom attribute definitions and ensure they exist in LDAP schema
	attributeFields, err := c.extractCustomAttributeDefinitions(c.BulkImportPath)
	if err != nil {
		return fmt.Errorf("failed to extract custom attribute definitions: %w", err)
	}

	if len(attributeFields) > 0 {
		if err := c.ensureCustomAttributeSchema(ldapConn, attributeFields, config); err != nil {
			return fmt.Errorf("failed to ensure custom attribute schema: %w", err)
		}
	}

	// Create users
	successCount := 0
	errorCount := 0

	for _, user := range users {
		if err := c.createLDAPUserWithConfig(ldapConn, user, config); err != nil {
			Log.WithFields(logrus.Fields{
				"username": user.Username,
				"error":    err.Error(),
			}).Warn("⚠️ Failed to create LDAP user")
			errorCount++
		} else {
			successCount++
		}
	}

	Log.WithFields(logrus.Fields{
		"success_count": successCount,
		"error_count":   errorCount,
	}).Info("✅ LDAP user import completed")

	if errorCount > 0 {
		return fmt.Errorf("failed to create %d out of %d users", errorCount, len(users))
	}

	return nil
}

// ensureLDAPStructureWithConfig ensures the base organizational structure exists in LDAP
func (c *Client) ensureLDAPStructureWithConfig(ldapConn *ldap.Conn, config *LDAPConfig) error {
	// Check if base DN exists
	searchRequest := ldap.NewSearchRequest(
		config.BaseDN,
		ldap.ScopeBaseObject,
		ldap.NeverDerefAliases,
		0, 0, false,
		"(objectClass=*)",
		[]string{"dn"},
		nil,
	)

	_, err := ldapConn.Search(searchRequest)
	if err != nil {
		// Base DN doesn't exist, create it
		Log.Info("🏢 Creating base LDAP organizational structure")

		// Create base domain
		addRequest := ldap.NewAddRequest(config.BaseDN, nil)
		addRequest.Attribute("objectClass", []string{"domain", "top"})
		// Extract the first DC component for the attribute
		dcValue := strings.Split(config.BaseDN, ",")[0]
		dcValue = strings.TrimPrefix(dcValue, "dc=")
		addRequest.Attribute("dc", []string{dcValue})

		if err := ldapConn.Add(addRequest); err != nil {
			Log.WithFields(logrus.Fields{"error": err.Error()}).Debug("Base DN might already exist")
		}
	}

	// Check if people OU exists
	peopleOU := fmt.Sprintf("ou=people,%s", config.BaseDN)
	searchRequest = ldap.NewSearchRequest(
		peopleOU,
		ldap.ScopeBaseObject,
		ldap.NeverDerefAliases,
		0, 0, false,
		"(objectClass=*)",
		[]string{"dn"},
		nil,
	)

	_, err = ldapConn.Search(searchRequest)
	if err != nil {
		// People OU doesn't exist, create it
		addRequest := ldap.NewAddRequest(peopleOU, nil)
		addRequest.Attribute("objectClass", []string{"organizationalUnit", "top"})
		addRequest.Attribute("ou", []string{"people"})

		if err := ldapConn.Add(addRequest); err != nil {
			return fmt.Errorf("failed to create people OU: %w", err)
		}
		Log.Info("✅ Created people organizational unit")
	}

	return nil
}

// createLDAPUserWithConfig creates a single user in LDAP
func (c *Client) createLDAPUserWithConfig(ldapConn *ldap.Conn, user LDAPUser, config *LDAPConfig) error {
	dn := fmt.Sprintf("uid=%s,ou=people,%s", user.Username, config.BaseDN)

	// Check if user already exists
	searchRequest := ldap.NewSearchRequest(
		dn,
		ldap.ScopeBaseObject,
		ldap.NeverDerefAliases,
		0, 0, false,
		"(objectClass=*)",
		[]string{"dn"},
		nil,
	)

	_, err := ldapConn.Search(searchRequest)
	if err == nil {
		// User already exists, skip
		Log.WithFields(logrus.Fields{"username": user.Username}).Debug("User already exists in LDAP, skipping")
		return nil
	}

	// Create user with appropriate object classes
	addRequest := ldap.NewAddRequest(dn, nil)

	// Standard object classes
	objectClasses := []string{"inetOrgPerson", "organizationalPerson", "person", "top"}

	// Add custom object class if we have custom attributes
	schemaConfig := ldapPkg.DefaultSchemaConfig()
	if len(user.CustomAttributes) > 0 {
		objectClasses = append(objectClasses, schemaConfig.ObjectClassName)
	}

	addRequest.Attribute("objectClass", objectClasses)
	addRequest.Attribute("uid", []string{user.Username})
	addRequest.Attribute("cn", []string{fmt.Sprintf("%s %s", user.FirstName, user.LastName)})
	addRequest.Attribute("sn", []string{user.LastName})
	addRequest.Attribute("givenName", []string{user.FirstName})
	addRequest.Attribute("mail", []string{user.Email})

	if user.Position != "" {
		addRequest.Attribute("title", []string{user.Position})
	}

	// Set password from JSONL or use default for demo purposes
	if user.Password != "" {
		addRequest.Attribute("userPassword", []string{user.Password})
	} else {
		addRequest.Attribute("userPassword", []string{"password123"})
	}

	// Add custom attributes if they exist
	for ldapAttr, value := range user.CustomAttributes {
		if value != "" {
			addRequest.Attribute(ldapAttr, []string{value})
			Log.WithFields(logrus.Fields{
				"username":  user.Username,
				"attribute": ldapAttr,
				"value":     value,
			}).Debug("Added custom LDAP attribute")
		}
	}

	// Debug: Log the complete LDAP add request
	Log.WithFields(logrus.Fields{
		"username":   user.Username,
		"dn":         dn,
		"attributes": c.formatLDAPAttributesForDebug(addRequest),
	}).Debug("Creating LDAP user with attributes")

	if err := ldapConn.Add(addRequest); err != nil {
		return fmt.Errorf("failed to add user %s: %w", user.Username, err)
	}

	Log.WithFields(logrus.Fields{"username": user.Username, "dn": dn}).Debug("Created LDAP user")
	return nil
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