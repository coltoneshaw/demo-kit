package mattermost

import (
	"os"
	"testing"
)

// getSiteURLFromEnv gets the site URL from environment or returns default
func getSiteURLFromEnv() (string, error) {
	return getEnvVariable("MM_SiteURL", "http://localhost:8065"), nil
}

// setupTestClient creates a client for testing
func setupTestClient(t *testing.T) *Client {
	t.Helper()

	// Get the site URL from environment variables
	siteURL, err := getSiteURLFromEnv()
	if err != nil {
		t.Fatalf("Failed to get siteURL from environment: %v", err)
	}

	// Create a new client with admin credentials from environment (using test defaults)
	adminUsername := getEnvVariable("MM_Username", "sysadmin")
	adminPassword := getEnvVariable("MM_Password", "Sys@dmin-sample1")

	client := NewClient(
		siteURL,
		adminUsername,
		adminPassword,
		"test-team",
		"", // No config path
	)
	
	// For tests, create a minimal config to support login flow
	client.Config = &Config{
		Environment:   "test",
		Server:        siteURL,
		AdminUsername: adminUsername,
		AdminPassword: adminPassword,
		DefaultTeam:   "test-team",
	}

	// Ensure we can connect to the server
	err = client.WaitForStart()
	if err != nil {
		t.Fatalf("Failed to connect to Mattermost server at %s: %v", siteURL, err)
	}

	// Login as admin
	err = client.Login()
	if err != nil {
		t.Fatalf("Failed to login to Mattermost server: %v", err)
	}

	return client
}

// getEnvVariable retrieves an environment variable or returns a default value
func getEnvVariable(name, defaultValue string) string {
	value := os.Getenv(name)
	if value == "" {
		return defaultValue
	}
	return value
}