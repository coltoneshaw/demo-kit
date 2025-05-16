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

// CategorizeMissionChannel adds a mission channel to the Active Missions category
// using the Mattermost Playbooks API
func (m *Mission) CategorizeMissionChannel(channelID, teamID string) error {
	if channelID == "" {
		return fmt.Errorf("channel ID is required")
	}

	_, err := m.client.Channel.AddMember(channelID, m.bot.GetBotUserInfo().UserId)
	if err != nil {
		return errors.Wrap(err, "failed to add bot to channel")
	}

	categoryName := "Active Missions"
	m.client.Log.Debug("Categorizing mission channel using Playbooks API",
		"channelId", channelID,
		"categoryName", categoryName)

	// Construct the URL for the categorize channel API
	url := fmt.Sprintf("/playbooks/api/v0/actions/channels/%s", channelID)

	// Create the payload
	payload := CategorizePayload{
		Enabled: true,
		Payload: Category{
			CategoryName: categoryName,
		},
		ChannelID:   channelID,
		ActionType:  "categorize_channel",
		TriggerType: "new_member_joins",
	}

	// Convert payload to JSON
	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return errors.Wrap(err, "failed to marshal categorize payload")
	}

	// Create the request
	req, err := http.NewRequest("POST", url, bytes.NewReader(jsonPayload))
	if err != nil {
		return errors.Wrap(err, "failed to create request")
	}

	// Set content type header
	req.Header.Set("Content-Type", "application/json")

	// Set Mattermost-User-ID header for inter-plugin authentication
	req.Header.Set("Mattermost-User-ID", m.bot.GetBotUserInfo().UserId)

	// Make the request to the Playbooks plugin
	resp := m.client.Plugin.HTTP(req)
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("categorize request failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	m.client.Log.Debug("Successfully categorized mission channel",
		"channelId", channelID,
		"categoryName", categoryName)
	return nil
}
