package mattermost

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/sirupsen/logrus"
)

// categorizeChannelAPI implements channel categorization using the Playbooks API
func (c *Client) categorizeChannelAPI(channelID string, channelName string, categoryName string) error {
	if channelID == "" || categoryName == "" {
		return fmt.Errorf("channel ID and category name are required")
	}

	// Check if channel already has a categorization action to avoid duplicates
	checkURL := fmt.Sprintf("%s/plugins/playbooks/api/v0/actions/channels/%s", c.ServerURL, channelID)
	checkReq, err := http.NewRequest("GET", checkURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create check request: %w", err)
	}
	checkReq.Header.Set("Authorization", "Bearer "+c.API.AuthToken)

	client := &http.Client{}
	checkResp, err := client.Do(checkReq)
	if err != nil {
		return fmt.Errorf("failed to check existing actions: %w", err)
	}
	defer func() { _ = checkResp.Body.Close() }()

	if checkResp.StatusCode == http.StatusOK {
		// Channel already has actions, check existing categorization
		body, _ := io.ReadAll(checkResp.Body)
		if strings.Contains(string(body), "categorize_channel") {
			// Return a special error to indicate "already categorized" (not a real error)
			return fmt.Errorf("ALREADY_CATEGORIZED")
		}
	}

	// Log only when we're actually going to create a new categorization action
	Log.WithFields(logrus.Fields{
		"channel_id": channelID,
	}).Debug(fmt.Sprintf("📋 Categorizing %s into %s", channelName, categoryName))

	// Construct the URL for the categorize channel API
	url := fmt.Sprintf("%s/plugins/playbooks/api/v0/actions/channels/%s",
		c.ServerURL, channelID)

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
	req.Header.Set("Authorization", "Bearer "+c.API.AuthToken)

	// Send the request
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send categorize request: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			Log.WithFields(logrus.Fields{"error": closeErr.Error()}).Warn("⚠️ Failed to close response body")
		}
	}()

	// Check response
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("categorize request failed with status %d: %s", resp.StatusCode, string(body))
	}

	Log.WithFields(logrus.Fields{
		"channel_id": channelID,
	}).Info(fmt.Sprintf("✅ Successfully categorized %s into %s", channelName, categoryName))
	return nil
}

// setChannelBannerAPI sets a banner for a channel using the Mattermost API
func (c *Client) setChannelBannerAPI(channelID, channelName, text, backgroundColor string, enabled bool) error {
	if channelID == "" {
		return fmt.Errorf("channel ID is required")
	}

	Log.WithFields(logrus.Fields{
		"channel_id": channelID,
	}).Debug(fmt.Sprintf("🎯 Setting banner for %s: %s", channelName, text))

	// Create the banner info using the proper model structure
	bannerInfo := &model.ChannelBannerInfo{
		Text:            &text,
		BackgroundColor: &backgroundColor,
		Enabled:         &enabled,
	}

	// Use PatchChannel to update the channel with banner info
	channelPatch := &model.ChannelPatch{
		BannerInfo: bannerInfo,
	}

	_, resp, err := c.API.PatchChannel(context.Background(), channelID, channelPatch)
	if err != nil {
		return handleAPIError(fmt.Sprintf("failed to set banner for channel '%s'", channelName), err, resp)
	}

	Log.WithFields(logrus.Fields{
		"channel_id": channelID,
	}).Info(fmt.Sprintf("✅ Successfully set banner for %s", channelName))
	return nil
}

// processChannelCategories processes channel categories
func (c *Client) processChannelCategories(bulkImportPath string) error {
	Log.WithFields(logrus.Fields{"file_path": bulkImportPath}).Info("📋 Processing channel categories")

	file, err := os.Open(bulkImportPath)
	if err != nil {
		return err
	}
	defer closeWithLog(file, "bulk import file")

	categorizedCount := 0
	errorCount := 0

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var categoryImport ChannelCategoryImport
		if err := json.Unmarshal([]byte(line), &categoryImport); err != nil {
			continue
		}

		if categoryImport.Type == "channel-category" {
			for _, channelName := range categoryImport.Channels {
				if err := c.categorizeChannel(categoryImport.Team, channelName, categoryImport.Category); err != nil {
					if err.Error() == "ALREADY_CATEGORIZED" {
						// Don't count as error or update - just silently skip
						continue
					}
					Log.WithFields(logrus.Fields{
						"channel_name": channelName,
						"team_name":    categoryImport.Team,
						"category":     categoryImport.Category,
						"error":        err.Error(),
					}).Warn("⚠️ Failed to categorize channel")
					errorCount++
				} else {
					categorizedCount++
				}
			}
		}
	}

	if categorizedCount > 0 || errorCount > 0 {
		Log.WithFields(logrus.Fields{
			"categorized_count": categorizedCount,
			"error_count":       errorCount,
		}).Info("✅ Channel categorization complete")
	} else {
		Log.Info("✅ All channels already properly categorized")
	}

	return scanner.Err()
}

// processChannelBanners processes channel banners
func (c *Client) processChannelBanners(bulkImportPath string) error {
	Log.WithFields(logrus.Fields{"file_path": bulkImportPath}).Info("🎯 Processing channel banners")

	file, err := os.Open(bulkImportPath)
	if err != nil {
		return err
	}
	defer closeWithLog(file, "bulk import file")

	bannersCount := 0
	errorCount := 0

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var bannerImport ChannelBannerImport
		if err := json.Unmarshal([]byte(line), &bannerImport); err != nil {
			continue
		}

		if bannerImport.Type == "channel-banner" {
			if err := c.setChannelBanner(bannerImport.Banner.Team, bannerImport.Banner.Channel, bannerImport.Banner.Text, bannerImport.Banner.BackgroundColor, bannerImport.Banner.Enabled); err != nil {
				Log.WithFields(logrus.Fields{
					"channel_name":     bannerImport.Banner.Channel,
					"team_name":        bannerImport.Banner.Team,
					"banner_text":      bannerImport.Banner.Text,
					"background_color": bannerImport.Banner.BackgroundColor,
					"error":            err.Error(),
				}).Warn("⚠️ Failed to set channel banner")
				errorCount++
			} else {
				bannersCount++
			}
		}
	}

	if bannersCount > 0 || errorCount > 0 {
		Log.WithFields(logrus.Fields{
			"banners_count": bannersCount,
			"error_count":   errorCount,
		}).Info("✅ Channel banner setup complete")
	} else {
		Log.Info("✅ No channel banners to process")
	}

	return scanner.Err()
}

// processCommands processes commands
func (c *Client) processCommands(bulkImportPath string) error {
	Log.WithFields(logrus.Fields{"file_path": bulkImportPath}).Info("📋 Processing commands")
	file, err := os.Open(bulkImportPath)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			Log.WithFields(logrus.Fields{"file_path": bulkImportPath, "error": closeErr.Error()}).Warn("⚠️ Failed to close file")
		}
	}()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var commandImport CommandImport
		if err := json.Unmarshal([]byte(line), &commandImport); err != nil {
			continue
		}

		if commandImport.Type == "command" {
			if err := c.executeCommand(commandImport.Command.Team, commandImport.Command.Channel, commandImport.Command.Text); err != nil {
				Log.WithFields(logrus.Fields{
					"team_name":    commandImport.Command.Team,
					"channel_name": commandImport.Command.Channel,
					"command_text": commandImport.Command.Text,
					"error":        err.Error(),
				}).Warn("⚠️ Failed to execute command")
			}
		}
	}

	return scanner.Err()
}

// categorizeChannel categorizes a channel by name
func (c *Client) categorizeChannel(teamName, channelName, categoryName string) error {
	team, resp, err := c.API.GetTeamByName(context.Background(), teamName, "")
	if err != nil {
		return handleAPIError(fmt.Sprintf("failed to get team '%s'", teamName), err, resp)
	}

	channels, _, _ := c.API.GetPublicChannelsForTeam(context.Background(), team.Id, 0, 1000, "")
	privateChannels, _, _ := c.API.GetPrivateChannelsForTeam(context.Background(), team.Id, 0, 1000, "")
	channels = append(channels, privateChannels...)

	for _, channel := range channels {
		if channel.Name == channelName {
			return c.categorizeChannelAPI(channel.Id, channel.Name, categoryName)
		}
	}

	return fmt.Errorf("channel '%s' not found in team '%s'", channelName, teamName)
}

// setChannelBanner sets a banner for a channel by name
func (c *Client) setChannelBanner(teamName, channelName, text, backgroundColor string, enabled bool) error {
	team, resp, err := c.API.GetTeamByName(context.Background(), teamName, "")
	if err != nil {
		return handleAPIError(fmt.Sprintf("failed to get team '%s'", teamName), err, resp)
	}

	channels, _, _ := c.API.GetPublicChannelsForTeam(context.Background(), team.Id, 0, 1000, "")
	privateChannels, _, _ := c.API.GetPrivateChannelsForTeam(context.Background(), team.Id, 0, 1000, "")
	channels = append(channels, privateChannels...)

	for _, channel := range channels {
		if channel.Name == channelName {
			return c.setChannelBannerAPI(channel.Id, channel.Name, text, backgroundColor, enabled)
		}
	}

	return fmt.Errorf("channel '%s' not found in team '%s'", channelName, teamName)
}

// executeCommand executes a command in a channel
func (c *Client) executeCommand(teamName, channelName, commandText string) error {
	team, resp, err := c.API.GetTeamByName(context.Background(), teamName, "")
	if err != nil {
		return handleAPIError(fmt.Sprintf("failed to get team '%s'", teamName), err, resp)
	}

	channels, _, _ := c.API.GetPublicChannelsForTeam(context.Background(), team.Id, 0, 1000, "")
	privateChannels, _, _ := c.API.GetPrivateChannelsForTeam(context.Background(), team.Id, 0, 1000, "")
	channels = append(channels, privateChannels...)

	for _, channel := range channels {
		if channel.Name == channelName {
			_, resp, err := c.API.ExecuteCommand(context.Background(), channel.Id, commandText)
			return handleAPIError(fmt.Sprintf("failed to execute command '%s'", commandText), err, resp)
		}
	}

	return fmt.Errorf("channel '%s' not found in team '%s'", channelName, teamName)
}