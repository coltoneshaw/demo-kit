package command

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/coltoneshaw/demokit/flightaware-plugin/server/flight"
	"github.com/coltoneshaw/demokit/flightaware-plugin/server/subscription"
	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/pluginapi"
)

type Command interface {
	Handle(args *model.CommandArgs) (*model.CommandResponse, error)
}

type CommandHandler struct {
	client          *pluginapi.Client
	flightService   flight.FlightInterface
	subscriptionMgr subscription.SubscriptionInterface
	messageService  MessageServiceInterface
	parser          *CommandParser
}

type MessageServiceInterface interface {
	SendEphemeralResponse(args *model.CommandArgs, message string) (*model.CommandResponse, error)
	SendPublicResponse(args *model.CommandArgs, post *model.Post) (*model.CommandResponse, error)
	SendPublicMessage(channelID, message string) error
	GetBotUserID() string
}

type DeparturesArgs struct {
	Airport string
}

type SubscribeArgs struct {
	Airport         string
	FrequencyStr    string
	UpdateFrequency int64
}

type UnsubscribeArgs struct {
	SubscriptionID string
}


// TableFormatter helps build markdown tables for subscription listings
type TableFormatter struct {
	title   string
	headers []string
	rows    [][]string
}

func NewTableFormatter(title string, headers ...string) *TableFormatter {
	return &TableFormatter{
		title:   title,
		headers: headers,
		rows:    make([][]string, 0),
	}
}

func (tf *TableFormatter) AddRow(values ...string) {
	tf.rows = append(tf.rows, values)
}

func (tf *TableFormatter) Build() string {
	var sb strings.Builder

	// Add title
	if tf.title != "" {
		sb.WriteString(tf.title)
		sb.WriteString("\n\n")
	}

	// Add headers
	sb.WriteString("|")
	for _, header := range tf.headers {
		sb.WriteString(" ")
		sb.WriteString(header)
		sb.WriteString(" |")
	}
	sb.WriteString("\n")

	// Add separator
	sb.WriteString("|")
	for range tf.headers {
		sb.WriteString("---|")
	}
	sb.WriteString("\n")

	// Add rows
	for _, row := range tf.rows {
		sb.WriteString("|")
		for _, cell := range row {
			sb.WriteString(" ")
			sb.WriteString(cell)
			sb.WriteString(" |")
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

func NewCommandHandler(client *pluginapi.Client, flightService flight.FlightInterface, subscriptionMgr subscription.SubscriptionInterface, messageService MessageServiceInterface) Command {
	err := client.SlashCommand.Register(&model.Command{
		Trigger:          "flights",
		Description:      "FlightAware Commands",
		DisplayName:      "FlightAware",
		AutoComplete:     true,
		AutoCompleteDesc: "Get flight departures and manage subscriptions",
		AutoCompleteHint: "[command]",
		AutocompleteData: &model.AutocompleteData{
			Trigger:  "flights",
			HelpText: "FlightAware Commands",
			SubCommands: []*model.AutocompleteData{
				{
					Trigger:  "departures",
					HelpText: "Get departures from an airport",
					Arguments: []*model.AutocompleteArg{
						{
							Type: model.AutocompleteArgTypeStaticList,
							Data: &model.AutocompleteStaticListArg{
								PossibleArguments: []model.AutocompleteListItem{
									{
										Item:     "--airport",
										HelpText: "Specify airport code (e.g., SFO, LAX, JFK, RDU)",
									},
								},
							},
							Name:     "airport",
							HelpText: "Airport code (e.g., SFO, LAX, JFK, RDU)",
							Required: false,
						},
					},
				},
				{
					Trigger:  "subscribe",
					HelpText: "Subscribe to airport departure updates",
					Arguments: []*model.AutocompleteArg{
						{
							Type: model.AutocompleteArgTypeStaticList,
							Data: &model.AutocompleteStaticListArg{
								PossibleArguments: []model.AutocompleteListItem{
									{
										Item:     "--airport",
										HelpText: "Specify airport code (e.g., SFO, LAX, JFK, RDU)",
									},
									{
										Item:     "--frequency",
										HelpText: "Update frequency in seconds (minimum 300)",
									},
								},
							},
							Name:     "airport",
							HelpText: "Airport code (e.g., SFO, LAX, JFK, RDU)",
							Required: false,
						},
					},
				},
				{
					Trigger:  "unsubscribe",
					HelpText: "Unsubscribe from departure updates",
					Arguments: []*model.AutocompleteArg{
						{
							Type: model.AutocompleteArgTypeText,
							Data: &model.AutocompleteTextArg{
								Hint:    "[subscription id]",
								Pattern: "^[a-zA-Z0-9_-]+$",
							},
							Required: true,
						},
					},
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
										HelpText: "(optional) Generate a list of all subscriptions on the server",
									},
								},
							},
							Required: false,
						},
					},
				},
				{
					Trigger:  "help",
					HelpText: "Show help information",
				},
			},
		},
	})
	if err != nil {
		client.Log.Error("Failed to register flights command", "error", err)
	}

	return &CommandHandler{
		client:          client,
		flightService:   flightService,
		subscriptionMgr: subscriptionMgr,
		messageService:  messageService,
		parser:          &CommandParser{},
	}
}

func (ch *CommandHandler) Handle(args *model.CommandArgs) (*model.CommandResponse, error) {
	subcommand, cmdArgs := ch.parseCommand(args.Command)

	if subcommand == "" {
		return ch.sendHelpResponse(args)
	}

	switch subcommand {
	case "departures":
		return ch.handleDeparturesCommand(args, cmdArgs)
	case "subscribe":
		return ch.handleSubscribeCommand(args, cmdArgs)
	case "unsubscribe":
		return ch.handleUnsubscribeCommand(args, cmdArgs)
	case "list":
		return ch.handleListCommand(args, cmdArgs)
	case "help", "--help":
		return ch.sendHelpResponse(args)
	default:
		return ch.sendUnknownCommandError(subcommand), nil
	}
}

// parseCommand extracts the subcommand and arguments from the raw command string
func (ch *CommandHandler) parseCommand(command string) (string, []string) {
	split := strings.Fields(command)
	if len(split) < 2 {
		return "", nil
	}

	subcommand := split[1]
	var cmdArgs []string
	if len(split) > 2 {
		cmdArgs = split[2:]
	}

	return subcommand, cmdArgs
}

// sendUnknownCommandError creates a standardized error response for unknown subcommands
func (ch *CommandHandler) sendUnknownCommandError(subcommand string) *model.CommandResponse {
	return ch.sendErrorResponse(fmt.Sprintf("Unknown subcommand: %s. Use `/flights help` for available commands.", subcommand))
}

// buildCommandFields creates the command array expected by the parser methods
func (ch *CommandHandler) buildCommandFields(subcommand string, cmdArgs []string) []string {
	return append([]string{"/flights", subcommand}, cmdArgs...)
}

func (ch *CommandHandler) handleDeparturesCommand(args *model.CommandArgs, cmdArgs []string) (*model.CommandResponse, error) {
	commandFields := ch.buildCommandFields("departures", cmdArgs)
	parsedArgs, err := ch.parser.ParseDeparturesCommand(commandFields)
	if err != nil {
		return ch.sendErrorResponse(fmt.Sprintf("Invalid command: %v. Use `/flights help` for usage.", err)), nil
	}

	flights, err := ch.flightService.GetDepartureFlights(parsedArgs.Airport)
	if err != nil {
		ch.client.Log.Error("Failed to fetch departure flights", "airport", parsedArgs.Airport, "error", err)
		return ch.sendErrorResponse(fmt.Sprintf("Unable to retrieve flight departures for %s. Please try again later.", parsedArgs.Airport)), nil
	}

	response := ch.flightService.FormatFlightResponse(flights, parsedArgs.Airport)

	post := &model.Post{
		ChannelId: args.ChannelId,
		Message:   response,
	}

	return ch.messageService.SendPublicResponse(args, post)
}

func (ch *CommandHandler) handleSubscribeCommand(args *model.CommandArgs, cmdArgs []string) (*model.CommandResponse, error) {
	commandFields := ch.buildCommandFields("subscribe", cmdArgs)
	parsedArgs, err := ch.parser.ParseSubscribeCommand(commandFields)
	if err != nil {
		return ch.sendErrorResponse(fmt.Sprintf("Invalid command: %v. Use `/flights help` for usage.", err)), nil
	}

	sub := &subscription.FlightSubscription{
		ID:              fmt.Sprintf("%s-%s-%d", parsedArgs.Airport, args.ChannelId, time.Now().Unix()),
		Airport:         parsedArgs.Airport,
		ChannelID:       args.ChannelId,
		UserID:          args.UserId,
		UpdateFrequency: parsedArgs.UpdateFrequency,
		LastUpdated:     time.Now(),
	}

	if err := ch.subscriptionMgr.AddSubscription(sub); err != nil {
		ch.client.Log.Error("Failed to create subscription", "airport", parsedArgs.Airport, "frequency", parsedArgs.UpdateFrequency, "channel_id", args.ChannelId, "error", err)
		return ch.sendErrorResponse(fmt.Sprintf("Unable to create subscription for %s. Please try again later.", parsedArgs.Airport)), nil
	}

	message := fmt.Sprintf("✅ Subscribed to departures from **%s**. Updates will be sent every %d seconds (ID: `%s`).", parsedArgs.Airport, parsedArgs.UpdateFrequency, sub.ID)
	post := &model.Post{
		ChannelId: args.ChannelId,
		Message:   message,
	}

	return ch.messageService.SendPublicResponse(args, post)
}

func (ch *CommandHandler) handleUnsubscribeCommand(args *model.CommandArgs, cmdArgs []string) (*model.CommandResponse, error) {
	commandFields := ch.buildCommandFields("unsubscribe", cmdArgs)
	parsedArgs, err := ch.parser.ParseUnsubscribeCommand(commandFields)
	if err != nil {
		return ch.sendErrorResponse(fmt.Sprintf("Invalid command: %v. Use `/flights help` for usage.", err)), nil
	}

	if parsedArgs.SubscriptionID == "" {
		subs := ch.subscriptionMgr.GetSubscriptionsForChannel(args.ChannelId)
		if len(subs) == 0 {
			return ch.sendErrorResponse("No active subscriptions found in this channel."), nil
		}

		table := NewTableFormatter("**Active Subscriptions in this Channel:**", "ID", "Airport", "Frequency", "Last Updated")
		for _, sub := range subs {
			table.AddRow(
				fmt.Sprintf("`%s`", sub.ID),
				sub.Airport,
				fmt.Sprintf("%d seconds", sub.UpdateFrequency),
				sub.LastUpdated.Format(time.RFC1123),
			)
		}

		message := table.Build() + "\nTo unsubscribe, use `/flights unsubscribe --id [subscription_id]`"
		return ch.messageService.SendEphemeralResponse(args, message)
	}

	sub, exists := ch.subscriptionMgr.GetSubscription(parsedArgs.SubscriptionID)
	if !exists {
		return ch.sendErrorResponse(fmt.Sprintf("Subscription with ID `%s` not found.", parsedArgs.SubscriptionID)), nil
	}

	if sub.ChannelID != args.ChannelId {
		return ch.sendErrorResponse("This subscription does not belong to this channel."), nil
	}

	if ch.subscriptionMgr.RemoveSubscription(parsedArgs.SubscriptionID) {
		message := fmt.Sprintf("✅ Unsubscribed from departures from **%s**.", sub.Airport)
		post := &model.Post{
			ChannelId: args.ChannelId,
			Message:   message,
		}
		return ch.messageService.SendPublicResponse(args, post)
	} else {
		return ch.sendErrorResponse("Failed to unsubscribe. Please try again."), nil
	}
}

func (ch *CommandHandler) handleListCommand(args *model.CommandArgs, cmdArgs []string) (*model.CommandResponse, error) {
	// Check if --all flag is provided
	showAll := slices.Contains(cmdArgs, "--all")

	var subs []*subscription.FlightSubscription
	var title string

	if showAll {
		subs = ch.subscriptionMgr.GetAllSubscriptions()
		title = "**All Active Flight Subscriptions on Server:**\n\n"
	} else {
		subs = ch.subscriptionMgr.GetSubscriptionsForChannel(args.ChannelId)
		title = "**Active Flight Subscriptions in this Channel:**\n\n"
	}

	if len(subs) == 0 {
		if showAll {
			return ch.sendErrorResponse("No active subscriptions found on the server."), nil
		} else {
			return ch.sendErrorResponse("No active subscriptions found in this channel."), nil
		}
	}

	var table *TableFormatter
	if showAll {
		table = NewTableFormatter(title, "ID", "Airport", "Channel", "Frequency", "Last Updated")
		for _, sub := range subs {
			channelName := "Unknown Channel"
			channelData, err := ch.client.Channel.Get(sub.ChannelID)
			if err != nil {
				ch.client.Log.Error("Failed to get channel data", "channel_id", sub.ChannelID, "error", err)
			}
			channelName = channelData.Name

			table.AddRow(
				fmt.Sprintf("`%s`", sub.ID),
				sub.Airport,
				fmt.Sprintf("~%s", channelName),
				fmt.Sprintf("%d seconds", sub.UpdateFrequency),
				sub.LastUpdated.Format(time.RFC1123),
			)
		}
	} else {
		table = NewTableFormatter(title, "ID", "Airport", "Frequency", "Last Updated")
		for _, sub := range subs {
			table.AddRow(
				fmt.Sprintf("`%s`", sub.ID),
				sub.Airport,
				fmt.Sprintf("%d seconds", sub.UpdateFrequency),
				sub.LastUpdated.Format(time.RFC1123),
			)
		}
	}

	post := &model.Post{
		ChannelId: args.ChannelId,
		Message:   table.Build(),
	}
	return ch.messageService.SendPublicResponse(args, post)
}

func (ch *CommandHandler) sendErrorResponse(message string) *model.CommandResponse {
	return &model.CommandResponse{
		Text:         message,
		ResponseType: model.CommandResponseTypeEphemeral,
	}
}

func (ch *CommandHandler) sendHelpResponse(args *model.CommandArgs) (*model.CommandResponse, error) {
	helpText := "**Flight Departures Bot Commands**\n\n" +
		"**One-time Queries:**\n" +
		"- `/flights departures --airport [code]` - Get recent departures from an airport\n\n" +
		"**Subscription Commands:**\n" +
		"- `/flights subscribe --airport [code] --frequency [seconds]` - Subscribe to airport departures\n" +
		"- `/flights unsubscribe --id [subscription_id]` - Unsubscribe from airport departures\n" +
		"- `/flights list` - List all subscriptions in this channel\n" +
		"- `/flights list --all` - List all subscriptions on the server\n" +
		"- `/flights help` - Show this help message\n\n" +
		"**Examples:**\n" +
		"- `/flights departures --airport SFO` - Get departures from San Francisco International\n" +
		"- `/flights departures --airport RDU` - Get departures from Raleigh-Durham International\n" +
		"- `/flights subscribe --airport EGLL --frequency 3600` - Subscribe to hourly updates for London Heathrow\n\n" +
		"**Note:** 3-letter airport codes (like SFO, LAX, JFK, RDU) are automatically converted to 4-letter ICAO codes (KSFO, KLAX, KJFK, KRDU).\n" +
		"Information includes flight callsign, airline, departure time, destination, and flight duration when available."

	return ch.messageService.SendEphemeralResponse(args, helpText)
}

// CommandParser implementation
type CommandParser struct{}

func (cp *CommandParser) ParseDeparturesCommand(commandFields []string) (*DeparturesArgs, error) {
	if len(commandFields) < 2 {
		return nil, fmt.Errorf("insufficient arguments")
	}

	args := &DeparturesArgs{}

	// Check if using flag syntax or simple syntax
	if len(commandFields) >= 3 && !strings.HasPrefix(commandFields[2], "--") {
		// Simple syntax: /flights departures <airport>
		args.Airport = strings.ToUpper(commandFields[2])
	} else {
		// Flag syntax: /flights departures --airport <airport>
		err := cp.parseDeparturesFlagSyntax(commandFields[2:], args)
		if err != nil {
			return nil, err
		}
	}

	if args.Airport == "" {
		return nil, fmt.Errorf("missing required parameter: airport code")
	}

	return args, nil
}

func (cp *CommandParser) ParseSubscribeCommand(commandFields []string) (*SubscribeArgs, error) {
	if len(commandFields) < 3 {
		return nil, fmt.Errorf("insufficient arguments")
	}

	args := &SubscribeArgs{
		FrequencyStr: "3600", // Default to 1 hour
	}

	// Check if using flag syntax or simple syntax
	if len(commandFields) >= 3 && !strings.HasPrefix(commandFields[2], "--") {
		// Simple syntax: /flights subscribe <airport> [frequency]
		args.Airport = strings.ToUpper(commandFields[2])
		if len(commandFields) >= 4 {
			args.FrequencyStr = commandFields[3]
		}
	} else {
		// Flag syntax: /flights subscribe --airport <airport> --frequency <frequency>
		err := cp.parseSubscribeFlagSyntax(commandFields[2:], args)
		if err != nil {
			return nil, err
		}
	}

	if args.Airport == "" {
		return nil, fmt.Errorf("missing required parameter: airport code")
	}

	// Parse frequency
	err := cp.parseFrequency(args)
	if err != nil {
		return nil, err
	}

	// Validate minimum frequency (5 minutes = 300 seconds)
	if args.UpdateFrequency < 300 {
		return nil, fmt.Errorf("update frequency must be at least 300 seconds (5 minutes)")
	}

	return args, nil
}

func (cp *CommandParser) ParseUnsubscribeCommand(commandFields []string) (*UnsubscribeArgs, error) {
	args := &UnsubscribeArgs{}

	// If no arguments provided, return empty args (will show list of subscriptions)
	if len(commandFields) < 3 {
		return args, nil
	}

	// Check if using flag syntax or simple syntax
	if len(commandFields) >= 3 && !strings.HasPrefix(commandFields[2], "--") {
		// Simple syntax: /flights unsubscribe <subscription_id>
		args.SubscriptionID = commandFields[2]
	} else {
		// Flag syntax: /flights unsubscribe --id <subscription_id>
		err := cp.parseUnsubscribeFlagSyntax(commandFields[2:], args)
		if err != nil {
			return nil, err
		}
	}

	return args, nil
}

// parseFlags is a generic flag parser that maps flag names to target string pointers
func (cp *CommandParser) parseFlags(fields []string, flagMap map[string]*string) {
	for i := 0; i < len(fields); i++ {
		if target, exists := flagMap[fields[i]]; exists && i+1 < len(fields) {
			*target = fields[i+1]
			i++ // Skip the flag value
		}
	}
}

func (cp *CommandParser) parseDeparturesFlagSyntax(fields []string, args *DeparturesArgs) error {
	var airport string
	flagMap := map[string]*string{
		"--airport": &airport,
	}
	cp.parseFlags(fields, flagMap)

	if airport != "" {
		args.Airport = strings.ToUpper(airport)
	}
	return nil
}

func (cp *CommandParser) parseSubscribeFlagSyntax(fields []string, args *SubscribeArgs) error {
	var airport string
	flagMap := map[string]*string{
		"--airport":   &airport,
		"--frequency": &args.FrequencyStr,
	}
	cp.parseFlags(fields, flagMap)

	if airport != "" {
		args.Airport = strings.ToUpper(airport)
	}
	return nil
}

func (cp *CommandParser) parseUnsubscribeFlagSyntax(fields []string, args *UnsubscribeArgs) error {
	flagMap := map[string]*string{
		"--id": &args.SubscriptionID,
	}
	cp.parseFlags(fields, flagMap)
	return nil
}

func (cp *CommandParser) parseFrequency(args *SubscribeArgs) error {
	// Parse as seconds
	updateFrequency, err := strconv.ParseInt(args.FrequencyStr, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid frequency: %s. Please use seconds (e.g., 3600 for 1 hour)", args.FrequencyStr)
	}

	args.UpdateFrequency = updateFrequency
	return nil
}
