package main

import (
	"strings"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/pluginapi"
)

type CommandHandler struct {
	client              *pluginapi.Client
	weatherCommand      *WeatherCommand
	helpCommand         *HelpCommand
	subscriptionCommand *SubscriptionCommand
}

func NewCommandHandler(
	client *pluginapi.Client,
	weatherService *WeatherService,
	subscriptionManager *SubscriptionManager,
	formatter *WeatherFormatter,
	messageService *MessageService,
) *CommandHandler {
	parser := NewCommandParser()
	
	handler := &CommandHandler{
		client:              client,
		weatherCommand:      NewWeatherCommand(weatherService, formatter, messageService),
		helpCommand:         NewHelpCommand(messageService),
		subscriptionCommand: NewSubscriptionCommand(client, subscriptionManager, messageService, parser),
	}

	err := client.SlashCommand.Register(&model.Command{
		Trigger:          "weather",
		Description:      "Weather Bot Commands",
		DisplayName:      "Weather",
		AutoComplete:     true,
		AutoCompleteDesc: "Get weather data and manage subscriptions",
		AutoCompleteHint: "[location] or [command]",
		AutocompleteData: &model.AutocompleteData{
			Trigger:  "weather",
			HelpText: "Weather Bot Commands",
			SubCommands: []*model.AutocompleteData{
				{
					Trigger:  "help",
					HelpText: "Show help information",
				},
				{
					Trigger:  "list",
					HelpText: "List active subscriptions in this channel",
					Arguments: []*model.AutocompleteArg{
						{
							Type: model.AutocompleteArgTypeStaticList,
							Data: &model.AutocompleteStaticListArg{
								PossibleArguments: []model.AutocompleteListItem{
									{
										Item:     "--all",
										HelpText: "Show all subscriptions on the server",
									},
								},
							},
							HelpText: "Optional: show all subscriptions on server",
							Required: false,
						},
					},
				},
				{
					Trigger:  "subscribe",
					HelpText: "Subscribe to weather updates",
					Arguments: []*model.AutocompleteArg{
						{
							Type: model.AutocompleteArgTypeText,
							Data: &model.AutocompleteTextArg{
								Hint: "[location]",
							},
							Name:     "location",
							HelpText: "Location for weather updates",
							Required: true,
						},
						{
							Type: model.AutocompleteArgTypeText,
							Data: &model.AutocompleteTextArg{
								Hint: "[frequency in ms or duration like 1h, 30m]",
							},
							Name:     "frequency",
							HelpText: "Update frequency",
							Required: true,
						},
					},
				},
				{
					Trigger:  "unsubscribe",
					HelpText: "Unsubscribe from weather updates",
					Arguments: []*model.AutocompleteArg{
						{
							Type: model.AutocompleteArgTypeText,
							Data: &model.AutocompleteTextArg{
								Hint: "[subscription-id]",
							},
							Name:     "id",
							HelpText: "Subscription ID",
							Required: true,
						},
					},
				},
			},
		},
	})

	if err != nil {
		client.Log.Error("Failed to register weather command", "error", err)
	}

	return handler
}

func (ch *CommandHandler) Handle(args *model.CommandArgs) (*model.CommandResponse, error) {
	split := strings.Fields(args.Command)
	if len(split) < 2 {
		return ch.helpCommand.Execute(args)
	}

	command := split[1]
	switch command {
	case "help", "--help":
		return ch.helpCommand.Execute(args)
	case "list":
		showAll := len(split) > 2 && split[2] == "--all"
		return ch.subscriptionCommand.ExecuteList(args, showAll)
	case "subscribe":
		return ch.subscriptionCommand.ExecuteSubscribe(args, split)
	case "unsubscribe":
		subscriptionID := ""
		if len(split) >= 3 {
			subscriptionID = split[2]
		}
		return ch.subscriptionCommand.ExecuteUnsubscribe(args, subscriptionID)
	default:
		// Treat as location for regular weather request
		location := strings.Join(split[1:], " ")
		return ch.weatherCommand.Execute(args, location)
	}
}