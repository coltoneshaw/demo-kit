package mattermost

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/sirupsen/logrus"
)

// Login authenticates with the Mattermost server
func (c *Client) Login() error {
	user, resp, err := c.API.Login(context.Background(), c.AdminUser, c.AdminPass)
	if err != nil {
		// If login failed and we're in local environment, try to create the default user
		if c.Config != nil && c.Config.Environment == "local" && resp != nil && resp.StatusCode == 401 {
			Log.WithFields(logrus.Fields{
				"username": c.AdminUser,
			}).Info("Default user not found in local environment, attempting to create...")

			// Try to create the default user
			if createErr := c.CreateDefaultUser(); createErr != nil {
				// If creation also failed, return the original login error
				return handleAPIError(fmt.Sprintf("login failed for user '%s' with password '%s', and failed to create user", c.AdminUser, c.AdminPass), err, resp)
			}

			// Try to login again after creating the user
			user, resp, err = c.API.Login(context.Background(), c.AdminUser, c.AdminPass)
			if err != nil {
				return handleAPIError(fmt.Sprintf("login failed even after creating user '%s'", c.AdminUser), err, resp)
			}
		} else {
			return handleAPIError(fmt.Sprintf("login failed for user '%s' with password '%s'", c.AdminUser, c.AdminPass), err, resp)
		}
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

// CreateDefaultUser creates the default sysadmin user using docker exec command
// This is only used in local development environments when the default user doesn't exist
func (c *Client) CreateDefaultUser() error {
	// Execute docker command to create the user (without -it for non-interactive execution)
	cmd := exec.Command("docker", "exec", "mattermost", "mmctl", "user", "create",
		"--email", "user@example.com",
		"--username", c.AdminUser,
		"--password", c.AdminPass,
		"--system-admin",
		"--email-verified",
		"--local")

	output, err := cmd.CombinedOutput()
	if err != nil {
		// Check if the error is because the user already exists
		outputStr := string(output)
		if strings.Contains(outputStr, "already exists") || strings.Contains(outputStr, "duplicate key") {
			Log.WithFields(logrus.Fields{
				"username": c.AdminUser,
			}).Info("User already exists, proceeding with login")
			return nil
		}
		return fmt.Errorf("failed to create default user via docker: %w\nOutput: %s", err, outputStr)
	}

	Log.WithFields(logrus.Fields{
		"username": c.AdminUser,
	}).Info("✅ Created default sysadmin user via docker")

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