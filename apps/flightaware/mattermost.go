package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"
)

// sendMattermostMessage sends a message to a Mattermost channel
func sendMattermostMessage(channelID, text string) error {
	// Get the Mattermost webhook URL from environment variable
	webhookURL := os.Getenv("FLIGHTS_MATTERMOST_WEBHOOK_URL")
	if webhookURL == "" {
		return fmt.Errorf("FLIGHTS_MATTERMOST_WEBHOOK_URL environment variable not set")
	}

	// Create the webhook payload
	payload := map[string]interface{}{
		"channel_id": channelID,
		"text":       text,
	}

	// Convert payload to JSON
	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("error marshaling JSON: %v", err)
	}

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	// Make the request
	resp, err := client.Post(webhookURL, "application/json", bytes.NewBuffer(jsonPayload))
	if err != nil {
		return fmt.Errorf("error making request to Mattermost: %v", err)
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Mattermost returned non-OK status: %d", resp.StatusCode)
	}

	return nil
}
