package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/mattermost/mattermost/server/public/model"
)

// Client wraps the Mattermost API client
type Client struct {
	client    *model.Client4
	serverURL string
	username  string
	password  string
}

// NewClient creates a new Mattermost client
func NewClient() (*Client, error) {
	// Create a context with timeout for all API operations
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Get settings from environment variables
	serverURL := os.Getenv("MM_ServiceSettings_SiteURL")
	if serverURL == "" {
		return nil, fmt.Errorf("MM_ServiceSettings_SiteURL environment variable not set")
	}

	username := os.Getenv("MM_Admin_Username")
	if username == "" {
		return nil, fmt.Errorf("MM_Admin_Username environment variable not set")
	}

	password := os.Getenv("MM_Admin_Password")
	if password == "" {
		return nil, fmt.Errorf("MM_Admin_Password environment variable not set")
	}

	// Create the client
	api := model.NewAPIv4Client(serverURL)

	c := &Client{
		client:    api,
		serverURL: serverURL,
		username:  username,
		password:  password,
	}

	// Login with credentials using context
	_, resp, err := c.client.Login(ctx, username, password)
	if err != nil {
		return nil, fmt.Errorf("login failed: %w (status code: %v)", err, resp.StatusCode)
	}

	return c, nil
}

// GetNewContext creates a new context with a standard timeout for API calls
func (c *Client) GetNewContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 30*time.Second)
}