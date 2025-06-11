package main

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/pluginapi"
)

type CommandHandler struct {
	client              *pluginapi.Client
	weatherService      *WeatherService
	subscriptionManager *SubscriptionManager
	botUserID           string
}

func NewCommandHandler(client *pluginapi.Client, weatherService *WeatherService, subscriptionManager *SubscriptionManager, botUserID string) *CommandHandler {
	handler := &CommandHandler{
		client:              client,
		weatherService:      weatherService,
		subscriptionManager: subscriptionManager,
		botUserID:           botUserID,
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
		return ch.executeHelpCommand(args)
	}

	command := split[1]
	switch command {
	case "help", "--help":
		return ch.executeHelpCommand(args)
	case "list":
		return ch.executeListCommand(args)
	case "subscribe":
		return ch.executeSubscribeCommand(args)
	case "unsubscribe":
		return ch.executeUnsubscribeCommand(args)
	default:
		// Treat as location for regular weather request
		location := strings.Join(split[1:], " ")
		return ch.executeWeatherCommand(args, location)
	}
}

// postBotResponse posts a message from the bot and returns an empty ephemeral response
func (ch *CommandHandler) commandResponsePost(post *model.Post, userID string, isEphemeral bool) (*model.CommandResponse, error) {
	ch.sendBotPost(post, userID, isEphemeral)

	return &model.CommandResponse{
		ResponseType: model.CommandResponseTypeEphemeral,
		Text:         "",
	}, nil
}

// postBotMessage posts a message from the bot
func (ch *CommandHandler) sendBotPost(post *model.Post, userID string, isEphemeral bool) *model.Post {
	post.UserId = ch.botUserID

	if isEphemeral {
		ch.client.Post.SendEphemeralPost(userID, post)
		return post
	}

	ch.client.Post.CreatePost(post)
	return post
}

// postWeatherMessage posts a weather message with rich formatting using attachments
func (ch *CommandHandler) formatWeatherMessage(userID, channelID string, weatherData *WeatherResponse) *model.Post {
	locationDisplay := weatherData.Location.Name
	if locationDisplay == "" {
		locationDisplay = fmt.Sprintf("%.2f,%.2f", weatherData.Location.Lat, weatherData.Location.Lon)
	}

	description, exists := WeatherCodeDescription[weatherData.Data.Values.WeatherCode]
	if !exists {
		description = "Unknown"
	}

	var windDirection string
	windDir := weatherData.Data.Values.WindDirection
	directions := []string{"N", "NNE", "NE", "ENE", "E", "ESE", "SE", "SSE", "S", "SSW", "SW", "WSW", "W", "WNW", "NW", "NNW"}
	index := int((float64(windDir)+11.25)/22.5) % 16
	windDirection = directions[index]

	// Create fields for the attachment
	fields := []*model.SlackAttachmentField{
		{
			Title: "Temperature",
			Value: fmt.Sprintf("%.1f°C (feels like %.1f°C)", weatherData.Data.Values.Temperature, weatherData.Data.Values.TemperatureApparent),
			Short: true,
		},
		{
			Title: "Condition",
			Value: description,
			Short: true,
		},
		{
			Title: "Humidity",
			Value: fmt.Sprintf("%d%%", weatherData.Data.Values.Humidity),
			Short: true,
		},
		{
			Title: "Wind",
			Value: fmt.Sprintf("%.1f km/h %s", weatherData.Data.Values.WindSpeed, windDirection),
			Short: true,
		},
		{
			Title: "Cloud Cover",
			Value: fmt.Sprintf("%d%%", weatherData.Data.Values.CloudCover),
			Short: true,
		},
		{
			Title: "Precipitation Chance",
			Value: fmt.Sprintf("%d%%", weatherData.Data.Values.PrecipitationProbability),
			Short: true,
		},
	}

	if weatherData.Data.Values.RainIntensity > 0 {
		fields = append(fields, &model.SlackAttachmentField{
			Title: "Rain Intensity",
			Value: fmt.Sprintf("%.1f mm/h", weatherData.Data.Values.RainIntensity),
			Short: true,
		})
	}

	if weatherData.Data.Values.WindGust > 0 {
		fields = append(fields, &model.SlackAttachmentField{
			Title: "Wind Gusts",
			Value: fmt.Sprintf("%.1f km/h", weatherData.Data.Values.WindGust),
			Short: true,
		})
	}

	// Determine color based on weather condition
	color := "#36a64f" // Default green
	switch {
	case weatherData.Data.Values.WeatherCode >= 8000: // Thunderstorm
		color = "#ff4444"
	case weatherData.Data.Values.WeatherCode >= 6000: // Freezing rain/ice
		color = "#8844ff"
	case weatherData.Data.Values.WeatherCode >= 5000: // Snow
		color = "#4488ff"
	case weatherData.Data.Values.WeatherCode >= 4000: // Rain
		color = "#4488aa"
	case weatherData.Data.Values.WeatherCode >= 2000: // Fog
		color = "#888888"
	case weatherData.Data.Values.WeatherCode >= 1100: // Cloudy
		color = "#aaaaaa"
	}

	attachment := &model.SlackAttachment{
		Title:     fmt.Sprintf("🌤️ Weather for %s", locationDisplay),
		Color:     color,
		Fields:    fields,
		Footer:    "Weather Bot",
		Timestamp: time.Now().Unix(),
	}

	post := &model.Post{
		ChannelId: channelID,
		UserId:    ch.botUserID,
		Props: map[string]any{
			"attachments": []*model.SlackAttachment{attachment},
		},
	}

	return post
}

func (ch *CommandHandler) executeHelpCommand(args *model.CommandArgs) (*model.CommandResponse, error) {
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

	post := &model.Post{
		ChannelId: args.ChannelId,
		Message:   helpText,
	}

	return ch.commandResponsePost(post, args.UserId, true)

}

func (ch *CommandHandler) executeWeatherCommand(args *model.CommandArgs, location string) (*model.CommandResponse, error) {
	if location == "" {
		return ch.commandResponsePost(&model.Post{
			ChannelId: args.ChannelId,
			Message:   "Please provide a location. Example: `/weather New York` or use `/weather help` for more commands.",
		}, args.UserId, true)
	}

	weatherData, err := ch.weatherService.GetWeatherData(location)
	if err != nil {
		ch.client.Log.Error("Error fetching weather data", "error", err, "location", location)
		return ch.commandResponsePost(&model.Post{
			ChannelId: args.ChannelId,
			Message:   fmt.Sprintf("Error fetching weather data: %v", err),
		}, args.UserId, true)
	}

	post := ch.formatWeatherMessage(args.UserId, args.ChannelId, weatherData)

	return ch.commandResponsePost(post, args.UserId, false)
}

func (ch *CommandHandler) executeListCommand(args *model.CommandArgs) (*model.CommandResponse, error) {
	split := strings.Fields(args.Command)
	showAll := len(split) > 2 && split[2] == "--all"

	var subs []*Subscription
	var title string

	if showAll {
		subs = ch.subscriptionManager.GetAllSubscriptions()
		title = "**All Weather Subscriptions on Server:**"
	} else {
		subs = ch.subscriptionManager.GetSubscriptionsForChannel(args.ChannelId)
		title = "**Active Weather Subscriptions in this Channel:**"
	}

	if len(subs) == 0 {
		message := "No active weather subscriptions found"
		if showAll {
			message += " on this server."
		} else {
			message += " in this channel."
		}

		return ch.commandResponsePost(&model.Post{
			ChannelId: args.ChannelId,
			Message:   message,
		}, args.UserId, true)
	}

	var subList strings.Builder
	subList.WriteString(title + "\n\n")

	if showAll {
		subList.WriteString("| ID | Location | Channel | Frequency | Last Updated |\n")
		subList.WriteString("|---|---------|---------|-----------|-------------|\n")

		for _, sub := range subs {
			// Get channel name for display
			channel, err := ch.client.Channel.Get(sub.ChannelID)
			channelName := sub.ChannelID // fallback to ID
			if err == nil {
				channelName = channel.DisplayName
				if channelName == "" {
					channelName = channel.Name
				}
			}

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

	return ch.commandResponsePost(&model.Post{
		ChannelId: args.ChannelId,
		Message:   subList.String(),
	}, args.UserId, true)
}

func (ch *CommandHandler) executeSubscribeCommand(args *model.CommandArgs) (*model.CommandResponse, error) {
	split := strings.Fields(args.Command)

	var location string
	var frequencyStr string

	// Check if using flag syntax or simple syntax
	if len(split) >= 4 && !strings.HasPrefix(split[2], "--") {
		// Simple syntax: /weather subscribe <location> <frequency>
		location = split[2]
		frequencyStr = split[3]
	} else {
		// Flag syntax: /weather subscribe --location <location> --frequency <frequency>
		for i := 2; i < len(split); i++ {
			switch split[i] {
			case "--location":
				if i+1 < len(split) {
					// Collect all words until next flag or end
					locationParts := []string{}
					j := i + 1
					for ; j < len(split) && !strings.HasPrefix(split[j], "--"); j++ {
						locationParts = append(locationParts, split[j])
					}
					location = strings.Join(locationParts, " ")
					i = j - 1 // Skip processed words
				}
			case "--frequency":
				if i+1 < len(split) {
					frequencyStr = split[i+1]
					i++ // Skip the frequency value
				}
			}
		}
	}

	// Check if we have both required parameters
	if location == "" || frequencyStr == "" {
		return ch.commandResponsePost(&model.Post{
			ChannelId: args.ChannelId,
			Message:   "Usage: `/weather subscribe --location <location> --frequency <frequency>` or `/weather subscribe <location> <frequency>`. Example: `/weather subscribe --location \"New York\" --frequency 1h`",
		}, args.UserId, true)

	}

	var updateFrequency int64
	var err error

	// Try to parse as milliseconds first
	updateFrequency, err = strconv.ParseInt(frequencyStr, 10, 64)
	if err != nil {
		// Try to parse as duration
		duration, err := time.ParseDuration(frequencyStr)
		if err != nil {
			return ch.commandResponsePost(&model.Post{
				ChannelId: args.ChannelId,
				Message:   fmt.Sprintf("Invalid frequency: %s. Please use milliseconds (e.g., 60000 for 1 minute) or a valid duration like 30s, 5m, 1h", frequencyStr),
			}, args.UserId, true)
		}
		updateFrequency = duration.Milliseconds()
	}

	// Validate minimum frequency (30 seconds)
	if updateFrequency < 30000 {
		return ch.commandResponsePost(&model.Post{
			ChannelId: args.ChannelId,
			Message:   "Update frequency must be at least 30000 milliseconds (30 seconds).",
		}, args.UserId, true)
	}

	// Create subscription
	subID := fmt.Sprintf("sub_%d", time.Now().UnixNano())
	subscription := &Subscription{
		ID:              subID,
		Location:        location,
		ChannelID:       args.ChannelId,
		UserID:          args.UserId,
		UpdateFrequency: updateFrequency,
		LastUpdated:     time.Now(),
		StopChan:        make(chan struct{}),
	}

	ch.subscriptionManager.AddSubscription(subscription)

	// Start the subscription goroutine
	go ch.startSubscription(subscription)

	confirmationMsg := fmt.Sprintf("✅ Subscribed to weather updates for **%s**. Updates will be sent every %d ms (ID: `%s`).", location, updateFrequency, subID)
	post := &model.Post{
		ChannelId: args.ChannelId,
		Message:   confirmationMsg,
	}
	return ch.commandResponsePost(post, args.UserId, false)
}

func (ch *CommandHandler) executeUnsubscribeCommand(args *model.CommandArgs) (*model.CommandResponse, error) {
	split := strings.Fields(args.Command)
	post := &model.Post{
		ChannelId: args.ChannelId,
	}
	if len(split) < 3 {
		post.Message = "Usage: `/weather unsubscribe <subscription_id>`. Use `/weather list` to see your subscriptions."
		return ch.commandResponsePost(post, args.UserId, true)
	}

	subscriptionID := split[2]

	if sub, exists := ch.subscriptionManager.GetSubscription(subscriptionID); exists {
		location := sub.Location
		if ch.subscriptionManager.RemoveSubscription(subscriptionID) {
			post.Message = fmt.Sprintf("✅ Unsubscribed from weather updates for **%s** (ID: `%s`).", location, subscriptionID)
			return ch.commandResponsePost(post, args.UserId, false)
		}
	}

	post.Message = fmt.Sprintf("No subscription found with ID: %s", subscriptionID)
	return ch.commandResponsePost(post, args.UserId, false)

}

func (ch *CommandHandler) startSubscription(sub *Subscription) {
	// Get initial weather data
	weatherData, err := ch.weatherService.GetWeatherData(sub.Location)
	post := &model.Post{
		ChannelId: sub.ChannelID,
	}
	if err != nil {
		ch.client.Log.Error("Error fetching initial weather data for subscription", "error", err, "subscription_id", sub.ID)
		post.Message = fmt.Sprintf(
			"⚠️ Could not fetch weather data for subscription to **%s** (ID: `%s`): %v",
			sub.Location, sub.ID, err)
		ch.commandResponsePost(post, sub.UserID, true)
	} else {
		post := ch.formatWeatherMessage(sub.UserID, sub.ChannelID, weatherData)

		ch.commandResponsePost(post, sub.UserID, false)
	}

	ticker := time.NewTicker(time.Duration(sub.UpdateFrequency) * time.Millisecond)
	defer ticker.Stop()

	consecutiveFailures := 0
	maxConsecutiveFailures := 5

	for {
		select {
		case <-ticker.C:
			weatherData, err := ch.weatherService.GetWeatherData(sub.Location)

			if err != nil {
				consecutiveFailures++
				ch.client.Log.Error("Error fetching weather data for subscription", "error", err, "subscription_id", sub.ID, "failures", consecutiveFailures)

				if consecutiveFailures == 1 || consecutiveFailures == maxConsecutiveFailures {
					post := &model.Post{
						ChannelId: sub.ChannelID,
					}
					post.Message = fmt.Sprintf("⚠️ Error updating weather for **%s**: %v", sub.Location, err)
					ch.commandResponsePost(post, sub.UserID, true)
				}

				if consecutiveFailures >= maxConsecutiveFailures {
					ticker.Reset(time.Duration(sub.UpdateFrequency*2) * time.Millisecond)
				}
				continue
			}

			if consecutiveFailures > 0 {
				ch.client.Log.Info("Successfully recovered subscription after failures", "subscription_id", sub.ID, "failures", consecutiveFailures)
				consecutiveFailures = 0
				ticker.Reset(time.Duration(sub.UpdateFrequency) * time.Millisecond)
			}

			post = ch.formatWeatherMessage(sub.UserID, sub.ChannelID, weatherData)

			ch.commandResponsePost(post, sub.UserID, false)

			sub.LastUpdated = time.Now()

		case <-sub.StopChan:
			ch.client.Log.Info("Stopping subscription", "subscription_id", sub.ID)
			return
		}
	}
}
