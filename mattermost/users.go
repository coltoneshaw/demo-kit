package mattermost

import (
	"context"
	"fmt"
	"strings"

	"github.com/mattermost/mattermost/server/public/model"
)

// UserManager handles all user-related operations
type UserManager struct {
	client      *Client
	cachedUsers []*model.User
	usersCached bool
}

// NewUserManager creates a new UserManager instance
func NewUserManager(client *Client) *UserManager {
	return &UserManager{
		client:      client,
		cachedUsers: nil,
		usersCached: false,
	}
}

// ensureUsersCached fetches all users if not already cached
func (um *UserManager) ensureUsersCached() error {
	if um.usersCached {
		return nil
	}

	users, err := um.FetchAllUsers()
	if err != nil {
		return err
	}

	um.cachedUsers = users
	um.usersCached = true
	return nil
}

// FetchAllUsers retrieves all users from the server in batches
func (um *UserManager) FetchAllUsers() ([]*model.User, error) {
	users := []*model.User{}
	totalFetched := 0
	page := 0

	// Fetch users in batches of 1000 until we get all users
	for {
		// Get a batch of users
		batch, resp, err := um.client.API.GetUsers(context.Background(), page, 1000, "")
		if err != nil {
			return nil, handleAPIError("failed to get users", err, resp)
		}

		users = append(users, batch...)
		totalFetched += len(batch)
		page++

		if len(batch) < 1000 {
			break // No more users to fetch
		}

		if totalFetched%1000 == 0 {
			fmt.Printf("Fetched %d users so far...\n", totalFetched)
		}
	}

	fmt.Printf("Total users fetched: %d\n", totalFetched)
	return users, nil
}

// findUserInCache looks for a user by username in the cached users list
func (um *UserManager) findUserInCache(username string) *model.User {
	for _, user := range um.cachedUsers {
		if user.Username == username {
			return user
		}
	}
	return nil
}

// GetUserByUsername retrieves a user by username, using cache first
func (um *UserManager) GetUserByUsername(username string) (*model.User, error) {
	// Try cache first if available
	if um.usersCached {
		if user := um.findUserInCache(username); user != nil {
			return user, nil
		}
	}

	// Fall back to API call if not in cache
	user, resp, err := um.client.API.GetUserByUsername(context.Background(), username, "")
	if err != nil {
		return nil, handleAPIError(fmt.Sprintf("failed to get user '%s'", username), err, resp)
	}

	// Add to cache if we have one
	if um.usersCached {
		um.cachedUsers = append(um.cachedUsers, user)
	}

	return user, nil
}

// UserExists checks if a user exists by username using cached user list
func (um *UserManager) UserExists(username string) (bool, *model.User, error) {
	// Ensure users are cached first
	if err := um.ensureUsersCached(); err != nil {
		return false, nil, fmt.Errorf("failed to cache users: %w", err)
	}

	user := um.findUserInCache(username)
	if user != nil {
		return true, user, nil
	}
	return false, nil, nil
}

// EnsureUserIsAdmin ensures a user has system_admin role
func (um *UserManager) EnsureUserIsAdmin(user *model.User) error {
	if !strings.Contains(user.Roles, "system_admin") {
		// Use UpdateUserRoles API to directly assign system_admin role
		_, err := um.client.API.UpdateUserRoles(context.Background(), user.Id, "system_admin system_user")
		if err != nil {
			fmt.Printf("❌ Failed to assign system_admin role to user '%s': %v\n", user.Username, err)
			return err
		}

		// Update the user object with the new roles
		user.Roles = "system_admin system_user"

		// Update cache if we have it
		if um.usersCached {
			for i, cachedUser := range um.cachedUsers {
				if cachedUser.Id == user.Id {
					um.cachedUsers[i].Roles = "system_admin system_user"
					break
				}
			}
		}

		fmt.Printf("✅ Successfully assigned system_admin role to user '%s'\n", user.Username)
	}
	return nil
}

// CreateUser creates a new user
func (um *UserManager) CreateUser(user *model.User) (*model.User, error) {
	fmt.Printf("Creating user '%s'...\n", user.Username)
	createdUser, resp, err := um.client.API.CreateUser(context.Background(), user)
	if err != nil {
		return nil, handleAPIError(fmt.Sprintf("failed to create user '%s'", user.Username), err, resp)
	}

	fmt.Printf("✅ Successfully created user '%s' (ID: %s)\n", createdUser.Username, createdUser.Id)
	return createdUser, nil
}

// CreateOrGetUser creates a new user or returns an existing one
func (um *UserManager) CreateOrGetUser(username, email, password, nickname string, isAdmin bool) (*model.User, error) {
	// Check if user already exists using cached list
	exists, user, err := um.UserExists(username)
	if err != nil {
		return nil, fmt.Errorf("failed to check if user exists: %w", err)
	}

	if exists {
		fmt.Printf("User '%s' already exists\n", username)

		// Ensure admin status if needed
		if isAdmin {
			if err := um.EnsureUserIsAdmin(user); err != nil {
				return user, err // Still return the user even if admin role update fails
			}
		}

		return user, nil
	}

	// Set roles if system admin
	roles := "system_user" // Always include system_user
	if isAdmin {
		roles = "system_admin system_user"
	}

	newUser := &model.User{
		Username: username,
		Email:    email,
		Password: password,
		Nickname: nickname,
		Roles:    roles,
	}

	// Create the user
	createdUser, err := um.CreateUser(newUser)
	if err != nil {
		// Check if error is about user already existing
		if strings.Contains(err.Error(), "already exists") {
			fmt.Printf("User '%s' was created by another process, fetching existing user\n", username)
			// Refresh cache and try to find the user
			um.usersCached = false
			return um.CreateOrGetUser(username, email, password, nickname, isAdmin)
		}
		return nil, err
	}

	// Add the newly created user to the cache
	if um.usersCached {
		um.cachedUsers = append(um.cachedUsers, createdUser)
	}

	return createdUser, nil
}

// CreateUsersFromConfig creates users from the configuration and assigns them to teams
func (um *UserManager) CreateUsersFromConfig(config *Config, teamManager *TeamManager) error {
	if config == nil || len(config.Users) == 0 {
		fmt.Println("No user configuration found. Using only default admin user.")
		return nil
	}

	fmt.Println("Creating users from configuration file...")

	// Fetch all users once at the beginning for efficient checking
	if err := um.ensureUsersCached(); err != nil {
		return fmt.Errorf("failed to fetch existing users: %w", err)
	}

	// Create teams first to ensure they exist before assigning users
	var teamMap map[string]*model.Team
	if len(config.Teams) > 0 {
		var err error
		teamMap, err = teamManager.CreateTeamsFromConfig(config)
		if err != nil {
			return fmt.Errorf("failed to create teams: %w", err)
		}
	}

	// Process each user from the config
	for _, userConfig := range config.Users {
		user, err := um.CreateOrGetUser(
			userConfig.Username,
			userConfig.Email,
			userConfig.Password,
			userConfig.Nickname,
			userConfig.IsSystemAdmin,
		)
		if err != nil {
			return fmt.Errorf("failed to create/get user '%s': %w", userConfig.Username, err)
		}

		// Add user to their specified teams
		if len(userConfig.Teams) > 0 && teamMap != nil {
			fmt.Printf("Adding user '%s' to %d teams...\n", userConfig.Username, len(userConfig.Teams))
			for _, teamName := range userConfig.Teams {
				team, exists := teamMap[teamName]
				if !exists {
					fmt.Printf("❌ Team '%s' not found for user '%s'\n", teamName, userConfig.Username)
					continue
				}

				if err := teamManager.AddUserToTeam(user, team); err != nil {
					fmt.Printf("❌ Failed to add user '%s' to team '%s': %v\n", userConfig.Username, teamName, err)
				}
			}
		}
	}

	return nil
}
