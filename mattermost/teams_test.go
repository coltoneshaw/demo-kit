package mattermost

import (
	"testing"

	"github.com/mattermost/mattermost/server/public/model"
)

// TestTeamManager tests team management functions
func TestTeamManager(t *testing.T) {
	client := setupTestClient(t)
	teamManager := client.TeamManager

	// Test team listing
	teams, err := teamManager.GetAllTeams()
	if err != nil {
		t.Fatalf("Failed to get teams: %v", err)
	}

	if len(teams) == 0 {
		t.Error("Expected at least one team, got none")
	}

	// Find a team that exists to test against
	var existingTeam *model.Team
	for _, team := range teams {
		existingTeam = team
		break
	}

	// Table-driven tests for teams
	testCases := []struct {
		name        string
		teamName    string
		shouldExist bool
	}{
		{
			name:        "Existing team should be found",
			teamName:    existingTeam.Name,
			shouldExist: true,
		},
		{
			name:        "Non-existent team should return nil",
			teamName:    "nonexistentteam123456789",
			shouldExist: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			team, err := teamManager.GetTeamByName(tc.teamName)
			
			if tc.shouldExist {
				if err != nil {
					t.Errorf("Expected no error when getting team, but got: %v", err)
				}
				if team == nil {
					t.Errorf("Expected team %s to be found, but got nil", tc.teamName)
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error for non-existent team, but got: %v", err)
				}
				if team != nil {
					t.Errorf("Expected team %s to be nil, but got team object", tc.teamName)
				}
			}
		})
	}
}

// TestCreateOrGetTeam tests team creation and retrieval
func TestCreateOrGetTeam(t *testing.T) {
	client := setupTestClient(t)
	teamManager := client.TeamManager

	// Test creating a new team
	testTeamName := "test-team-123"
	testDisplayName := "Test Team 123"
	testDescription := "A test team for unit testing"

	// Clean up any existing test team first
	existingTeam, err := teamManager.GetTeamByName(testTeamName)
	if err != nil {
		t.Fatalf("Error checking for existing team: %v", err)
	}
	if existingTeam != nil {
		t.Logf("Test team %s already exists, skipping creation test", testTeamName)
		return
	}

	// Create the team
	teamConfig := TeamConfig{
		Name:        testTeamName,
		DisplayName: testDisplayName,
		Description: testDescription,
		Type:        model.TeamOpen,
	}
	team, err := teamManager.CreateOrGetTeam(teamConfig)
	if err != nil {
		t.Fatalf("Failed to create team: %v", err)
	}

	if team.Name != testTeamName {
		t.Errorf("Expected team name %s, got %s", testTeamName, team.Name)
	}

	if team.DisplayName != testDisplayName {
		t.Errorf("Expected display name %s, got %s", testDisplayName, team.DisplayName)
	}

	if team.Description != testDescription {
		t.Errorf("Expected description %s, got %s", testDescription, team.Description)
	}

	// Test getting the same team (should not create duplicate)
	team2, err := teamManager.CreateOrGetTeam(teamConfig)
	if err != nil {
		t.Errorf("Failed to get existing team: %v", err)
	}

	if team.Id != team2.Id {
		t.Errorf("Expected same team ID, got different teams")
	}
}

// TestTeamMembership tests adding users to teams
func TestTeamMembership(t *testing.T) {
	client := setupTestClient(t)
	teamManager := client.TeamManager
	userManager := client.UserManager

	// Get an existing team
	teams, err := teamManager.GetAllTeams()
	if err != nil {
		t.Fatalf("Failed to get teams: %v", err)
	}

	if len(teams) == 0 {
		t.Skip("No teams available for membership testing")
	}

	team := teams[0]

	// Get the admin user
	adminUsername := getEnvVariable("MM_Username", "sysadmin")
	user, err := userManager.GetUserByUsername(adminUsername)
	if err != nil {
		t.Fatalf("Failed to get admin user: %v", err)
	}

	// Test adding user to team (should work or already be a member)
	err = teamManager.AddUserToTeam(user, team)
	if err != nil {
		t.Errorf("Failed to add user to team: %v", err)
	}

	// Test checking membership
	isMember, err := teamManager.IsUserTeamMember(team.Id, user.Id)
	if err != nil {
		t.Errorf("Failed to check team membership: %v", err)
	}

	if !isMember {
		t.Errorf("Expected user to be a team member, but they're not")
	}
}

// TestTeamCaching tests that team caching works efficiently
func TestTeamCaching(t *testing.T) {
	client := setupTestClient(t)
	teamManager := client.TeamManager

	// First call should fetch from server
	t.Log("First call - should fetch from server")
	teams1, err := teamManager.GetAllTeams()
	if err != nil {
		t.Fatalf("Failed to get teams: %v", err)
	}

	if len(teams1) == 0 {
		t.Error("Expected at least one team, got none")
	}

	// Subsequent calls through GetTeamByName should use cache
	t.Log("Testing GetTeamByName with caching")
	for _, team := range teams1 {
		cachedTeam, err := teamManager.GetTeamByName(team.Name)
		if err != nil {
			t.Errorf("Error getting team: %v", err)
		}
		if cachedTeam == nil {
			t.Errorf("Expected team %s to be found", team.Name)
		}
		if cachedTeam != nil && cachedTeam.Id != team.Id {
			t.Errorf("Cached team ID doesn't match original")
		}
	}

	// Test non-existent team
	nonExistentTeam, err := teamManager.GetTeamByName("nonexistent-team-12345")
	if err != nil {
		t.Errorf("Expected no error for non-existent team, but got: %v", err)
	}
	if nonExistentTeam != nil {
		t.Error("Expected non-existent team to be nil")
	}
}

// TestCreateTeamsFromConfigCaching tests efficient team creation with caching
func TestCreateTeamsFromConfigCaching(t *testing.T) {
	client := setupTestClient(t)
	teamManager := client.TeamManager

	// Create a mock config with multiple teams
	config := &Config{
		Teams: map[string]TeamConfig{
			"existing-team": {
				Name:        "existing-team",
				DisplayName: "Existing Team",
				Description: "A team that exists",
				Type:        "O",
			},
			"another-team": {
				Name:        "another-team", 
				DisplayName: "Another Team",
				Description: "Another team to check",
				Type:        "O",
			},
		},
	}

	// This should fetch teams once and then use cache for all subsequent checks
	t.Log("Creating teams from config - should cache efficiently")
	teamMap, err := teamManager.CreateTeamsFromConfig(config)
	if err != nil {
		t.Fatalf("Failed to create teams from config: %v", err)
	}

	if len(teamMap) == 0 {
		t.Error("Expected teams to be created/found")
	}

	// Verify the teams exist and caching works
	for teamName := range config.Teams {
		if _, exists := teamMap[teamName]; !exists {
			t.Errorf("Expected team %s to be in team map", teamName)
		}

		// This should use cache
		team, err := teamManager.GetTeamByName(teamName)
		if err != nil {
			t.Errorf("Error getting team: %v", err)
		}
		if team == nil {
			t.Errorf("Expected team %s to be found", teamName)
		}
	}
}

// TestCreateTeamCacheRefresh tests that cache is refreshed after team creation
func TestCreateTeamCacheRefresh(t *testing.T) {
	client := setupTestClient(t)
	teamManager := client.TeamManager

	testTeamName := "cache-refresh-test-team"
	
	// Clean up if exists
	existingTeam, err := teamManager.GetTeamByName(testTeamName)
	if err != nil {
		t.Fatalf("Error checking for existing team: %v", err)
	}
	if existingTeam != nil {
		t.Logf("Test team %s already exists, skipping creation test", testTeamName)
		return
	}

	t.Log("Creating a new team - should refresh cache")
	teamConfig := TeamConfig{
		Name:        testTeamName,
		DisplayName: "Cache Refresh Test",
		Description: "Testing cache refresh",
		Type:        "O",
	}
	team, err := teamManager.CreateTeam(teamConfig)
	if err != nil {
		t.Fatalf("Failed to create team: %v", err)
	}

	t.Log("Checking if new team exists in cache")
	cachedTeam, err := teamManager.GetTeamByName(testTeamName)
	if err != nil {
		t.Errorf("Error getting team: %v", err)
	}
	if cachedTeam == nil {
		t.Errorf("Expected new team to exist in cache after creation")
	}
	if cachedTeam != nil && cachedTeam.Id != team.Id {
		t.Errorf("Cached team ID doesn't match created team ID")
	}
}