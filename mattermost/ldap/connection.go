package ldap

import (
	"fmt"

	"github.com/go-ldap/ldap/v3"
	"github.com/sirupsen/logrus"
)

// ConnectToSchema creates connection using schema admin credentials
func (c *Client) ConnectToSchema(ldapConfig *LDAPConfig) (*ldap.Conn, error) {
	// Connect to LDAP server
	conn, err := ldap.DialURL(ldapConfig.URL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to LDAP server: %w", err)
	}

	// Bind as schema admin
	err = conn.Bind(ldapConfig.SchemaBindDN, ldapConfig.SchemaPassword)
	if err != nil {
		if closeErr := conn.Close(); closeErr != nil {
			Log.WithError(closeErr).Warn("Failed to close LDAP connection during error handling")
		}
		return nil, fmt.Errorf("failed to bind as schema admin: %w", err)
	}

	Log.WithFields(logrus.Fields{
		"schema_bind_dn": ldapConfig.SchemaBindDN,
		"url":           ldapConfig.URL,
	}).Debug("Connected to LDAP as schema admin")

	return conn, nil
}

// Connect creates a standard LDAP connection using directory admin credentials
func (c *Client) Connect() (*ldap.Conn, error) {
	// Connect to LDAP server
	conn, err := ldap.DialURL(c.config.URL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to LDAP server: %w", err)
	}

	// Bind as directory admin
	err = conn.Bind(c.config.BindDN, c.config.BindPassword)
	if err != nil {
		if closeErr := conn.Close(); closeErr != nil {
			Log.WithError(closeErr).Warn("Failed to close LDAP connection during error handling")
		}
		return nil, fmt.Errorf("failed to bind as directory admin: %w", err)
	}

	Log.WithFields(logrus.Fields{
		"bind_dn": c.config.BindDN,
		"url":    c.config.URL,
	}).Debug("Connected to LDAP as directory admin")

	return conn, nil
}