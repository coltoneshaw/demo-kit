package mattermost

import (
	"context"
	"fmt"
	"strings"

	"github.com/sirupsen/logrus"
)

// Login authenticates with the Mattermost server
func (c *Client) Login() error {
	user, resp, err := c.API.Login(context.Background(), c.AdminUser, c.AdminPass)
	if err != nil {
		return handleAPIError(fmt.Sprintf("login failed for user '%s' with password '%s'", c.AdminUser, c.AdminPass), err, resp)
	}

	// Ensure the logged-in user has admin privileges
	if !strings.Contains(user.Roles, "system_admin") {
		// Use UpdateUserRoles API to directly assign system_admin role
		_, err := c.API.UpdateUserRoles(context.Background(), user.Id, "system_admin system_user")
		if err != nil {
			return fmt.Errorf("failed to assign system_admin role to user '%s': %w", c.AdminUser, err)
		}
		Log.WithFields(logrus.Fields{"user_name": c.AdminUser}).Info("✅ Assigned system_admin role to user")
	}

	return nil
}

// CheckLicense verifies that the Mattermost server has a valid license
func (c *Client) CheckLicense() error {
	// Try to get license information to verify it's valid
	license, resp, err := c.API.GetOldClientLicense(context.Background(), "")
	if err != nil || (resp != nil && resp.StatusCode != 200) {
		return handleAPIError("failed to get license", err, resp)
	}

	if license == nil {
		return fmt.Errorf("❌ No valid license found on the server")
	}

	// Check if the server is licensed
	isLicensed, exists := license["IsLicensed"]
	if !exists || isLicensed != "true" {
		return fmt.Errorf("❌ Mattermost server is not licensed. This setup tool requires a licensed Mattermost Enterprise server (IsLicensed: %s)", isLicensed)
	}

	// Get license ID for confirmation
	licenseId, hasId := license["Id"]
	if hasId {
		Log.WithFields(logrus.Fields{"license_id": licenseId}).Info("✅ Server is licensed")
	} else {
		Log.Info("✅ Server is licensed")
	}

	return nil
}

// CheckDeletionSettings verifies that the server has deletion APIs enabled
func (c *Client) CheckDeletionSettings() error {
	config, resp, err := c.API.GetConfig(context.Background())
	if err != nil {
		return handleAPIError("failed to get server config", err, resp)
	}

	// Check EnableAPIUserDeletion
	if config.ServiceSettings.EnableAPIUserDeletion == nil || !*config.ServiceSettings.EnableAPIUserDeletion {
		return fmt.Errorf("ServiceSettings.EnableAPIUserDeletion is not enabled. Please enable it in the server configuration to use the reset command")
	}

	// Check EnableAPITeamDeletion
	if config.ServiceSettings.EnableAPITeamDeletion == nil || !*config.ServiceSettings.EnableAPITeamDeletion {
		return fmt.Errorf("ServiceSettings.EnableAPITeamDeletion is not enabled. Please enable it in the server configuration to use the reset command")
	}

	Log.Info("✅ API deletion settings are enabled")
	return nil
}