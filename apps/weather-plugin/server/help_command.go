package main

import (
	"github.com/mattermost/mattermost/server/public/model"
)

type HelpCommand struct {
	messageService *MessageService
}

func NewHelpCommand(messageService *MessageService) *HelpCommand {
	return &HelpCommand{
		messageService: messageService,
	}
}

func (hc *HelpCommand) Execute(args *model.CommandArgs) (*model.CommandResponse, error) {
	helpText := "**Weather Bot Commands**\n\n" +
		"**Basic Commands:**\n" +
		"- `/weather <location>` - Get current weather for a location\n" +
		"- `/weather help` - Show this help message\n" +
		"- `/weather list` - List active subscriptions in this channel\n" +
		"- `/weather list --all` - List all subscriptions on the server\n\n" +
		"**Subscription Commands:**\n" +
		"- `/weather subscribe --location <location> --frequency <frequency>` - Subscribe to weather updates\n" +
		"- `/weather unsubscribe <subscription_id>` - Unsubscribe from specific weather updates\n\n" +
		"**Parameters:**\n" +
		"- `location` - Any location name (returns random weather data)\n" +
		"- `frequency` - How often to send updates in milliseconds (e.g., 3600000 for hourly) or duration (e.g., 1h, 30m)\n\n" +
		"**Examples:**\n" +
		"- `/weather London` - Get current weather for London\n" +
		"- `/weather subscribe --location Tokyo --frequency 1h` - Get hourly weather updates for Tokyo\n" +
		"- `/weather subscribe --location \"New York\" --frequency 30m` - Get updates every 30 minutes for New York"

	return hc.messageService.SendEphemeralResponse(args, helpText)
}