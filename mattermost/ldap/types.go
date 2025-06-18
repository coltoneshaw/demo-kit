package ldap

// LDAPConfig represents LDAP connection configuration
type LDAPConfig struct {
	URL            string `json:"url"`             // LDAP server URL (e.g., ldap://localhost:10389)
	BindDN         string `json:"bind_dn"`         // Admin bind DN (e.g., cn=admin,dc=planetexpress,dc=com)
	BindPassword   string `json:"bind_password"`   // Admin password
	BaseDN         string `json:"base_dn"`         // Base DN (e.g., dc=planetexpress,dc=com)
	SchemaBindDN   string `json:"schema_bind_dn"`  // Schema admin bind DN (e.g., cn=admin,cn=config)
	SchemaPassword string `json:"schema_password"` // Schema admin password
}

// SchemaConfig represents configuration for LDAP schema extensions
type SchemaConfig struct {
	BaseOID            string // Base OID for custom attributes (e.g., "1.3.6.1.4.1.99999")
	ObjectClassName    string // Name of the auxiliary object class
	ObjectClassOID     string // OID for the auxiliary object class
	AttributeOIDStart  int    // Starting number for attribute OIDs
}

// AttributeDefinition represents an LDAP attribute type definition
type AttributeDefinition struct {
	Name        string
	OID         string
	Description string
	Syntax      string
	Equality    string
	Ordering    string
	Substr      string
	SingleValue bool
}

// UserAttributeField represents a custom user attribute field configuration
type UserAttributeField struct {
	Name          string // Field name in Mattermost
	DisplayName   string // Human-readable display name
	Type          string // Field type (text, number, select, boolean)
	LDAPAttribute string // LDAP attribute name mapping
	Required      bool   // Whether the field is required
}

// NewUserAttributeField creates a new UserAttributeField with the given parameters
func NewUserAttributeField(name, displayName, fieldType, ldapAttribute string, required bool) UserAttributeField {
	return UserAttributeField{
		Name:          name,
		DisplayName:   displayName,
		Type:          fieldType,
		LDAPAttribute: ldapAttribute,
		Required:      required,
	}
}



// DefaultSchemaConfig returns a default schema configuration
func DefaultSchemaConfig() *SchemaConfig {
	return &SchemaConfig{
		BaseOID:           "1.3.6.1.4.1.99999", // Private enterprise OID space
		ObjectClassName:   "mattermostCustomAttributes",
		ObjectClassOID:    "1.3.6.1.4.1.99999.1.1",
		AttributeOIDStart: 100,
	}
}