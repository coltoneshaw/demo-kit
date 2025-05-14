package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
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

// CategorizeMissionChannel categorizes a mission channel into the "Active Missions" category
// using the Mattermost Playbooks API
func (c *Client) CategorizeMissionChannel(channelID string) error {
	if channelID == "" {
		return fmt.Errorf("channel ID is required")
	}

	categoryName := "Active Missions"
	log.Printf("Categorizing mission channel %s in category '%s' using Playbooks API...", channelID, categoryName)

	// Construct the URL for the categorize channel API
	url := fmt.Sprintf("%s/plugins/playbooks/api/v0/actions/channels/%s", c.serverURL, channelID)

	// Create the payload
	type Category struct {
		CategoryName string `json:"category_name"`
	}

	type CategorizePayload struct {
		Enabled     bool     `json:"enabled"`
		Payload     Category `json:"payload"`
		ChannelID   string   `json:"channel_id"`
		ActionType  string   `json:"action_type"`
		TriggerType string   `json:"trigger_type"`
	}

	payload := CategorizePayload{
		Enabled: true,
		Payload: Category{
			CategoryName: categoryName,
		},
		ChannelID:   channelID,
		ActionType:  "categorize_channel",
		TriggerType: "new_member_joins",
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal categorize payload: %w", err)
	}

	// Create the request
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonPayload))
	if err != nil {
		return fmt.Errorf("failed to create categorize request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.client.AuthToken)

	// Send the request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send categorize request: %w", err)
	}
	defer resp.Body.Close()

	// Check response
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("categorize request failed with status %d: %s", resp.StatusCode, string(body))
	}

	log.Printf("✅ Successfully categorized mission channel in '%s' category", categoryName)
	return nil
}

// GetStatusEmoji returns the appropriate emoji for a mission status
func GetStatusEmoji(status string) string {
	switch status {
	case "stalled":
		return "🛑" // Stop sign - mission is not active
	case "in-air":
		return "✈️" // Airplane - mission is in flight
	case "completed":
		return "✅" // Check mark - mission successfully completed
	case "cancelled":
		return "❌" // Cross mark - mission cancelled
	default:
		return "⚪" // Default - unknown status
	}
}

// UpdateChannelDisplayName updates a channel's display name with the appropriate
// status emoji based on the mission status
func (c *Client) UpdateChannelDisplayName(ctx context.Context, channelID, callsign, name, status string) error {
	if channelID == "" {
		return fmt.Errorf("channel ID is required")
	}

	// Get the current channel
	channel, resp, err := c.client.GetChannel(ctx, channelID, "")
	if err != nil {
		return fmt.Errorf("failed to get channel: %w (status code: %v)", err, resp.StatusCode)
	}

	// Create the new display name with emoji prefix
	emoji := GetStatusEmoji(status)
	newDisplayName := fmt.Sprintf("%s %s: %s", emoji, callsign, name)

	// Check if the display name already has the correct emoji
	if channel.DisplayName == newDisplayName {
		// No change needed
		return nil
	}

	// Create patch with new display name
	patch := &model.ChannelPatch{
		DisplayName: &newDisplayName,
	}

	// Update the channel
	_, resp, err = c.client.PatchChannel(ctx, channelID, patch)
	if err != nil {
		return fmt.Errorf("failed to update channel display name: %w (status code: %v)", err, resp.StatusCode)
	}

	log.Printf("✅ Successfully updated channel display name to '%s'", newDisplayName)
	return nil
}

// UploadFile uploads a file to Mattermost and returns the file info
func (c *Client) UploadFile(ctx context.Context, channelID, filePath string) (*model.FileInfo, error) {
	if channelID == "" {
		return nil, fmt.Errorf("channel ID is required")
	}

	if filePath == "" {
		return nil, fmt.Errorf("file path is required")
	}

	// Check if file exists
	_, err := os.Stat(filePath)
	if os.IsNotExist(err) {
		// Return the original error so it can be properly checked with os.IsNotExist
		return nil, fmt.Errorf("file %s not found: %w", filePath, err)
	} else if err != nil {
		return nil, fmt.Errorf("failed to check file: %w", err)
	}

	// Open the file
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	// Extract filename from path
	filename := filepath.Base(filePath)

	// Read file contents
	fileData, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("failed to read file data: %w", err)
	}

	// Upload the file using UploadFileWithContext
	fileUploadResp, resp, err := c.client.UploadFile(ctx, fileData, channelID, filename)
	if err != nil {
		return nil, fmt.Errorf("failed to upload file: %w (status code: %v)", err, resp.StatusCode)
	}

	if len(fileUploadResp.FileInfos) == 0 {
		return nil, fmt.Errorf("file upload response contained no file infos")
	}

	log.Printf("✅ Successfully uploaded file %s to channel %s", filename, channelID)
	return fileUploadResp.FileInfos[0], nil
}

// SendPostWithAttachment creates and sends a post with a file attachment
func (c *Client) SendPostWithAttachment(ctx context.Context, channelID, message, filePath string) (*model.Post, error) {
	if channelID == "" {
		return nil, fmt.Errorf("channel ID is required")
	}

	// Upload the file
	fileInfo, err := c.UploadFile(ctx, channelID, filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to upload file for attachment: %w", err)
	}

	// Create post with file attachment
	post := &model.Post{
		ChannelId: channelID,
		Message:   message,
		FileIds:   []string{fileInfo.Id},
		Props: map[string]any{
			"from_webhook":      "true",
			"override_username": "mission-ops-bot",
		},
	}

	createdPost, resp, err := c.client.CreatePost(ctx, post)
	if err != nil {
		return nil, fmt.Errorf("error sending post with attachment: %w (status code: %d)", err, resp.StatusCode)
	}

	log.Printf("✅ Successfully sent post with attachment to channel %s", channelID)
	return createdPost, nil
}
