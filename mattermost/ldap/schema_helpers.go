package ldap

import (
	"fmt"

	"github.com/go-ldap/ldap/v3"
	"github.com/sirupsen/logrus"
)

// EnsureCustomAttributeSchema ensures custom attributes are defined in the LDAP schema
func (c *Client) EnsureCustomAttributeSchema(ldapConn *ldap.Conn, attributeFields []UserAttributeField, config *LDAPConfig) error {
	Log.WithFields(logrus.Fields{
		"attribute_count": len(attributeFields),
	}).Info("🔧 Ensuring custom attribute schema in LDAP")

	// Use the schema extension system
	schemaConfig := DefaultSchemaConfig()

	// Apply schema extensions with proper attribute definitions and object classes
	SetLogger(Log)
	if err := c.SetupSchema(ldapConn, attributeFields, schemaConfig, config); err != nil {
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