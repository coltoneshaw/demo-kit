package ldap

import (
	"fmt"
	"strings"

	"github.com/sirupsen/logrus"
)

// GenerateLDIFContent generates LDIF content from user data with schema extensions
func (c *Client) GenerateLDIFContent(users []LDAPUser, attributeFields []UserAttributeField) (string, error) {
	var ldif strings.Builder

	// Check if we need schema extensions
	hasCustomAttributes := false
	for _, user := range users {
		if len(user.CustomAttributes) > 0 {
			hasCustomAttributes = true
			break
		}
	}

	// Add schema extensions if needed
	if hasCustomAttributes && len(attributeFields) > 0 {
		schemaConfig := DefaultSchemaConfig()
		schemaLDIF, err := c.BuildSchemaLDIF(attributeFields, schemaConfig)
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
			schemaConfig := DefaultSchemaConfig()
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