package ldap

import (
	"fmt"
	"strings"

	"github.com/go-ldap/ldap/v3"
	"github.com/sirupsen/logrus"
)

// LDAPGroup represents an LDAP group with members and Mattermost configuration
type LDAPGroup struct {
	Name           string   // Group name (cn)
	UniqueID       string   // Unique identifier for the group
	Members        []string // Array of usernames (will be converted to DNs)
	DN             string   // Full distinguished name of the group
	AllowReference bool     // Whether the group can be mentioned with @groupname in Mattermost
}

// CreateGroup creates a new LDAP group with proper structure
func (c *Client) CreateGroup(ldapConn *ldap.Conn, group LDAPGroup, config *LDAPConfig) error {
	// Construct group DN
	groupDN := fmt.Sprintf("cn=%s,ou=groups,%s", group.Name, config.BaseDN)
	
	Log.WithFields(logrus.Fields{
		"group_name": group.Name,
		"unique_id":  group.UniqueID,
		"member_count": len(group.Members),
	}).Info("🏗️ Creating LDAP group")

	// Check if group already exists
	if exists, err := c.GroupExists(ldapConn, groupDN); err != nil {
		Log.WithFields(logrus.Fields{
			"group_name": group.Name,
			"error":     err.Error(),
		}).Warn("Failed to check if group exists, attempting to create anyway")
	} else if exists {
		Log.WithFields(logrus.Fields{
			"group_name": group.Name,
		}).Info("Group already exists, will sync membership")
		return c.SyncGroupMembership(ldapConn, group, config)
	}

	// Ensure groups OU exists
	if err := c.ensureGroupsOU(ldapConn, config); err != nil {
		return fmt.Errorf("failed to ensure groups OU: %w", err)
	}

	// Convert usernames to UIDs (full DNs)
	memberDNs := make([]string, len(group.Members))
	for i, username := range group.Members {
		memberDNs[i] = fmt.Sprintf("uid=%s,ou=people,%s", username, config.BaseDN)
	}

	// Create group entry
	addRequest := ldap.NewAddRequest(groupDN, nil)
	
	// Object classes for groups - using Group as specified by user
	objectClasses := []string{"Group", "top"}
	
	addRequest.Attribute("objectClass", objectClasses)
	addRequest.Attribute("cn", []string{group.Name})
	addRequest.Attribute("groupType", []string{"2147483650"})
	
	// Add members using member attribute (full DNs)
	if len(group.Members) > 0 {
		addRequest.Attribute("member", memberDNs)
	}

	// Execute the add request
	if err := ldapConn.Add(addRequest); err != nil {
		return fmt.Errorf("failed to create group %s: %w", group.Name, err)
	}

	Log.WithFields(logrus.Fields{
		"group_name": group.Name,
		"unique_id":  group.UniqueID,
		"members":    len(group.Members),
	}).Info("✅ Successfully created LDAP group")

	return nil
}

// SyncGroupMembership synchronizes group membership with desired state
func (c *Client) SyncGroupMembership(ldapConn *ldap.Conn, group LDAPGroup, config *LDAPConfig) error {
	groupDN := fmt.Sprintf("cn=%s,ou=groups,%s", group.Name, config.BaseDN)
	
	Log.WithFields(logrus.Fields{
		"group_name": group.Name,
		"desired_members": len(group.Members),
	}).Info("🔄 Syncing group membership")

	// Get current members
	currentMembers, err := c.GetGroupMembers(ldapConn, groupDN)
	if err != nil {
		return fmt.Errorf("failed to get current group members: %w", err)
	}

	// Convert desired usernames to UIDs (full DNs)
	desiredMemberDNs := make([]string, len(group.Members))
	for i, username := range group.Members {
		desiredMemberDNs[i] = fmt.Sprintf("uid=%s,ou=people,%s", username, config.BaseDN)
	}

	// Find members to add and remove
	toAdd, toRemove := c.calculateMembershipChanges(currentMembers, desiredMemberDNs)

	// Remove members that shouldn't be there
	if len(toRemove) > 0 {
		if err := c.RemoveGroupMembers(ldapConn, groupDN, toRemove); err != nil {
			return fmt.Errorf("failed to remove members: %w", err)
		}
	}

	// Add members that should be there
	if len(toAdd) > 0 {
		if err := c.AddGroupMembers(ldapConn, groupDN, toAdd); err != nil {
			return fmt.Errorf("failed to add members: %w", err)
		}
	}

	Log.WithFields(logrus.Fields{
		"group_name": group.Name,
		"added":      len(toAdd),
		"removed":    len(toRemove),
		"final_count": len(desiredMemberDNs),
	}).Info("✅ Group membership synchronized")

	return nil
}

// GetGroupMembers retrieves current members of a group
func (c *Client) GetGroupMembers(ldapConn *ldap.Conn, groupDN string) ([]string, error) {
	searchRequest := ldap.NewSearchRequest(
		groupDN,
		ldap.ScopeBaseObject,
		ldap.NeverDerefAliases,
		0, 0, false,
		"(objectClass=Group)",
		[]string{"member"},
		nil,
	)

	searchResult, err := ldapConn.Search(searchRequest)
	if err != nil {
		return nil, fmt.Errorf("failed to search for group: %w", err)
	}

	if len(searchResult.Entries) == 0 {
		return []string{}, nil
	}

	members := searchResult.Entries[0].GetAttributeValues("member")
	return members, nil
}

// AddGroupMembers adds members to an LDAP group (Group uses member)
func (c *Client) AddGroupMembers(ldapConn *ldap.Conn, groupDN string, memberDNs []string) error {
	if len(memberDNs) == 0 {
		return nil
	}

	modifyRequest := ldap.NewModifyRequest(groupDN, nil)
	modifyRequest.Add("member", memberDNs)

	Log.WithFields(logrus.Fields{
		"group_dn": groupDN,
		"members":  memberDNs,
	}).Debug("Adding members to group")

	if err := ldapConn.Modify(modifyRequest); err != nil {
		return fmt.Errorf("failed to add members to group: %w", err)
	}

	return nil
}

// RemoveGroupMembers removes members from an LDAP group
func (c *Client) RemoveGroupMembers(ldapConn *ldap.Conn, groupDN string, memberDNs []string) error {
	if len(memberDNs) == 0 {
		return nil
	}

	modifyRequest := ldap.NewModifyRequest(groupDN, nil)
	modifyRequest.Delete("member", memberDNs)

	Log.WithFields(logrus.Fields{
		"group_dn": groupDN,
		"members":  memberDNs,
	}).Debug("Removing members from group")

	if err := ldapConn.Modify(modifyRequest); err != nil {
		return fmt.Errorf("failed to remove members from group: %w", err)
	}

	return nil
}

// GroupExists checks if a group exists in LDAP
func (c *Client) GroupExists(ldapConn *ldap.Conn, groupDN string) (bool, error) {
	searchRequest := ldap.NewSearchRequest(
		groupDN,
		ldap.ScopeBaseObject,
		ldap.NeverDerefAliases,
		0, 0, false,
		"(objectClass=Group)",
		[]string{"dn"},
		nil,
	)

	_, err := ldapConn.Search(searchRequest)
	if err != nil {
		if ldap.IsErrorWithCode(err, ldap.LDAPResultNoSuchObject) {
			return false, nil
		}
		return false, err
	}

	return true, nil
}

// ensureGroupsOU ensures the groups organizational unit exists
func (c *Client) ensureGroupsOU(ldapConn *ldap.Conn, config *LDAPConfig) error {
	ouDN := fmt.Sprintf("ou=groups,%s", config.BaseDN)
	
	// Check if groups OU exists
	searchRequest := ldap.NewSearchRequest(
		ouDN,
		ldap.ScopeBaseObject,
		ldap.NeverDerefAliases,
		0, 0, false,
		"(objectClass=organizationalUnit)",
		[]string{"dn"},
		nil,
	)

	_, err := ldapConn.Search(searchRequest)
	if err == nil {
		// OU already exists
		return nil
	}

	// Create groups OU
	Log.Info("📁 Creating groups organizational unit")
	addRequest := ldap.NewAddRequest(ouDN, nil)
	addRequest.Attribute("objectClass", []string{"organizationalUnit", "top"})
	addRequest.Attribute("ou", []string{"groups"})

	if err := ldapConn.Add(addRequest); err != nil {
		return fmt.Errorf("failed to create groups OU: %w", err)
	}

	Log.Info("✅ Created groups organizational unit")
	return nil
}

// calculateMembershipChanges determines what members to add and remove
func (c *Client) calculateMembershipChanges(current, desired []string) (toAdd, toRemove []string) {
	currentSet := make(map[string]bool)
	for _, member := range current {
		currentSet[member] = true
	}

	desiredSet := make(map[string]bool)
	for _, member := range desired {
		desiredSet[member] = true
	}

	// Find members to add (in desired but not in current)
	for member := range desiredSet {
		if !currentSet[member] {
			toAdd = append(toAdd, member)
		}
	}

	// Find members to remove (in current but not in desired)
	for member := range currentSet {
		if !desiredSet[member] {
			// Skip dummy members
			if !strings.Contains(member, "cn=dummy") {
				toRemove = append(toRemove, member)
			}
		}
	}

	return toAdd, toRemove
}