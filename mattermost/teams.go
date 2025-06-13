package mattermost

import (
	"context"
	"fmt"

	"github.com/mattermost/mattermost/server/public/model"
)

// TeamManager handles all team-related operations
type TeamManager struct {
	client      *Client
	cachedTeams []*model.Team
}

// NewTeamManager creates a new TeamManager instance
func NewTeamManager(client *Client) *TeamManager {
	return &TeamManager{
		client:      client,
		cachedTeams: nil,
	}
}

// GetAllTeams retrieves all teams from the server
func (tm *TeamManager) GetAllTeams() ([]*model.Team, error) {
	teams, resp, err := tm.client.API.GetAllTeams(context.Background(), "", 0, 100)
	if err != nil {
		return nil, handleAPIError("failed to get teams", err, resp)
	}
	fmt.Printf("Fetched %d teams from server\n", len(teams))
	return teams, nil
}

// refreshTeamsCache fetches all teams and updates the cache
func (tm *TeamManager) refreshTeamsCache() error {
	teams, err := tm.GetAllTeams()
	if err != nil {
		return err
	}

	tm.cachedTeams = teams
	return nil
}

// ensureTeamsCached fetches teams if cache is empty
func (tm *TeamManager) ensureTeamsCached() error {
	if tm.cachedTeams != nil {
		return nil
	}
	return tm.refreshTeamsCache()
}

// findTeamInCache looks for a team by name in the cached teams list
func (tm *TeamManager) findTeamInCache(teamName string) *model.Team {
	for _, team := range tm.cachedTeams {
		if team.Name == teamName {
			return team
		}
	}
	return nil
}

// GetTeamByName retrieves a team by name from cache
func (tm *TeamManager) GetTeamByName(teamName string) (*model.Team, error) {
	// Ensure cache is populated
	if err := tm.ensureTeamsCached(); err != nil {
		return nil, err
	}

	// Search in cache
	team := tm.findTeamInCache(teamName)
	if team != nil {
		return team, nil
	}

	return nil, nil
}

// CreateTeam creates a new team
func (tm *TeamManager) CreateTeam(teamConfig TeamConfig) (*model.Team, error) {
	fmt.Printf("Creating '%s' team...\n", teamConfig.Name)

	// Default to Open type if not specified
	if teamConfig.Type == "" {
		return nil, fmt.Errorf("team type must be specified")
	}

	newTeam := &model.Team{
		Name:        teamConfig.Name,
		DisplayName: teamConfig.DisplayName,
		Description: teamConfig.Description,
		Type:        teamConfig.Type,
	}

	createdTeam, createResp, err := tm.client.API.CreateTeam(context.Background(), newTeam)
	if err != nil {
		return nil, handleAPIError("failed to create team", err, createResp)
	}

	// Refresh the cache after creating a new team
	if err := tm.refreshTeamsCache(); err != nil {
		fmt.Printf("⚠️ Warning: Failed to refresh teams cache after creation: %v\n", err)
	}

	fmt.Printf("✅ Successfully created team '%s' (ID: %s)\n", createdTeam.Name, createdTeam.Id)
	return createdTeam, nil
}

// CreateOrGetTeam creates a new team or returns an existing one
func (tm *TeamManager) CreateOrGetTeam(teamConfig TeamConfig) (*model.Team, error) {
	// Check if team already exists
	team, err := tm.GetTeamByName(teamConfig.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to check if team exists: %w", err)
	}

	if team != nil {
		fmt.Printf("Team '%s' already exists\n", teamConfig.Name)
		return team, nil
	}

	// Create the team
	return tm.CreateTeam(teamConfig)
}

// IsUserTeamMember checks if a user is a member of a team
func (tm *TeamManager) IsUserTeamMember(teamID, userID string) (bool, error) {
	_, resp, err := tm.client.API.GetTeamMember(context.Background(), teamID, userID, "")
	if err != nil {
		if resp != nil && resp.StatusCode == 404 {
			return false, nil
		}
		return false, handleAPIError("failed to check team membership", err, resp)
	}
	return true, nil
}

// AddUserToTeam adds a user to a team
func (tm *TeamManager) AddUserToTeam(user *model.User, team *model.Team) error {
	// Check if user is already a team member
	isMember, err := tm.IsUserTeamMember(team.Id, user.Id)
	if err != nil {
		return fmt.Errorf("failed to check team membership: %w", err)
	}

	if isMember {
		fmt.Printf("User '%s' is already a member of team '%s'\n", user.Username, team.Name)
		return nil
	}

	_, teamResp, err := tm.client.API.AddTeamMember(context.Background(), team.Id, user.Id)
	if err != nil {
		// Check if the error is because the user is already a member
		if teamResp != nil && teamResp.StatusCode == 400 {
			fmt.Printf("User '%s' is already a member of team '%s'\n", user.Username, team.Name)
			return nil
		}
		return fmt.Errorf("failed to add user '%s' to team '%s': %w", user.Username, team.Name, err)
	}

	fmt.Printf("✅ Added user '%s' to team '%s'\n", user.Username, team.Name)
	return nil
}

// CreateTeamsFromConfig creates teams from the configuration
func (tm *TeamManager) CreateTeamsFromConfig(config *Config) (map[string]*model.Team, error) {
	teamMap := make(map[string]*model.Team)

	if config == nil || len(config.Teams) == 0 {
		return nil, fmt.Errorf("no teams defined in config")
	}

	fmt.Println("Creating teams from configuration file...")

	// Fetch all teams once at the beginning for efficient checking
	if err := tm.ensureTeamsCached(); err != nil {
		return nil, err
	}

	// Process each team from the config
	for _, teamConfig := range config.Teams {
		team, err := tm.CreateOrGetTeam(teamConfig)
		if err != nil {
			fmt.Printf("❌ Error with team '%s': %v\n", teamConfig.Name, err)
			continue
		}

		teamMap[team.Name] = team
	}

	return teamMap, nil
}

// AddUsersToTeams adds users to teams according to the configuration
func (tm *TeamManager) AddUsersToTeams(teamMap map[string]*model.Team, config *Config, userManager *UserManager) error {
	if config == nil || len(config.Users) == 0 {
		fmt.Println("No user configuration found. Adding only default admin to default team...")

		// Get default team
		team, exists := teamMap[tm.client.TeamName]
		if !exists {
			return fmt.Errorf("default team '%s' not found", tm.client.TeamName)
		}

		// Get default admin user
		user, err := userManager.GetUserByUsername(DefaultAdminUsername)
		if err != nil {
			fmt.Printf("❌ Default admin user '%s' not found: %v\n", DefaultAdminUsername, err)
			return nil
		}

		// Add admin user to team
		if err := tm.AddUserToTeam(user, team); err != nil {
			fmt.Printf("❌ %v\n", err)
		}

		return nil
	}

	fmt.Println("Adding users to teams from configuration...")

	// Loop through each user in config
	for _, userConfig := range config.Users {
		// Get the user
		user, err := userManager.GetUserByUsername(userConfig.Username)
		if err != nil {
			fmt.Printf("❌ User '%s' not found, can't add to teams\n", userConfig.Username)
			return err
		}

		// Add user to each of their teams
		for _, teamName := range userConfig.Teams {
			// Skip if team doesn't exist
			team, exists := teamMap[teamName]
			if !exists {
				fmt.Printf("❌ Team '%s' not found, can't add user '%s'\n", teamName, userConfig.Username)
				return err
			}

			// Add user to team with error handling
			if err := tm.AddUserToTeam(user, team); err != nil {
				fmt.Printf("❌ %v\n", err)
				return err
			}
		}
	}

	return nil
}
