package ldap

import (
	"github.com/coltoneshaw/demokit/mattermost/logger"
	"github.com/go-ldap/ldap/v3"
	"github.com/sirupsen/logrus"
)

// Log is an alias to the global logger
var Log = logger.Log

// Client represents an LDAP client for schema operations
type Client struct {
	config *LDAPConfig
}

// NewClient creates a new LDAP client with the given configuration
func NewClient(config *LDAPConfig) *Client {
	return &Client{
		config: config,
	}
}

// SchemaManager interface defines schema management operations
type SchemaManager interface {
	SetupSchema(ldapConn *ldap.Conn, attributeFields []UserAttributeField, schemaConfig *SchemaConfig, ldapConfig *LDAPConfig) error
	CreateAttributes(attributeFields []UserAttributeField, schemaConfig *SchemaConfig, ldapConfig *LDAPConfig) error
	CreateObjectClass(attributeFields []UserAttributeField, schemaConfig *SchemaConfig, ldapConfig *LDAPConfig) error
	AttributeExists(ldapConn *ldap.Conn, attributeName string) (bool, error)
	ObjectClassExists(ldapConn *ldap.Conn, objectClassName string) (bool, error)
}

// SetLogger allows overriding the logger if needed
func SetLogger(l *logrus.Logger) {
	Log = l
}
