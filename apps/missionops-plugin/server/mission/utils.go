package mission

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/pkg/errors"
)

// CategorizeMissionChannel adds a mission channel to the Active Missions category
// using the Mattermost Playbooks API
func (m *Mission) CategorizeMissionChannel(channelID, teamID string) error {
	if channelID == "" {
		return fmt.Errorf("channel ID is required")
	}

	categoryName := "Active Missions"
	m.client.Log.Debug("Categorizing mission channel using Playbooks API",
		"channelId", channelID,
		"categoryName", categoryName)

	// Get the site URL from the plugin API
	siteURL := m.client.Configuration.GetConfig().ServiceSettings.SiteURL

	// Construct the URL for the categorize channel API
	url := fmt.Sprintf("%s/plugins/playbooks/api/v0/actions/channels/%s", *siteURL, channelID)

	// Create the payload for Playbooks API
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
		return errors.Wrap(err, "failed to marshal categorize payload")
	}

	// Create the request
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonPayload))
	if err != nil {
		return errors.Wrap(err, "failed to create categorize request")
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")

	req.Header.Set("Authorization", "Bearer "+m.bot.GetBotToken())

	// Send the request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return errors.Wrap(err, "failed to send categorize request")
	}
	defer resp.Body.Close()

	// Check response
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("categorize request failed with status %d: %s", resp.StatusCode, string(body))
	}

	m.client.Log.Debug("Successfully categorized mission channel",
		"channelId", channelID,
		"categoryName", categoryName)
	return nil
}
