package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/pluginapi"
)

type SubscriptionCommand struct {
	client              *pluginapi.Client
	subscriptionManager *SubscriptionManager
	messageService      *MessageService
	parser              *CommandParser
}

func NewSubscriptionCommand(client *pluginapi.Client, subscriptionManager *SubscriptionManager, messageService *MessageService, parser *CommandParser) *SubscriptionCommand {
	return &SubscriptionCommand{
		client:              client,
		subscriptionManager: subscriptionManager,
		messageService:      messageService,
		parser:              parser,
	}
}

func (sc *SubscriptionCommand) ExecuteList(args *model.CommandArgs, showAll bool) (*model.CommandResponse, error) {
	var subs []*Subscription
	var title string

	if showAll {
		subs = sc.subscriptionManager.GetAllSubscriptions()
		title = "**All Weather Subscriptions on Server:**"
	} else {
		subs = sc.subscriptionManager.GetSubscriptionsForChannel(args.ChannelId)
		title = "**Active Weather Subscriptions in this Channel:**"
	}

	if len(subs) == 0 {
		message := "No active weather subscriptions found"
		if showAll {
			message += " on this server."
		} else {
			message += " in this channel."
		}
		return sc.messageService.SendEphemeralResponse(args, message)
	}

	var subList strings.Builder
	subList.WriteString(title + "\n\n")

	if showAll {
		subList.WriteString("| ID | Location | Channel | Frequency | Last Updated |\n")
		subList.WriteString("|---|---------|---------|-----------|-------------|\n")

		for _, sub := range subs {
			channelName := sc.getChannelName(sub.ChannelID)
			subList.WriteString(fmt.Sprintf("| `%s` | %s | %s | %d ms | %s |\n",
				sub.ID, sub.Location, channelName, sub.UpdateFrequency, sub.LastUpdated.Format(time.RFC1123)))
		}
	} else {
		subList.WriteString("| ID | Location | Frequency | Last Updated |\n")
		subList.WriteString("|---|---------|-----------|-------------|\n")

		for _, sub := range subs {
			subList.WriteString(fmt.Sprintf("| `%s` | %s | %d ms | %s |\n",
				sub.ID, sub.Location, sub.UpdateFrequency, sub.LastUpdated.Format(time.RFC1123)))
		}
	}

	subList.WriteString("\nTo unsubscribe, use: `/weather unsubscribe SUBSCRIPTION_ID`")

	return sc.messageService.SendEphemeralResponse(args, subList.String())
}

func (sc *SubscriptionCommand) ExecuteSubscribe(args *model.CommandArgs, commandFields []string) (*model.CommandResponse, error) {
	subscribeArgs, err := sc.parser.ParseSubscribeCommand(commandFields)
	if err != nil {
		usageMsg := "Usage: `/weather subscribe --location <location> --frequency <frequency>` or `/weather subscribe <location> <frequency>`. Example: `/weather subscribe --location \"New York\" --frequency 1h`"
		return sc.messageService.SendEphemeralResponse(args, usageMsg)
	}

	// Create subscription
	subID := fmt.Sprintf("sub_%d", time.Now().UnixNano())
	subscription := &Subscription{
		ID:              subID,
		Location:        subscribeArgs.Location,
		ChannelID:       args.ChannelId,
		UserID:          args.UserId,
		UpdateFrequency: subscribeArgs.UpdateFrequency,
		LastUpdated:     time.Now(),
	}

	sc.subscriptionManager.AddSubscription(subscription)

	// Start the subscription goroutine
	go sc.subscriptionManager.StartSubscription(subscription)

	confirmationMsg := fmt.Sprintf("✅ Subscribed to weather updates for **%s**. Updates will be sent every %d ms (ID: `%s`).", 
		subscribeArgs.Location, subscribeArgs.UpdateFrequency, subID)
	
	return sc.messageService.SendEphemeralResponse(args, confirmationMsg)
}

func (sc *SubscriptionCommand) ExecuteUnsubscribe(args *model.CommandArgs, subscriptionID string) (*model.CommandResponse, error) {
	if subscriptionID == "" {
		return sc.messageService.SendEphemeralResponse(args, "Usage: `/weather unsubscribe <subscription_id>`. Use `/weather list` to see your subscriptions.")
	}

	if sub, exists := sc.subscriptionManager.GetSubscription(subscriptionID); exists {
		location := sub.Location
		if sc.subscriptionManager.RemoveSubscription(subscriptionID) {
			message := fmt.Sprintf("✅ Unsubscribed from weather updates for **%s** (ID: `%s`).", location, subscriptionID)
			return sc.messageService.SendEphemeralResponse(args, message)
		}
	}

	message := fmt.Sprintf("No subscription found with ID: %s", subscriptionID)
	return sc.messageService.SendEphemeralResponse(args, message)
}

func (sc *SubscriptionCommand) getChannelName(channelID string) string {
	channel, err := sc.client.Channel.Get(channelID)
	if err != nil {
		return channelID // fallback to ID
	}
	
	channelName := channel.DisplayName
	if channelName == "" {
		channelName = channel.Name
	}
	return channelName
}