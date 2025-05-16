package command

import (
	"fmt"
	"strings"
	"time"

	"github.com/mattermost/mattermost/server/public/model"
)

// executeMissionSubscriptionsCommand handles the /mission subscriptions command
func (c *Handler) executeMissionSubscriptionsCommand(args *model.CommandArgs) (*model.CommandResponse, error) {
	// Parse arguments for help
	commandArgs := parseArgs(args.Command)
	if commandArgs["help"] != "" || commandArgs["--help"] != "" {
		return executeMissionSubscriptionsHelpCommand(args)
	}

	// Get subscriptions for the channel
	subs, err := c.subscription.GetSubscriptionsForChannel(args.ChannelId)
	if err != nil {
		c.client.Log.Error("Error getting subscriptions", "error", err.Error())
		return &model.CommandResponse{
			ResponseType: model.CommandResponseTypeEphemeral,
			Text:         fmt.Sprintf("Error getting subscriptions: %v", err),
		}, nil
	}

	if len(subs) == 0 {
		return &model.CommandResponse{
			ResponseType: model.CommandResponseTypeEphemeral,
			Text:         "No active subscriptions found in this channel.",
		}, nil
	}

	var sb strings.Builder
	sb.WriteString("**Active Mission Subscriptions in this Channel:**\n\n")
	sb.WriteString("| ID | Status Types | Frequency | Last Updated | Next Update In |\n")
	sb.WriteString("|---|-------------|-----------|-------------|-------------|\n")

	now := time.Now()

	for _, sub := range subs {
		// Format status types for display
		statusTypesText := "all"
		if len(sub.StatusTypes) > 0 {
			statusTypesText = strings.Join(sub.StatusTypes, ", ")
		}

		// Calculate time until next update
		nextUpdateTime := sub.LastUpdated.Add(time.Duration(sub.UpdateFrequency) * time.Second)
		var timeUntilNext string

		if now.After(nextUpdateTime) {
			timeUntilNext = "Due now"
		} else {
			// Calculate the duration until next update
			duration := nextUpdateTime.Sub(now)

			// Format in a human-readable way
			if duration.Hours() >= 1 {
				hours := int(duration.Hours())
				minutes := int(duration.Minutes()) % 60
				timeUntilNext = fmt.Sprintf("%dh %dm", hours, minutes)
			} else if duration.Minutes() >= 1 {
				minutes := int(duration.Minutes())
				seconds := int(duration.Seconds()) % 60
				timeUntilNext = fmt.Sprintf("%dm %ds", minutes, seconds)
			} else {
				timeUntilNext = fmt.Sprintf("%ds", int(duration.Seconds()))
			}
		}

		sb.WriteString(fmt.Sprintf("| `%s` | %s | %d seconds | %s | %s |\n",
			sub.ID, statusTypesText, sub.UpdateFrequency, sub.LastUpdated.Format(time.RFC1123), timeUntilNext))
	}

	sb.WriteString("\nTo unsubscribe, use `/mission unsubscribe --id [subscription_id]`")

	_, err = c.bot.PostMessageFromBot(args.ChannelId, sb.String())
	if err != nil {
		c.client.Log.Error("Error sending message", "error", err.Error())
		return &model.CommandResponse{
			ResponseType: model.CommandResponseTypeEphemeral,
			Text:         "Error sending message. Please check your permissions.",
		}, nil
	}

	return &model.CommandResponse{
		ResponseType: model.CommandResponseTypeInChannel,
		Text:         "",
	}, nil
}

// executeMissionSubscriptionsHelpCommand handles showing help for the subscriptions command
func executeMissionSubscriptionsHelpCommand(args *model.CommandArgs) (*model.CommandResponse, error) {
	helpText := "**Mission Subscriptions Command Help**\n\n" +
		"The subscriptions command shows all active mission status subscriptions in the current channel.\n\n" +
		"**Usage:**\n" +
		"- `/mission subscriptions` - List all active subscriptions in this channel\n\n" +
		"**Available Information:**\n" +
		"- Subscription ID (needed for unsubscribing)\n" +
		"- Status types being monitored\n" +
		"- Update frequency\n" +
		"- Last update time\n" +
		"- Time until next update\n\n" +
		"To subscribe to mission updates, use `/mission subscribe --type [status1,status2] --frequency [seconds]`\n" +
		"To cancel a subscription, use `/mission unsubscribe --id [subscription_id]`"

	return &model.CommandResponse{
		ResponseType: model.CommandResponseTypeEphemeral,
		Text:         helpText,
	}, nil
}
