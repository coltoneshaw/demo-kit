package ldap

import (
	"fmt"
	"strings"

	"github.com/go-ldap/ldap/v3"
	"github.com/sirupsen/logrus"
)

// LDAPUser represents a user that will be created in LDAP
type LDAPUser struct {
	Username         string
	Email            string
	FirstName        string
	LastName         string
	Password         string
	Position         string
	CustomAttributes map[string]string
}

// EnsureLDAPStructure ensures the base organizational structure exists in LDAP
func (c *Client) EnsureLDAPStructure(ldapConn *ldap.Conn, config *LDAPConfig) error {
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

// CreateLDAPUser creates a single user in LDAP
func (c *Client) CreateLDAPUser(ldapConn *ldap.Conn, user LDAPUser, config *LDAPConfig) error {
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
	schemaConfig := DefaultSchemaConfig()
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
		"attributes": FormatLDAPAttributesForDebug(addRequest),
	}).Debug("Creating LDAP user with attributes")

	if err := ldapConn.Add(addRequest); err != nil {
		return fmt.Errorf("failed to add user %s: %w", user.Username, err)
	}

	Log.WithFields(logrus.Fields{"username": user.Username, "dn": dn}).Debug("Created LDAP user")
	return nil
}

// FormatLDAPAttributesForDebug formats LDAP attributes for debug logging
func FormatLDAPAttributesForDebug(addRequest *ldap.AddRequest) map[string]any {
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

// ImportUsersToLDAP connects directly to LDAP and creates users with custom configuration
func (c *Client) ImportUsersToLDAP(users []LDAPUser, attributeFields []UserAttributeField, config *LDAPConfig) error {
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
	if err := c.EnsureLDAPStructure(ldapConn, config); err != nil {
		return fmt.Errorf("failed to ensure LDAP structure: %w", err)
	}

	// Ensure custom attribute schema if needed
	if len(attributeFields) > 0 {
		if err := c.EnsureCustomAttributeSchema(ldapConn, attributeFields, config); err != nil {
			return fmt.Errorf("failed to ensure custom attribute schema: %w", err)
		}
	}

	// Create users
	successCount := 0
	errorCount := 0

	for _, user := range users {
		if err := c.CreateLDAPUser(ldapConn, user, config); err != nil {
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