package command

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/coltoneshaw/demokit/missionops-plugin/server/subscription"
	"github.com/mattermost/mattermost/server/public/model"
)

// executeMissionSubscribeCommand handles the /mission subscribe command
func (c *Handler) executeMissionSubscribeCommand(args *model.CommandArgs) (*model.CommandResponse, error) {
	// Parse arguments
	commandArgs := parseArgs(args.Command)

	typesStr := commandArgs["type"]
	frequencyStr := commandArgs["frequency"]

	// Check for help request
	if commandArgs["help"] != "" || commandArgs["--help"] != "" {
		return executeMissionSubscribeHelpCommand(args)
	}

	if typesStr == "" {
		return &model.CommandResponse{
			ResponseType: model.CommandResponseTypeEphemeral,
			Text:         "Status types are required. Use `--type [status1,status2,...]` or `--type all`",
		}, nil
	}

	if frequencyStr == "" {
		return &model.CommandResponse{
			ResponseType: model.CommandResponseTypeEphemeral,
			Text:         "Update frequency is required. Use `--frequency [seconds]`",
		}, nil
	}

	// Parse status types
	var statusTypes []string
	if typesStr != "all" {
		// Split by comma
		statusTypes = strings.Split(typesStr, ",")

		// Validate each status type
		validStatuses := map[string]bool{
			"stalled":   true,
			"in-air":    true,
			"completed": true,
			"cancelled": true,
		}

		for _, status := range statusTypes {
			if !validStatuses[status] {
				return &model.CommandResponse{
					ResponseType: model.CommandResponseTypeEphemeral,
					Text:         fmt.Sprintf("Invalid status type: %s. Valid types: stalled, in-air, completed, cancelled, or 'all'", status),
				}, nil
			}
		}
	}

	// Parse frequency
	frequency, err := strconv.ParseInt(frequencyStr, 10, 64)
	if err != nil {
		return &model.CommandResponse{
			ResponseType: model.CommandResponseTypeEphemeral,
			Text:         "Invalid frequency format. Please use seconds (e.g., 3600 for hourly).",
		}, nil
	}

	// Validate minimum frequency (5 minutes = 300 seconds)
	if frequency < 300 {
		return &model.CommandResponse{
			ResponseType: model.CommandResponseTypeEphemeral,
			Text:         "Frequency must be at least 300 seconds (5 minutes).",
		}, nil
	}

	// Create a new subscription
	subscription := &subscription.MissionSubscription{
		ID:              fmt.Sprintf("mission-sub-%s-%d", args.ChannelId, time.Now().Unix()),
		ChannelID:       args.ChannelId,
		UserID:          args.UserId,
		StatusTypes:     statusTypes,
		UpdateFrequency: frequency,
		LastUpdated:     time.Now(),
	}

	// Add the subscription
	if err := c.subscription.AddSubscription(subscription); err != nil {
		c.client.Log.Error("Error adding subscription", "error", err.Error())
		return &model.CommandResponse{
			ResponseType: model.CommandResponseTypeEphemeral,
			Text:         fmt.Sprintf("Error setting up subscription: %v", err),
		}, nil
	}

	// Start the subscription job
	if err := c.subscription.StartSubscriptionJob(subscription); err != nil {
		c.client.Log.Error("Error starting subscription job", "error", err.Error())
		return &model.CommandResponse{
			ResponseType: model.CommandResponseTypeEphemeral,
			Text:         "Error starting subscription. Please try again.",
		}, nil
	}

	// Format status types for display
	statusTypesText := "all mission statuses"
	if len(statusTypes) > 0 {
		statusTypesText = fmt.Sprintf("mission statuses: %s", strings.Join(statusTypes, ", "))
	}

	// Send confirmation
	return &model.CommandResponse{
		ResponseType: model.CommandResponseTypeInChannel,
		Text:         fmt.Sprintf("✅ Subscribed to %s. Updates will be sent every %d seconds (ID: `%s`).", statusTypesText, frequency, subscription.ID),
	}, nil
}

// executeMissionSubscribeHelpCommand handles showing help for the subscribe command
func executeMissionSubscribeHelpCommand(args *model.CommandArgs) (*model.CommandResponse, *model.AppError) {
	helpText := "**Mission Subscription Command Help**\n\n" +
		"The subscribe command allows you to receive automatic updates about missions with specific statuses.\n\n" +
		"**Usage:**\n" +
		"- `/mission subscribe --type [status1,status2,...] --frequency [seconds]` - Subscribe to specific mission statuses\n" +
		"- `/mission subscribe --type all --frequency [seconds]` - Subscribe to all mission statuses\n\n" +
		"**Parameters:**\n" +
		"- `--type` or `--types`: Comma-separated list of statuses to subscribe to (stalled, in-air, completed, cancelled), or 'all'\n" +
		"- `--frequency`: How often to receive updates, in seconds (minimum 300 seconds / 5 minutes)\n\n" +
		"**Examples:**\n" +
		"- `/mission subscribe --type stalled,in-air --frequency 3600` - Hourly updates for stalled and in-air missions\n" +
		"- `/mission subscribe --type all --frequency 1800` - Updates every 30 minutes for all mission statuses\n\n" +
		"To view existing subscriptions, use `/mission subscriptions`\n" +
		"To cancel a subscription, use `/mission unsubscribe --id [subscription_id]`"

	return &model.CommandResponse{
		ResponseType: model.CommandResponseTypeEphemeral,
		Text:         helpText,
	}, nil
}
