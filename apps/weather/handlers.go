package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// handleWeatherRequest handles the /weather endpoint
func handleWeatherRequest(w http.ResponseWriter, r *http.Request, apiKey string) {
	// Get location from query parameter
	location := r.URL.Query().Get("location")
	if location == "" {
		http.Error(w, "Location parameter is required", http.StatusBadRequest)
		return
	}

	// Get weather data
	weatherData, err := getWeatherData(location, apiKey)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error fetching weather data: %v", err), http.StatusInternalServerError)
		return
	}

	// Set response headers
	w.Header().Set("Content-Type", "application/json")

	// Return the weather data
	json.NewEncoder(w).Encode(weatherData)
}

// handleHealthCheck handles the /health endpoint
func handleHealthCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// handleAPIUsage handles the /api-usage endpoint
func handleAPIUsage(w http.ResponseWriter, r *http.Request, subscriptionManager *SubscriptionManager) {
	w.Header().Set("Content-Type", "application/json")

	hourlyUsage, dailyUsage := subscriptionManager.CalculateAPIUsage()

	usage := map[string]interface{}{
		"hourly_usage":       hourlyUsage,
		"daily_usage":        dailyUsage,
		"hourly_limit":       subscriptionManager.HourlyLimit,
		"daily_limit":        subscriptionManager.DailyLimit,
		"hourly_remaining":   subscriptionManager.HourlyLimit - hourlyUsage,
		"daily_remaining":    subscriptionManager.DailyLimit - dailyUsage,
		"subscription_count": len(subscriptionManager.Subscriptions),
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(usage)
}

// handleBotImage handles the /bot.png endpoint
func handleBotImage(w http.ResponseWriter, r *http.Request, staticDir string) {
	// Path to the bot.png file
	botImagePath := filepath.Join(staticDir, "bot.png")

	// Check if the file exists
	if _, err := os.Stat(botImagePath); os.IsNotExist(err) {
		log.Printf("bot.png not found at %s", botImagePath)
		http.Error(w, "Image not found", http.StatusNotFound)
		return
	}

	// Set the content type
	w.Header().Set("Content-Type", "image/png")

	// Serve the file
	http.ServeFile(w, r, botImagePath)
}

// handleIncomingWebhook handles the /incoming endpoint for Mattermost webhooks
func handleIncomingWebhook(w http.ResponseWriter, r *http.Request, apiKey string, subscriptionManager *SubscriptionManager) {
	// Set content type header early
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		log.Printf("Method not allowed: %s", r.Method)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Read the request body for logging
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("Error reading request body: %v", err)
		http.Error(w, "Error reading request body", http.StatusBadRequest)
		return
	}

	// Log the raw request body
	log.Printf("Received webhook payload: %s", string(bodyBytes))

	// Create a new reader with the same body data for parsing the form
	r.Body = io.NopCloser(bytes.NewReader(bodyBytes))

	// Parse the form data
	if err := r.ParseForm(); err != nil {
		log.Printf("Error parsing form data: %v", err)
		http.Error(w, "Error parsing form data", http.StatusBadRequest)
		return
	}

	// Log all form values for debugging
	log.Printf("Parsed form data:")
	for key, values := range r.Form {
		log.Printf("  %s: %v", key, values)
	}

	// Extract user information
	userID := r.FormValue("user_id")
	userName := r.FormValue("user_name")
	channelName := r.FormValue("channel_name")
	channelID := r.FormValue("channel_id")
	text := r.FormValue("text")

	log.Printf("Processing weather request from user: %s (%s) in channel: %s (%s) with text: %s",
		userName, userID, channelName, channelID, text)

	// Check for simple commands first
	if text == "--help" || text == "help" {
		sendHelpResponse(w, channelID)
		return
	} else if text == "limits" {
		sendLimitsResponse(w, channelID, subscriptionManager)
		return
	} else if text == "list" {
		sendListResponse(w, channelID, userID, subscriptionManager)
		return
	}

	// Parse command flags
	var location string
	var subscribe bool
	var unsubscribe bool
	var updateFrequency int64
	var subscriptionID string
	var showLimits bool

	// Split text into words
	words := strings.Fields(text)
	for i := 0; i < len(words); i++ {
		switch words[i] {
		case "--location":
			if i+1 < len(words) {
				// Collect all words until next flag or end
				locationParts := []string{}
				j := i + 1
				for ; j < len(words) && !strings.HasPrefix(words[j], "--"); j++ {
					locationParts = append(locationParts, words[j])
				}
				location = strings.Join(locationParts, " ")
				i = j - 1 // Skip processed words
			}
		case "--subscribe":
			subscribe = true
		case "--unsubscribe":
			unsubscribe = true
		case "--update-frequency":
			if i+1 < len(words) {
				// Try to parse as milliseconds directly
				msValue, err := strconv.ParseInt(words[i+1], 10, 64)
				if err != nil {
					// If not a direct number, try to parse as duration and convert to milliseconds
					duration, err := time.ParseDuration(words[i+1])
					if err != nil {
						log.Printf("Invalid update frequency: %s", words[i+1])
						response := MattermostResponse{
							Text:         fmt.Sprintf("Invalid update frequency: %s. Please use milliseconds (e.g., 60000 for 1 minute) or a valid duration like 30s, 5m, 1h", words[i+1]),
							ResponseType: "ephemeral",
							ChannelID:    channelID,
						}
						json.NewEncoder(w).Encode(response)
						return
					}
					// Convert duration to milliseconds
					updateFrequency = duration.Milliseconds()
				} else {
					updateFrequency = msValue
				}
				i++ // Skip the frequency value
			}
		case "--id":
			if i+1 < len(words) {
				subscriptionID = words[i+1]
				i++ // Skip the ID value
			}
		case "--limits":
			showLimits = true
		}
	}

	// If no flags were used, treat the entire text as location
	// But don't treat special commands as a location
	if location == "" && !strings.Contains(text, "--") && text != "help" && text != "list" {
		location = text
	}

	// Handle limits request (for backward compatibility with --limits flag)
	if showLimits {
		sendLimitsResponse(w, channelID, subscriptionManager)
		return
	}

	// Handle unsubscribe request
	if unsubscribe {
		handleUnsubscribe(w, channelID, subscriptionID, subscriptionManager)
		return
	}

	// Check if location is provided for regular weather or subscription
	if location == "" && (subscribe || !unsubscribe) {
		log.Printf("No location provided, sending help message")
		response := MattermostResponse{
			Text:         "Please provide a location. Example: `/weather New York` or `/weather --location London, UK --subscribe --update-frequency 30m`\n\nUse `/weather help` for a complete list of commands.",
			ResponseType: "ephemeral",
			ChannelID:    channelID,
		}
		json.NewEncoder(w).Encode(response)
		return
	}

	// Handle subscription request
	if subscribe {
		handleSubscribe(w, channelID, userID, location, updateFrequency, apiKey, subscriptionManager)
		return
	}

	// Handle regular weather request
	handleRegularWeatherRequest(w, channelID, location, apiKey)
}

// sendHelpResponse sends the help text as an ephemeral message
func sendHelpResponse(w http.ResponseWriter, channelID string) {
	helpText := "**Weather Bot Commands**\n\n" +
		"**Basic Commands:**\n" +
		"- `/weather <location>` - Get current weather for a location\n" +
		"- `/weather --help` - Show this help message\n" +
		"- `/weather limits` - Show API usage limits and current usage\n" +
		"- `/weather list` - List your active subscriptions\n\n" +

		"**Subscription Commands:**\n" +
		"- `/weather --subscribe --location <location> --update-frequency <time>` - Subscribe to weather updates\n" +
		"- `/weather --unsubscribe --id <subscription_id>` - Unsubscribe from specific weather updates\n\n" +

		"**Parameters:**\n" +
		"- `location` - City name, zip code, or coordinates (e.g., 'New York', '10001', '40.7128,-74.0060')\n" +
		"- `update-frequency` - How often to send updates in milliseconds (e.g., 3600000 for hourly) or duration (e.g., 1h, 30m)\n" +
		"- `subscription_id` - ID of an active subscription\n\n" +

		"**Examples:**\n" +
		"- `/weather London` - Get current weather for London\n" +
		"- `/weather list` - Show your active subscriptions\n" +
		"- `/weather --subscribe --location Tokyo --update-frequency 3600000` - Get hourly weather updates for Tokyo\n" +
		"- `/weather --subscribe --location \"San Francisco\" --update-frequency 1h` - Get hourly weather updates for San Francisco"

	response := MattermostResponse{
		Text:         helpText,
		ResponseType: "ephemeral",
		ChannelID:    channelID,
	}
	json.NewEncoder(w).Encode(response)
}

// sendLimitsResponse sends the API usage limits as an ephemeral message
func sendLimitsResponse(w http.ResponseWriter, channelID string, subscriptionManager *SubscriptionManager) {
	hourlyUsage, dailyUsage := subscriptionManager.CalculateAPIUsage()

	limitsText := fmt.Sprintf("**Weather API Usage Limits**\n\n"+
		"**Current Usage:**\n"+
		"- Hourly: %d/%d requests (%d%% used)\n"+
		"- Daily: %d/%d requests (%d%% used)\n\n"+
		"**Active Subscriptions:** %d\n\n"+
		"Use `/weather --subscribe --location <location> --update-frequency <ms>` to create a new subscription.",
		hourlyUsage, subscriptionManager.HourlyLimit, (hourlyUsage*100)/subscriptionManager.HourlyLimit,
		dailyUsage, subscriptionManager.DailyLimit, (dailyUsage*100)/subscriptionManager.DailyLimit,
		len(subscriptionManager.Subscriptions))

	response := MattermostResponse{
		Text:         limitsText,
		ResponseType: "ephemeral",
		ChannelID:    channelID,
	}
	json.NewEncoder(w).Encode(response)
}

// sendListResponse sends the list of user's subscriptions as an ephemeral message
func sendListResponse(w http.ResponseWriter, channelID string, userID string, subscriptionManager *SubscriptionManager) {
	// List subscriptions for the user
	subs := subscriptionManager.GetSubscriptionsForUser(userID)
	if len(subs) == 0 {
		response := MattermostResponse{
			Text:         "You don't have any active weather subscriptions.",
			ResponseType: "ephemeral",
			ChannelID:    channelID,
		}
		json.NewEncoder(w).Encode(response)
		return
	}

	// Build list of subscriptions
	var subList strings.Builder
	subList.WriteString("Your active weather subscriptions:\n\n")
	for _, sub := range subs {
		subList.WriteString(fmt.Sprintf("ID: `%s`\nLocation: %s\nFrequency: %d ms\n\n",
			sub.ID, sub.Location, sub.UpdateFrequency))
	}
	subList.WriteString("To unsubscribe, use: `/weather --unsubscribe --id SUBSCRIPTION_ID`")

	response := MattermostResponse{
		Text:         subList.String(),
		ResponseType: "ephemeral",
		ChannelID:    channelID,
	}
	json.NewEncoder(w).Encode(response)
}

// handleUnsubscribe handles the unsubscribe command
func handleUnsubscribe(w http.ResponseWriter, channelID string, subscriptionID string, subscriptionManager *SubscriptionManager) {
	if subscriptionID == "" {
		// For backward compatibility, show the list of subscriptions
		response := MattermostResponse{
			Text:         "Please use `/weather list` to see your subscriptions or specify an ID with `--id` to unsubscribe.",
			ResponseType: "ephemeral",
			ChannelID:    channelID,
		}
		json.NewEncoder(w).Encode(response)
		return
	}

	// Remove the subscription
	if subscriptionManager.RemoveSubscription(subscriptionID) {
		response := MattermostResponse{
			Text:         fmt.Sprintf("Successfully unsubscribed from weather updates for subscription ID: %s", subscriptionID),
			ResponseType: "ephemeral",
			ChannelID:    channelID,
		}
		json.NewEncoder(w).Encode(response)
	} else {
		response := MattermostResponse{
			Text:         fmt.Sprintf("No subscription found with ID: %s", subscriptionID),
			ResponseType: "ephemeral",
			ChannelID:    channelID,
		}
		json.NewEncoder(w).Encode(response)
	}
}

// handleSubscribe handles the subscribe command
func handleSubscribe(w http.ResponseWriter, channelID string, userID string, location string, updateFrequency int64, apiKey string, subscriptionManager *SubscriptionManager) {
	// Set default update frequency if not provided
	if updateFrequency == 0 {
		updateFrequency = 3600000 // Default to hourly updates (3600000 ms = 1 hour)
	}

	// Validate minimum update frequency (30 seconds = 30000 milliseconds)
	if updateFrequency < 30000 {
		response := MattermostResponse{
			Text:         "Update frequency must be at least 30000 milliseconds (30 seconds).",
			ResponseType: "ephemeral",
			ChannelID:    channelID,
		}
		json.NewEncoder(w).Encode(response)
		return
	}

	// Check if adding this subscription would exceed API limits
	withinLimits, limitMessage := subscriptionManager.CheckSubscriptionLimits(updateFrequency)
	if !withinLimits {
		response := MattermostResponse{
			Text:         limitMessage,
			ResponseType: "ephemeral",
			ChannelID:    channelID,
		}
		json.NewEncoder(w).Encode(response)
		return
	}

	// Create a unique ID for the subscription
	subID := fmt.Sprintf("sub_%d", time.Now().UnixNano())

	// Create the subscription
	subscription := &Subscription{
		ID:              subID,
		Location:        location,
		ChannelID:       channelID,
		UserID:          userID,
		UpdateFrequency: updateFrequency,
		LastUpdated:     time.Now(),
		StopChan:        make(chan struct{}),
	}

	// Add the subscription
	subscriptionManager.AddSubscription(subscription)
	log.Printf("Added new subscription with ID: %s", subID)

	// Start a goroutine to handle the subscription
	go startSubscription(subscription, apiKey, subscriptionManager)

	// Send confirmation
	response := MattermostResponse{
		Text:         fmt.Sprintf("Successfully subscribed to weather updates for %s every %d ms. Subscription ID: `%s`", location, updateFrequency, subID),
		ResponseType: "ephemeral",
		ChannelID:    channelID,
	}
	json.NewEncoder(w).Encode(response)
}


// handleRegularWeatherRequest handles a regular weather request
func handleRegularWeatherRequest(w http.ResponseWriter, channelID string, location string, apiKey string) {
	weatherData, err := getWeatherData(location, apiKey)
	if err != nil {
		log.Printf("Error fetching weather data: %v", err)
		response := MattermostResponse{
			Text:         fmt.Sprintf("Error fetching weather data: %v", err),
			ResponseType: "ephemeral",
			ChannelID:    channelID,
		}
		json.NewEncoder(w).Encode(response)
		return
	}

	// Create a formatted response
	weatherText := formatWeatherResponse(weatherData)
	response := MattermostResponse{
		Text:         weatherText,
		ResponseType: "in_channel",
		ChannelID:    channelID,
	}

	// Return the response
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Error encoding response: %v", err)
		http.Error(w, "Error encoding response", http.StatusInternalServerError)
		return
	}

	log.Printf("Successfully sent weather response to channel: %s", channelID)
}
