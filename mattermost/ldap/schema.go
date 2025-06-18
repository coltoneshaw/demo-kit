package ldap

import (
	"fmt"
	"strings"

	"github.com/go-ldap/ldap/v3"
	"github.com/sirupsen/logrus"
)

// SetupSchema creates custom LDAP attributes and object classes
func (c *Client) SetupSchema(ldapConn *ldap.Conn, attributeFields []UserAttributeField, schemaConfig *SchemaConfig, ldapConfig *LDAPConfig) error {
	if len(attributeFields) == 0 {
		Log.Info("📋 No custom attributes found, skipping schema extensions")
		return nil
	}

	Log.WithFields(logrus.Fields{
		"attribute_count": len(attributeFields),
		"object_class":    schemaConfig.ObjectClassName,
	}).Info("🔧 Applying LDAP schema extensions")

	// Apply schema modifications to OpenLDAP
	return c.CreateSchemaItems(ldapConn, attributeFields, schemaConfig, ldapConfig)
}

// CreateSchemaItems creates attributes and object class in LDAP schema
// Orchestrates creation of custom attributes and auxiliary object class
func (c *Client) CreateSchemaItems(ldapConn *ldap.Conn, attributeFields []UserAttributeField, schemaConfig *SchemaConfig, ldapConfig *LDAPConfig) error {
	Log.Info("🔧 Applying LDAP schema modifications")
	
	// Apply custom attribute definitions
	if err := c.CreateAttributes(attributeFields, schemaConfig, ldapConfig); err != nil {
		return fmt.Errorf("failed to apply custom attribute types: %w", err)
	}

	// Apply auxiliary object class
	if err := c.CreateObjectClass(attributeFields, schemaConfig, ldapConfig); err != nil {
		return fmt.Errorf("failed to apply auxiliary object class: %w", err)
	}

	Log.Info("✅ Successfully applied LDAP schema modifications")
	return nil
}

// CreateAttributes adds custom attribute definitions to LDAP schema
// Checks if attributes exist first, only creates new ones
func (c *Client) CreateAttributes(attributeFields []UserAttributeField, schemaConfig *SchemaConfig, ldapConfig *LDAPConfig) error {
	Log.Info("📝 Applying custom attribute type definitions")

	// Create schema admin connection
	ldapConn, err := c.ConnectToSchema(ldapConfig)
	if err != nil {
		return fmt.Errorf("failed to connect to LDAP schema admin: %w", err)
	}
	defer func() {
		if err := ldapConn.Close(); err != nil {
			Log.WithError(err).Warn("Failed to close LDAP connection")
		}
	}()

	schemaDN := "cn={0}core,cn=schema,cn=config"
	createdCount := 0
	skippedCount := 0
	
	// First, ensure uniqueID attribute exists for groups
	if err := c.ensureUniqueIDAttribute(ldapConn, schemaDN, schemaConfig); err != nil {
		return fmt.Errorf("failed to ensure uniqueID attribute: %w", err)
	}
	
	for i, field := range attributeFields {
		if field.LDAPAttribute == "" {
			continue
		}

		// Check if attribute already exists
		exists, err := c.AttributeExists(ldapConn, field.LDAPAttribute)
		if err != nil {
			Log.WithFields(logrus.Fields{
				"attribute": field.LDAPAttribute,
				"error":    err.Error(),
			}).Warn("Failed to check if attribute exists, attempting to create anyway")
		} else if exists {
			Log.WithFields(logrus.Fields{
				"attribute": field.LDAPAttribute,
			}).Debug("Custom attribute already exists, skipping creation")
			skippedCount++
			continue
		}

		oid := fmt.Sprintf("%s.1.%d", schemaConfig.BaseOID, schemaConfig.AttributeOIDStart+i)
		syntax := GetSyntax(field.Type)

		// Create attribute type definition
		attributeTypeDef := fmt.Sprintf("( %s NAME '%s' DESC '%s' SYNTAX '%s' SINGLE-VALUE )",
			oid, field.LDAPAttribute, field.DisplayName, syntax)

		// Apply the modification
		modifyRequest := ldap.NewModifyRequest(schemaDN, nil)
		modifyRequest.Add("olcAttributetypes", []string{attributeTypeDef})

		Log.WithFields(logrus.Fields{
			"attribute": field.LDAPAttribute,
			"oid":      oid,
			"syntax":   syntax,
		}).Debug("Adding custom attribute type")

		if err := ldapConn.Modify(modifyRequest); err != nil {
			return fmt.Errorf("failed to add attribute type %s: %w", field.LDAPAttribute, err)
		}
		createdCount++
	}

	Log.WithFields(logrus.Fields{
		"total":   len(attributeFields),
		"created": createdCount,
		"skipped": skippedCount,
	}).Info("✅ Successfully applied custom attribute types")

	return nil
}

// CreateObjectClass adds auxiliary object class to LDAP schema  
// Checks if object class exists first, only creates if missing
func (c *Client) CreateObjectClass(attributeFields []UserAttributeField, schemaConfig *SchemaConfig, ldapConfig *LDAPConfig) error {
	Log.Info("🏗️ Applying auxiliary object class")

	// Create schema admin connection
	ldapConn, err := c.ConnectToSchema(ldapConfig)
	if err != nil {
		return fmt.Errorf("failed to connect to LDAP schema admin: %w", err)
	}
	defer func() {
		if err := ldapConn.Close(); err != nil {
			Log.WithError(err).Warn("Failed to close LDAP connection")
		}
	}()

	schemaDN := "cn={0}core,cn=schema,cn=config"
	
	// Build list of MAY attributes
	var mayAttributes []string
	for _, field := range attributeFields {
		if field.LDAPAttribute != "" {
			mayAttributes = append(mayAttributes, field.LDAPAttribute)
		}
	}
	
	// Always include uniqueID for groups support
	mayAttributes = append(mayAttributes, "uniqueID")

	// Check if object class already exists
	exists, err := c.ObjectClassExists(ldapConn, schemaConfig.ObjectClassName)
	if err != nil {
		Log.WithFields(logrus.Fields{
			"object_class": schemaConfig.ObjectClassName,
			"error":       err.Error(),
		}).Warn("Failed to check if object class exists, attempting to create anyway")
	} else if exists {
		Log.WithFields(logrus.Fields{
			"object_class": schemaConfig.ObjectClassName,
		}).Info("Auxiliary object class already exists, skipping creation")
		return nil
	}

	// Create object class definition
	objectClassDef := fmt.Sprintf("( %s NAME '%s' DESC '%s' AUXILIARY MAY ( %s ) )",
		schemaConfig.ObjectClassOID,
		schemaConfig.ObjectClassName,
		"Auxiliary object class for Mattermost custom attributes",
		strings.Join(mayAttributes, " $ "))

	// Apply the modification
	modifyRequest := ldap.NewModifyRequest(schemaDN, nil)
	modifyRequest.Add("olcObjectClasses", []string{objectClassDef})

	Log.WithFields(logrus.Fields{
		"object_class": schemaConfig.ObjectClassName,
		"oid":         schemaConfig.ObjectClassOID,
		"attributes":  mayAttributes,
	}).Debug("Adding auxiliary object class")

	if err := ldapConn.Modify(modifyRequest); err != nil {
		return fmt.Errorf("failed to add object class %s: %w", schemaConfig.ObjectClassName, err)
	}

	Log.Info("✅ Successfully applied auxiliary object class")
	return nil
}

// AttributeExists checks if attribute is already defined in schema
func (c *Client) AttributeExists(ldapConn *ldap.Conn, attributeName string) (bool, error) {
	// Search for the attribute in the subschema
	searchRequest := ldap.NewSearchRequest(
		"cn=Subschema", // Base DN for subschema
		ldap.ScopeBaseObject,
		ldap.NeverDerefAliases,
		0, 0, false,
		"(objectClass=*)", // Filter
		[]string{"attributeTypes"}, // Attributes to return
		nil,
	)

	searchResult, err := ldapConn.Search(searchRequest)
	if err != nil {
		return false, fmt.Errorf("failed to search subschema: %w", err)
	}

	if len(searchResult.Entries) == 0 {
		return false, nil
	}

	// Check if our attribute name appears in any attributeTypes definition
	attributeTypes := searchResult.Entries[0].GetAttributeValues("attributeTypes")
	for _, attrType := range attributeTypes {
		// Look for NAME 'attributeName' in the attribute type definition
		if strings.Contains(attrType, fmt.Sprintf("NAME '%s'", attributeName)) {
			return true, nil
		}
	}

	return false, nil
}

// ObjectClassExists checks if object class is already defined in schema
func (c *Client) ObjectClassExists(ldapConn *ldap.Conn, objectClassName string) (bool, error) {
	// Search for the object class in the subschema
	searchRequest := ldap.NewSearchRequest(
		"cn=Subschema", // Base DN for subschema
		ldap.ScopeBaseObject,
		ldap.NeverDerefAliases,
		0, 0, false,
		"(objectClass=*)", // Filter
		[]string{"objectClasses"}, // Attributes to return
		nil,
	)

	searchResult, err := ldapConn.Search(searchRequest)
	if err != nil {
		return false, fmt.Errorf("failed to search subschema: %w", err)
	}

	if len(searchResult.Entries) == 0 {
		return false, nil
	}

	// Check if our object class name appears in any objectClasses definition
	objectClasses := searchResult.Entries[0].GetAttributeValues("objectClasses")
	for _, objClass := range objectClasses {
		// Look for NAME 'objectClassName' in the object class definition
		if strings.Contains(objClass, fmt.Sprintf("NAME '%s'", objectClassName)) {
			return true, nil
		}
	}

	return false, nil
}

// AddObjectClassToUser adds custom object class to user entry
func (c *Client) AddObjectClassToUser(ldapConn *ldap.Conn, userDN string, schemaConfig *SchemaConfig) error {

	Log.WithFields(logrus.Fields{
		"user_dn":     userDN,
		"object_class": schemaConfig.ObjectClassName,
	}).Debug("🔧 Adding custom object class to user")

	// Modify the user entry to add the auxiliary object class
	modifyRequest := ldap.NewModifyRequest(userDN, nil)
	modifyRequest.Add("objectClass", []string{schemaConfig.ObjectClassName})

	if err := ldapConn.Modify(modifyRequest); err != nil {
		// Check if the error is because the object class is already present
		if strings.Contains(err.Error(), "attribute 'objectClass' exists") {
			Log.WithFields(logrus.Fields{
				"user_dn": userDN,
			}).Debug("Custom object class already present on user")
			return nil
		}
		return fmt.Errorf("failed to add custom object class to user: %w", err)
	}

	Log.WithFields(logrus.Fields{
		"user_dn":     userDN,
		"object_class": schemaConfig.ObjectClassName,
	}).Debug("✅ Added custom object class to user")

	return nil
}

// GetSyntax returns LDAP syntax OID for field type
func GetSyntax(fieldType string) string {
	switch fieldType {
	case "text":
		return "1.3.6.1.4.1.1466.115.121.1.15" // Directory String
	case "number":
		return "1.3.6.1.4.1.1466.115.121.1.27" // Integer
	case "boolean":
		return "1.3.6.1.4.1.1466.115.121.1.7" // Boolean
	case "select":
		return "1.3.6.1.4.1.1466.115.121.1.15" // Directory String for select options
	default:
		return "1.3.6.1.4.1.1466.115.121.1.15" // Default to Directory String
	}
}

// GetMatchRule returns LDAP equality matching rule for field type
func GetMatchRule(fieldType string) string {
	switch fieldType {
	case "text", "select":
		return "caseIgnoreMatch"
	case "number":
		return "integerMatch"
	case "boolean":
		return "booleanMatch"
	default:
		return "caseIgnoreMatch"
	}
}

// BuildSchemaLDIF generates LDIF content for schema extensions  
func (c *Client) BuildSchemaLDIF(attributeFields []UserAttributeField, config *SchemaConfig) (string, error) {
	var ldif strings.Builder

	Log.WithFields(logrus.Fields{
		"attribute_count": len(attributeFields),
		"base_oid":        config.BaseOID,
	}).Info("🔧 Generating LDAP schema LDIF")

	// Generate attribute type definitions
	attributeDefs := BuildAttributeDefs(attributeFields, config)

	// Add schema entry header
	ldif.WriteString("# LDAP Schema Extensions for Mattermost Custom Attributes\n")
	ldif.WriteString("# Generated automatically - do not edit manually\n\n")

	// Add uniqueID attribute for groups
	ldif.WriteString("# uniqueID Attribute for Groups\n")
	ldif.WriteString("dn: cn={0}core,cn=schema,cn=config\n")
	ldif.WriteString("changetype: modify\n")
	ldif.WriteString("add: olcAttributetypes\n")
	uniqueIDAttr := fmt.Sprintf("olcAttributetypes: ( %s.2.107 NAME 'uniqueID' DESC 'Unique identifier for groups' SYNTAX '1.3.6.1.4.1.1466.115.121.1.15' EQUALITY caseIgnoreMatch SINGLE-VALUE )", config.BaseOID)
	ldif.WriteString(uniqueIDAttr + "\n")
	ldif.WriteString("-\n\n")

	// Add attribute type definitions
	for _, attrDef := range attributeDefs {
		ldif.WriteString(fmt.Sprintf("# Attribute Type: %s\n", attrDef.Name))
		ldif.WriteString("dn: cn={0}core,cn=schema,cn=config\n")
		ldif.WriteString("changetype: modify\n")
		ldif.WriteString("add: olcAttributetypes\n")
		
		// Build attribute definition
		attrLine := fmt.Sprintf("olcAttributetypes: ( %s NAME '%s' DESC '%s' SYNTAX '%s'",
			attrDef.OID, attrDef.Name, attrDef.Description, attrDef.Syntax)
			
		if attrDef.Equality != "" {
			attrLine += fmt.Sprintf(" EQUALITY %s", attrDef.Equality)
		}
		if attrDef.Ordering != "" {
			attrLine += fmt.Sprintf(" ORDERING %s", attrDef.Ordering)
		}
		if attrDef.Substr != "" {
			attrLine += fmt.Sprintf(" SUBSTR %s", attrDef.Substr)
		}
		if attrDef.SingleValue {
			attrLine += " SINGLE-VALUE"
		}
		attrLine += " )"
		
		ldif.WriteString(attrLine + "\n")
		ldif.WriteString("-\n\n")
	}

	// Build auxiliary object class
	var mayAttributes []string
	for _, field := range attributeFields {
		if field.LDAPAttribute != "" {
			mayAttributes = append(mayAttributes, field.LDAPAttribute)
		}
	}
	
	// Always include uniqueID for groups support
	mayAttributes = append(mayAttributes, "uniqueID")

	ldif.WriteString("# Auxiliary Object Class for Custom Attributes\n")
	ldif.WriteString("dn: cn={0}core,cn=schema,cn=config\n")
	ldif.WriteString("changetype: modify\n")
	ldif.WriteString("add: olcObjectClasses\n")
	objectClassLine := fmt.Sprintf("olcObjectClasses: ( %s NAME '%s' DESC '%s' AUXILIARY MAY ( %s ) )",
		config.ObjectClassOID,
		config.ObjectClassName,
		"Auxiliary object class for Mattermost custom attributes and groups",
		strings.Join(mayAttributes, " $ "))
	ldif.WriteString(objectClassLine + "\n")
	ldif.WriteString("-\n\n")

	return ldif.String(), nil
}

// ShowSchemaExtensions displays LDAP schema extensions for custom attributes
func (c *Client) ShowSchemaExtensions(attributeFields []UserAttributeField, schemaConfig *SchemaConfig) error {
	if len(attributeFields) == 0 {
		Log.Info("📋 No custom attributes found")
		return nil
	}

	// Generate schema LDIF
	schemaLDIF, err := c.BuildSchemaLDIF(attributeFields, schemaConfig)
	if err != nil {
		return fmt.Errorf("failed to generate schema LDIF: %w", err)
	}

	Log.WithFields(logrus.Fields{
		"attribute_count": len(attributeFields),
		"base_oid":        schemaConfig.BaseOID,
		"object_class":    schemaConfig.ObjectClassName,
	}).Info("📋 Schema Extension Summary")

	Log.Info("📋 LDAP Schema Extensions LDIF:")
	fmt.Println(schemaLDIF)

	return nil
}

// BuildAttributeDefs creates LDAP attribute definitions from user fields
func BuildAttributeDefs(attributeFields []UserAttributeField, config *SchemaConfig) []AttributeDefinition {
	var definitions []AttributeDefinition
	
	for i, field := range attributeFields {
		if field.LDAPAttribute == "" {
			continue // Skip fields without LDAP mapping
		}

		attrDef := AttributeDefinition{
			Name:        field.LDAPAttribute,
			OID:         fmt.Sprintf("%s.2.%d", config.BaseOID, config.AttributeOIDStart+i),
			Description: fmt.Sprintf("Custom attribute: %s", field.DisplayName),
			Syntax:      GetSyntax(field.Type),
			Equality:    GetMatchRule(field.Type),
			SingleValue: true, // Most custom attributes are single-valued
		}

		// Add string-specific rules for text fields
		if field.Type == "text" {
			attrDef.Ordering = "caseIgnoreOrderingMatch"
			attrDef.Substr = "caseIgnoreSubstringsMatch"
		}

		definitions = append(definitions, attrDef)
	}

	return definitions
}

// ensureUniqueIDAttribute ensures the uniqueID attribute exists for groups
func (c *Client) ensureUniqueIDAttribute(ldapConn *ldap.Conn, schemaDN string, schemaConfig *SchemaConfig) error {
	// Check if uniqueID attribute already exists
	exists, err := c.AttributeExists(ldapConn, "uniqueID")
	if err != nil {
		Log.WithFields(logrus.Fields{
			"attribute": "uniqueID",
			"error":    err.Error(),
		}).Warn("Failed to check if uniqueID attribute exists, attempting to create anyway")
	} else if exists {
		Log.WithFields(logrus.Fields{
			"attribute": "uniqueID",
		}).Debug("uniqueID attribute already exists, skipping creation")
		return nil
	}

	// Create uniqueID attribute type definition
	oid := fmt.Sprintf("%s.2.107", schemaConfig.BaseOID)
	attributeTypeDef := fmt.Sprintf("( %s NAME 'uniqueID' DESC 'Unique identifier for groups' SYNTAX '1.3.6.1.4.1.1466.115.121.1.15' EQUALITY caseIgnoreMatch SINGLE-VALUE )", oid)

	// Apply the modification
	modifyRequest := ldap.NewModifyRequest(schemaDN, nil)
	modifyRequest.Add("olcAttributetypes", []string{attributeTypeDef})

	Log.WithFields(logrus.Fields{
		"attribute": "uniqueID",
		"oid":      oid,
	}).Debug("Adding uniqueID attribute type")

	if err := ldapConn.Modify(modifyRequest); err != nil {
		return fmt.Errorf("failed to add uniqueID attribute type: %w", err)
	}

	Log.Info("✅ Successfully created uniqueID attribute for groups")
	return nil
}