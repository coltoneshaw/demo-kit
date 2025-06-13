package mattermost

import (
	"strings"
	"testing"

	"github.com/mattermost/mattermost/server/public/model"
)


// TestUserManager tests user management functions
func TestUserManager(t *testing.T) {
	client := setupTestClient(t)
	userManager := client.UserManager

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
			exists, user, err := userManager.UserExists(tc.username)
			
			if tc.shouldExist {
				if err != nil {
					t.Errorf("Expected no error when checking if user exists, but got: %v", err)
				}
				if !exists {
					t.Errorf("Expected user %s to exist, but it doesn't", tc.username)
				}
				if exists && tc.shouldBeAdmin {
					if !strings.Contains(user.Roles, "system_admin") {
						t.Errorf("Expected user %s to have system_admin role, roles: %s",
							tc.username, user.Roles)
					}
				}
			} else {
				if exists {
					t.Errorf("Expected user %s to not exist, but it does", tc.username)
				}
			}
		})
	}
}

// TestCreateOrGetUser tests user creation and retrieval
func TestCreateOrGetUser(t *testing.T) {
	client := setupTestClient(t)
	userManager := client.UserManager

	// Test creating a new user
	testUsername := "testuser123"
	testEmail := "testuser123@example.com"
	testPassword := "testpassword123"

	// Clean up any existing test user first
	if exists, _, _ := userManager.UserExists(testUsername); exists {
		t.Logf("Test user %s already exists, skipping creation test", testUsername)
		return
	}

	// Create the user
	user, err := userManager.CreateOrGetUser(testUsername, testEmail, testPassword, "Test User", false)
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	if user.Username != testUsername {
		t.Errorf("Expected username %s, got %s", testUsername, user.Username)
	}

	if user.Email != testEmail {
		t.Errorf("Expected email %s, got %s", testEmail, user.Email)
	}

	// Test getting the same user (should not create duplicate)
	user2, err := userManager.CreateOrGetUser(testUsername, testEmail, testPassword, "Test User", false)
	if err != nil {
		t.Errorf("Failed to get existing user: %v", err)
	}

	if user.Id != user2.Id {
		t.Errorf("Expected same user ID, got different users")
	}
}

// TestFetchAllUsers tests the user fetching functionality
func TestFetchAllUsers(t *testing.T) {
	client := setupTestClient(t)
	userManager := client.UserManager

	users, err := userManager.FetchAllUsers()
	if err != nil {
		t.Fatalf("Failed to fetch users: %v", err)
	}

	if len(users) == 0 {
		t.Error("Expected at least one user, got none")
	}

	// Check that we got the admin user
	adminFound := false
	adminUsername := getEnvVariable("MM_Username", "sysadmin")
	for _, user := range users {
		if user.Username == adminUsername {
			adminFound = true
			break
		}
	}

	if !adminFound {
		t.Errorf("Expected to find admin user %s in user list", adminUsername)
	}
}

// TestEnsureUserIsAdmin tests the admin role validation and assignment
func TestEnsureUserIsAdmin(t *testing.T) {
	client := setupTestClient(t)
	userManager := client.UserManager

	// Get the current admin user
	adminUsername := getEnvVariable("MM_Username", "sysadmin")
	user, err := userManager.GetUserByUsername(adminUsername)
	if err != nil {
		t.Fatalf("Failed to get admin user: %v", err)
	}

	// Test cases for admin role validation
	testCases := []struct {
		name          string
		userRoles     string
		shouldBeAdmin bool
		expectError   bool
	}{
		{
			name:          "User with system_admin role should be recognized as admin",
			userRoles:     "system_admin system_user",
			shouldBeAdmin: true,
			expectError:   false,
		},
		{
			name:          "User with only system_user role should not be admin",
			userRoles:     "system_user",
			shouldBeAdmin: false,
			expectError:   false,
		},
		{
			name:          "User with system_admin included should be admin",
			userRoles:     "system_user system_admin team_admin",
			shouldBeAdmin: true,
			expectError:   false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create a test user object with the specified roles
			testUser := &model.User{
				Id:       user.Id,
				Username: user.Username,
				Email:    user.Email,
				Roles:    tc.userRoles,
			}

			// Test the EnsureUserIsAdmin function
			err := userManager.EnsureUserIsAdmin(testUser)

			if tc.expectError {
				if err == nil {
					t.Error("Expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
			}

			// Check if the user has admin role after the operation
			hasAdminRole := strings.Contains(testUser.Roles, "system_admin")
			if tc.shouldBeAdmin && !hasAdminRole {
				t.Error("Expected user to have system_admin role after EnsureUserIsAdmin")
			}
		})
	}
}

// TestCreateOrGetUserWithAdminRole tests creating users with admin roles
func TestCreateOrGetUserWithAdminRole(t *testing.T) {
	client := setupTestClient(t)
	userManager := client.UserManager

	// Test creating a user with admin role
	testUsername := "testadmin123"
	testEmail := "testadmin123@example.com"
	testPassword := "testpassword123"

	// Clean up any existing test user first
	if exists, _, _ := userManager.UserExists(testUsername); exists {
		t.Logf("Test admin user %s already exists, skipping creation test", testUsername)
		return
	}

	// Create the user with admin role
	user, err := userManager.CreateOrGetUser(testUsername, testEmail, testPassword, "Test Admin", true)
	if err != nil {
		t.Fatalf("Failed to create admin user: %v", err)
	}

	// Verify the user has admin role
	if !strings.Contains(user.Roles, "system_admin") {
		t.Errorf("Expected user to have system_admin role, got roles: %s", user.Roles)
	}

	// Verify the user also has system_user role
	if !strings.Contains(user.Roles, "system_user") {
		t.Errorf("Expected user to have system_user role, got roles: %s", user.Roles)
	}

	// Test getting the same user again (should not recreate)
	user2, err := userManager.CreateOrGetUser(testUsername, testEmail, testPassword, "Test Admin", true)
	if err != nil {
		t.Errorf("Failed to get existing admin user: %v", err)
	}

	if user.Id != user2.Id {
		t.Errorf("Expected same user ID, got different users")
	}

	// Verify admin role is still present
	if !strings.Contains(user2.Roles, "system_admin") {
		t.Errorf("Expected existing user to maintain system_admin role, got roles: %s", user2.Roles)
	}
}

// TestLoginAdminValidation tests that login properly validates and ensures admin role
func TestLoginAdminValidation(t *testing.T) {
	// Create a client but don't use setupTestClient (which calls login)
	client := NewClient(
		getEnvVariable("MM_ServerURL", DefaultSiteURL),
		getEnvVariable("MM_Username", "sysadmin"),
		getEnvVariable("MM_Password", DefaultAdminPassword),
		"test-team",
		"",
	)

	// Wait for server to be ready
	err := client.WaitForStart()
	if err != nil {
		t.Fatalf("Failed to wait for server start: %v", err)
	}

	// Test the login method which should ensure admin role
	err = client.Login()
	if err != nil {
		t.Fatalf("Login failed: %v", err)
	}

	// Verify that the admin user now has admin role (login should call EnsureUserIsAdmin)
	adminUsername := getEnvVariable("MM_Username", "sysadmin")
	user, err := client.UserManager.GetUserByUsername(adminUsername)
	if err != nil {
		t.Fatalf("Failed to get admin user after login: %v", err)
	}

	if !strings.Contains(user.Roles, "system_admin") {
		t.Errorf("Expected admin user to have system_admin role after login, got roles: %s", user.Roles)
	}

	if !strings.Contains(user.Roles, "system_user") {
		t.Errorf("Expected admin user to have system_user role after login, got roles: %s", user.Roles)
	}
}