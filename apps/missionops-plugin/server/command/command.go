package command

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/coltoneshaw/demokit/missionops-plugin/server/bot"
	"github.com/coltoneshaw/demokit/missionops-plugin/server/mission"
	"github.com/coltoneshaw/demokit/missionops-plugin/server/subscription"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/pluginapi"
)

type Handler struct {
	client       *pluginapi.Client
	mission      mission.MissionInterface
	bot          bot.BotInterface
	subscription subscription.SubscriptionInterface
}

type Command interface {
	Handle(args *model.CommandArgs) (*model.CommandResponse, error)
	executeMissionHelpCommand(args *model.CommandArgs) (*model.CommandResponse, error)
	executeMissionStartCommand(args *model.CommandArgs) (*model.CommandResponse, error)
	executeMissionListCommand(args *model.CommandArgs) (*model.CommandResponse, error)
	executeMissionStatusCommand(args *model.CommandArgs) (*model.CommandResponse, error)
	executeMissionCompleteCommand(args *model.CommandArgs) (*model.CommandResponse, error)
	executeMissionSubscribeCommand(args *model.CommandArgs) (*model.CommandResponse, error)
	executeMissionUnsubscribeCommand(args *model.CommandArgs) (*model.CommandResponse, error)
	executeMissionSubscriptionsCommand(args *model.CommandArgs) (*model.CommandResponse, error)
	HandleMissionComplete(w http.ResponseWriter, r *http.Request)
}

const helloCommandTrigger = "hello"

// Register all your slash commands in the NewCommandHandler function.
func NewCommandHandler(client *pluginapi.Client, mission mission.MissionInterface, bot bot.BotInterface, subscription subscription.SubscriptionInterface) Command {
	err := client.SlashCommand.Register(&model.Command{
		Trigger:          "mission",
		Description:      "Mission Operations Commands",
		DisplayName:      "Mission Ops",
		AutoComplete:     true,
		AutoCompleteDesc: "Available commands: start, list, status, complete, subscribe, unsubscribe, subscriptions",
		AutoCompleteHint: "[command]",
		AutocompleteData: &model.AutocompleteData{
			Trigger:  "mission",
			HelpText: "Mission Operations Commands",
			SubCommands: []*model.AutocompleteData{
				{
					Trigger:  "start",
					HelpText: "Create a new mission",
					Arguments: []*model.AutocompleteArg{
						{
							Type: model.AutocompleteArgTypeText,
							Data: &model.AutocompleteTextArg{
								Hint:    "[name]",
								Pattern: "^[a-zA-Z0-9-_\\s]+$",
							},
							Name:     "name",
							HelpText: "Mission name",
							Required: true,
						},
						{
							Type: model.AutocompleteArgTypeText,
							Data: &model.AutocompleteTextArg{
								Hint:    "[callsign]",
								Pattern: "^[a-zA-Z0-9-_]+$",
							},
							Name:     "callsign",
							HelpText: "Mission callsign",
							Required: true,
						},
						{
							Type: model.AutocompleteArgTypeText,
							Data: &model.AutocompleteTextArg{
								Hint:    "[airport]",
								Pattern: "^[A-Z]{3,4}$",
							},
							Name:     "departureAirport",
							HelpText: "Departure airport code",
							Required: true,
						},
						{
							Type: model.AutocompleteArgTypeText,
							Data: &model.AutocompleteTextArg{
								Hint:    "[airport]",
								Pattern: "^[A-Z]{3,4}$",
							},
							Name:     "arrivalAirport",
							HelpText: "Arrival airport code",
							Required: true,
						},
						{
							Type: model.AutocompleteArgTypeText,
							Data: &model.AutocompleteTextArg{
								Hint: "@user1 @user2",
							},
							Name:     "crew",
							HelpText: "Crew members (space-separated usernames)",
							Required: false,
						},
					},
				},
				{
					Trigger:  "list",
					HelpText: "List all missions",
				},
				{
					Trigger:  "status",
					HelpText: "Update mission status",
					Arguments: []*model.AutocompleteArg{
						{
							Type: model.AutocompleteArgTypeStaticList,
							Data: &model.AutocompleteStaticListArg{
								PossibleArguments: []model.AutocompleteListItem{
									{
										Item:     "stalled",
										HelpText: "Mission is not active",
									},
									{
										Item:     "in-air",
										HelpText: "Mission is in progress",
									},
									{
										Item:     "completed",
										HelpText: "Mission has been completed successfully",
									},
									{
										Item:     "cancelled",
										HelpText: "Mission has been cancelled",
									},
								},
							},
							HelpText: "New status for the mission",
							Required: true,
						},
						{
							Type: model.AutocompleteArgTypeText,
							Data: &model.AutocompleteTextArg{
								Hint: "[mission-id]",
							},
							Name:     "id",
							HelpText: "Mission ID (required if not in a mission channel)",
							Required: false,
						},
					},
				},
				{
					Trigger:  "complete",
					HelpText: "Fill out a post-mission report",
					Arguments: []*model.AutocompleteArg{
						{
							Type: model.AutocompleteArgTypeText,
							Data: &model.AutocompleteTextArg{
								Hint: "[mission-id]",
							},
							Name:     "id",
							HelpText: "Mission ID (required if not in a mission channel)",
							Required: false,
						},
					},
				},
				{
					Trigger:  "subscribe",
					HelpText: "Subscribe to mission status updates",
					Arguments: []*model.AutocompleteArg{
						{
							Type: model.AutocompleteArgTypeText,
							Data: &model.AutocompleteTextArg{
								Hint: "status1,status2 or all",
							},
							Name:     "type",
							HelpText: "Status types to subscribe to (comma-separated or 'all')",
							Required: true,
						},
						{
							Type: model.AutocompleteArgTypeText,
							Data: &model.AutocompleteTextArg{
								Hint:    "[seconds]",
								Pattern: "^[0-9]+$",
							},
							Name:     "frequency",
							HelpText: "Update frequency in seconds (minimum 300)",
							Required: true,
						},
					},
				},
				{
					Trigger:  "unsubscribe",
					HelpText: "Unsubscribe from mission updates",
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
				{
					Trigger:  "subscriptions",
					HelpText: "List all subscriptions in this channel",
				},
			},
		},
	})
	if err != nil {
		client.Log.Error("Failed to register command", "error", err)
	}
	return &Handler{
		client:       client,
		mission:      mission,
		bot:          bot,
		subscription: subscription,
	}
}

// // ExecuteCommand hook calls this method to execute the commands that were registered in the NewCommandHandler function.
// func (c *Handler) Handle(args *model.CommandArgs) (*model.CommandResponse, error) {
// 	trigger := strings.TrimPrefix(strings.Fields(args.Command)[0], "/")
// 	switch trigger {
// 	case helloCommandTrigger:
// 		return c.executeHelloCommand(args), nil
// 	default:
// 		return &model.CommandResponse{
// 			ResponseType: model.CommandResponseTypeEphemeral,
// 			Text:         fmt.Sprintf("Unknown command: %s", args.Command),
// 		}, nil
// 	}
// }

// ExecuteCommand handles the mission slash command
func (c *Handler) Handle(args *model.CommandArgs) (*model.CommandResponse, error) {
	c.client.Log.Debug("Handling command", "command", args.Command)

	// Split the command into parts
	split := strings.Fields(args.Command)
	if len(split) < 2 {
		// Command with no subcommand
		return c.executeMissionHelpCommand(args)
	}

	// Get the subcommand
	subcommand := split[1]

	switch subcommand {
	case "start":
		return c.executeMissionStartCommand(args)
	case "list":
		return c.executeMissionListCommand(args)
	case "status":
		return c.executeMissionStatusCommand(args)
	case "complete":
		return c.executeMissionCompleteCommand(args)
	case "subscribe":
		return c.executeMissionSubscribeCommand(args)
	case "unsubscribe":
		return c.executeMissionUnsubscribeCommand(args)
	case "subscriptions":
		return c.executeMissionSubscriptionsCommand(args)
	case "help", "--help":
		return c.executeMissionHelpCommand(args)
	default:
		return &model.CommandResponse{
			ResponseType: model.CommandResponseTypeEphemeral,
			Text:         fmt.Sprintf("Unknown subcommand: %s. Use `/mission help` for available commands.", subcommand),
		}, nil
	}
}

func (c *Handler) logCommandError(text string) *model.CommandResponse {
	c.client.Log.Error(text)
	return &model.CommandResponse{
		ResponseType: model.CommandResponseTypeEphemeral,
		Text:         text,
	}
}
