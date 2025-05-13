package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
)

// sendMattermostMessage sends a message to a Mattermost channel
func sendMattermostMessage(channelID, text string) error {
	// Get the Mattermost webhook URL from environment variable
	webhookURL := os.Getenv("MATTERMOST_WEBHOOK_URL")
	if webhookURL == "" {
		return fmt.Errorf("MATTERMOST_WEBHOOK_URL environment variable not set")
	}
	
	// Create the webhook payload with channel parameter
	payload := map[string]interface{}{
		"text":         text,
		"channel":      channelID,  // This is the key parameter that controls where the message goes
		"username":     "Weather Bot",
		"icon_url":     os.Getenv("BOT_ICON_URL"),
	}
	
	// Convert to JSON
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("error marshaling webhook payload: %v", err)
	}
	
	// Send the request
	log.Printf("Sending webhook message to channel %s", channelID)
	resp, err := http.Post(webhookURL, "application/json", bytes.NewBuffer(payloadBytes))
	if err != nil {
		return fmt.Errorf("error sending webhook: %v", err)
	}
	defer resp.Body.Close()
	
	// Check response status
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("error from Mattermost webhook: status %d, body: %s", resp.StatusCode, string(body))
	}
	
	log.Printf("Successfully sent message to channel %s via webhook", channelID)
	return nil
}
