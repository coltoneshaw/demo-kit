package command

import (
	"fmt"
	"strings"

	"github.com/mattermost/mattermost/server/public/model"
)

// executeMissionUnsubscribeCommand handles the /mission unsubscribe command
func (c *Handler) executeMissionUnsubscribeCommand(args *model.CommandArgs) (*model.CommandResponse, error) {
	// Parse arguments
	commandArgs := parseArgs(args.Command)
	subscriptionID := commandArgs["id"]

	// Check for help request
	if commandArgs["help"] != "" || commandArgs["--help"] != "" {
		return executeMissionUnsubscribeHelpCommand(args)
	}

	if subscriptionID == "" {
		// If no ID provided, list subscriptions for the channel
		return c.executeMissionSubscriptionsCommand(args)
	}

	// Get the subscription
	sub, err := c.subscription.GetSubscription(subscriptionID)
	if err != nil {
		return &model.CommandResponse{
			ResponseType: model.CommandResponseTypeEphemeral,
			Text:         fmt.Sprintf("Subscription with ID `%s` not found.", subscriptionID),
		}, nil
	}

	// Check if the subscription belongs to this channel
	if sub.ChannelID != args.ChannelId {
		return &model.CommandResponse{
			ResponseType: model.CommandResponseTypeEphemeral,
			Text:         "This subscription does not belong to this channel.",
		}, nil
	}

	// Remove the subscription
	if err := c.subscription.RemoveSubscription(subscriptionID); err != nil {
		c.client.Log.Error("Error removing subscription", "error", err.Error())
		return &model.CommandResponse{
			ResponseType: model.CommandResponseTypeEphemeral,
			Text:         "Failed to unsubscribe. Please try again.",
		}, nil
	}

	// Format status types for display
	statusTypesText := "all mission statuses"
	if len(sub.StatusTypes) > 0 {
		statusTypesText = fmt.Sprintf("mission statuses: %s", strings.Join(sub.StatusTypes, ", "))
	}

	return &model.CommandResponse{
		ResponseType: model.CommandResponseTypeInChannel,
		Text:         fmt.Sprintf("✅ Unsubscribed from %s.", statusTypesText),
	}, nil
}

// executeMissionUnsubscribeHelpCommand handles showing help for the unsubscribe command
func executeMissionUnsubscribeHelpCommand(args *model.CommandArgs) (*model.CommandResponse, error) {
	helpText := "**Mission Unsubscribe Command Help**\n\n" +
		"The unsubscribe command allows you to stop receiving automatic mission updates.\n\n" +
		"**Usage:**\n" +
		"- `/mission unsubscribe --id [subscription_id]` - Unsubscribe from mission updates\n" +
		"- `/mission unsubscribe` - List all subscriptions in this channel with their IDs\n\n" +
		"**Parameters:**\n" +
		"- `--id`: The ID of the subscription to cancel (required)\n\n" +
		"**Example:**\n" +
		"- `/mission unsubscribe --id mission-sub-abc123`\n\n" +
		"If you don't know your subscription ID, run `/mission subscriptions` to see all active subscriptions in this channel."

	return &model.CommandResponse{
		ResponseType: model.CommandResponseTypeEphemeral,
		Text:         helpText,
	}, nil
}
