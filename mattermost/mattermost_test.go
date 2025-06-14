// Package mattermost tests for the mattermost client functionality
package mattermost

import (
	"context"
	"strings"
	"testing"

	"github.com/mattermost/mattermost/server/public/model"
)




// TestNewClient tests the client initialization using a table-driven approach
func TestNewClient(t *testing.T) {
	// Get the site URL from environment variables
	siteURL, err := getSiteURLFromEnv()
	if err != nil {
		t.Fatalf("Failed to get siteURL from environment: %v", err)
	}

	// Define test cases
	testCases := []struct {
		name          string
		serverURL     string
		adminUser     string
		adminPass     string
		teamName      string
		configPath    string
		expectedError bool
	}{
		{
			name:          "Valid client with admin credentials from environment",
			serverURL:     siteURL,
			adminUser:     getEnvVariable("MM_Username", "systemadmin"),
			adminPass:     getEnvVariable("MM_Password", "Password123!"),
			teamName:      "test-team",
			configPath:    "",
			expectedError: false,
		},
		{
			name:          "Valid client with custom team name",
			serverURL:     siteURL,
			adminUser:     getEnvVariable("MM_Username", "systemadmin"),
			adminPass:     getEnvVariable("MM_Password", "Password123!"),
			teamName:      "custom-team",
			configPath:    "",
			expectedError: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create a new client with test case parameters
			client := NewClient(
				tc.serverURL,
				tc.adminUser,
				tc.adminPass,
				tc.teamName,
				tc.configPath,
			)

			// Verify the client was initialized correctly
			if client.API == nil {
				t.Fatal("API client was not initialized")
			}

			if client.ServerURL != tc.serverURL {
				t.Errorf("Expected server URL to be %s, got %s", tc.serverURL, client.ServerURL)
			}

			if client.AdminUser != tc.adminUser {
				t.Errorf("Expected admin user to be %s, got %s", tc.adminUser, client.AdminUser)
			}

			if client.AdminPass != tc.adminPass {
				t.Errorf("Expected admin password to be %s, got %s", tc.adminPass, client.AdminPass)
			}

			if client.TeamName != tc.teamName {
				t.Errorf("Expected team name to be %s, got %s", tc.teamName, client.TeamName)
			}

			// Command URLs are read from config
		})
	}
}

// TestConnectionToServer tests the connection to the Mattermost server
func TestConnectionToServer(t *testing.T) {
	client := setupTestClient(t)

	// Test GetPing directly
	_, resp, err := client.API.GetPing(context.Background())
	if err != nil {
		t.Errorf("Failed to ping server: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("Expected status code 200, got %d", resp.StatusCode)
	}
}

// TestWaitForStart tests the WaitForStart function
func TestWaitForStart(t *testing.T) {
	// This test is mostly redundant as setupTestClient already calls WaitForStart
	// but we'll keep it simpler to test just this function
	siteURL, err := getSiteURLFromEnv()
	if err != nil {
		t.Fatalf("Failed to get siteURL from environment: %v", err)
	}

	// Create a client with admin credentials from environment (using root.go defaults)
	adminUsername := getEnvVariable("MM_Username", "systemadmin")
	adminPassword := getEnvVariable("MM_Password", "Password123!")

	client := NewClient(
		siteURL,
		adminUsername,
		adminPassword,
		"test-team",
		"", // No config path
	)

	// Test just the WaitForStart function
	err = client.WaitForStart()
	if err != nil {
		t.Errorf("Expected no error from WaitForStart, but got: %v", err)
	}
}

// TestUserManagement tests user creation and management functions
func TestUserManagement(t *testing.T) {
	client := setupTestClient(t)

	// Table-driven tests for user existence and properties
	testCases := []struct {
		name          string
		username      string
		shouldExist   bool
		shouldBeAdmin bool
	}{
		{
			name:          "Admin user from environment exists and has admin role",
			username:      getEnvVariable("MM_Username", "sysadmin"),
			shouldExist:   true,
			shouldBeAdmin: true,
		},
		{
			name:          "Non-existent user should not exist",
			username:      "nonexistentuser123456789",
			shouldExist:   false,
			shouldBeAdmin: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			user, resp, err := client.API.GetUserByUsername(context.Background(), tc.username, "")
			
			exists := (err == nil && resp.StatusCode == 200)

			if tc.shouldExist != exists {
				t.Errorf("Expected user %s existence to be %v, but got %v",
					tc.username, tc.shouldExist, exists)
			}

			if exists && tc.shouldBeAdmin {
				if !strings.Contains(user.Roles, "system_admin") {
					t.Errorf("Expected user %s to have system_admin role, roles: %s",
						tc.username, user.Roles)
				}
			}
		})
	}
}

// TestTeamManagement tests team creation and management
func TestTeamManagement(t *testing.T) {
	client := setupTestClient(t)

	// Test team listing
	teams, resp, err := client.API.GetAllTeams(context.Background(), "", 0, 100)
	if err != nil {
		t.Fatalf("Failed to get teams: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("Expected status code 200, got %d", resp.StatusCode)
	}

	// Create a map of team names for quick lookup
	teamMap := make(map[string]*model.Team)
	for _, team := range teams {
		teamMap[team.Name] = team
	}

	// Find a team that exists to test against
	existingTeamName := ""
	for _, team := range teams {
		existingTeamName = team.Name
		break
	}

	// Table-driven tests for teams
	testCases := []struct {
		name         string
		teamName     string
		expectExists bool
	}{
		{
			name:         "Existing team should exist",
			teamName:     existingTeamName,
			expectExists: len(teams) > 0, // Only expect true if we have teams
		},
		{
			name:         "Non-existent team should not exist",
			teamName:     "nonexistentteam123456789",
			expectExists: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, exists := teamMap[tc.teamName]
			if tc.expectExists != exists {
				t.Errorf("Expected team %s existence to be %v, but got %v",
					tc.teamName, tc.expectExists, exists)
			}
		})
	}
}

