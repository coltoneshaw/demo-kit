package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/mattermost/mattermost/server/public/model"
)

// handleIncomingWebhook handles incoming webhooks from Mattermost
func handleIncomingWebhook(w http.ResponseWriter, r *http.Request, missionManager *MissionManager, subscriptionManager *SubscriptionManager) {
	// Set content type header
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		log.Printf("Method not allowed: %s", r.Method)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse the form data (Mattermost sends slash commands as form data)
	if err := r.ParseForm(); err != nil {
		log.Printf("Error parsing form data: %v", err)
		http.Error(w, "Invalid request format", http.StatusBadRequest)
		return
	}

	// Log the form data for debugging
	log.Printf("Received webhook form data: %v", r.Form)

	// Extract values from form data
	text := r.FormValue("text")
	channelID := r.FormValue("channel_id")
	userID := r.FormValue("user_id")
	command := r.FormValue("command")
	teamID := r.FormValue("team_id")
	teamName := r.FormValue("team_domain")

	log.Printf("Command: %s, Text: %s, ChannelID: %s, UserID: %s, TeamID: %s, TeamName: %s", command, text, channelID, userID, teamID, teamName)

	if channelID == "" {
		log.Printf("Missing channel_id field")
		http.Error(w, "Missing channel_id field", http.StatusBadRequest)
		return
	}

	// Create Mattermost client
	client, err := NewClient()
	if err != nil {
		log.Printf("Error creating Mattermost client: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Handle the different commands
	if command == "/mission" {
		// If text is empty or "help" or "--help", show help
		if text == "" || text == "help" || text == "--help" {
			sendHelpResponse(w, channelID)
			return
		}

		// Parse the command arguments
		args := parseCommand(text)

		// Check command type
		if len(args) >= 1 {
			switch args[0] {
			case "start":
				handleStartCommand(client, w, args[1:], channelID, userID, teamID, missionManager)
				return
			case "list":
				handleListCommand(client, w, channelID, missionManager)
				return
			case "status":
				handleStatusCommand(client, w, args[1:], channelID, missionManager, subscriptionManager)
				return
			case "complete":
				handleCompleteCommand(client, w, r, args[1:], channelID, userID, missionManager)
				return
			case "subscribe":
				handleSubscribeCommand(client, w, args[1:], channelID, userID, subscriptionManager)
				return
			case "unsubscribe":
				handleUnsubscribeCommand(client, w, args[1:], channelID, userID, subscriptionManager)
				return
			case "subscriptions":
				handleListSubscriptionsCommand(client, w, args[1:], channelID, subscriptionManager)
				return
			case "--help":
				sendHelpResponse(w, channelID)
				return
			}
		}

		// Unknown subcommand
		sendErrorResponse(w, channelID, fmt.Sprintf("Unknown subcommand: %s. Use `/mission help` for available commands.", text))
		return
	}

	// If we get here, it's an unknown command
	sendHelpResponse(w, channelID)
}

// handleStartCommand handles the start command to create a new mission
func handleStartCommand(c *Client, w http.ResponseWriter, args []string, channelID, userID, teamID string, missionManager *MissionManager) {
	ctx := context.Background()

	// Parse arguments
	var name, callsign, departureAirport, arrivalAirport string
	var crew []string

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--name":
			if i+1 < len(args) {
				name = args[i+1]
				i++
			}
		case "--callsign":
			if i+1 < len(args) {
				callsign = args[i+1]
				i++
			}
		case "--departureAirport":
			if i+1 < len(args) {
				departureAirport = strings.ToUpper(args[i+1])
				i++
			}
		case "--arrivalAirport":
			if i+1 < len(args) {
				arrivalAirport = strings.ToUpper(args[i+1])
				i++
			}
		case "--crew":
			// Collect all usernames until the next flag or end of args
			j := i + 1
			for j < len(args) && !strings.HasPrefix(args[j], "--") {
				crew = append(crew, args[j])
				j++
			}
			i = j - 1
		}
	}

	// Validate required parameters
	if name == "" {
		sendErrorResponse(w, channelID, "Mission name is required. Use `--name [name]`")
		return
	}

	if callsign == "" {
		sendErrorResponse(w, channelID, "Mission callsign is required. Use `--callsign [callsign]`")
		return
	}

	if departureAirport == "" {
		sendErrorResponse(w, channelID, "Departure airport is required. Use `--departureAirport [code]`")
		return
	}

	if arrivalAirport == "" {
		sendErrorResponse(w, channelID, "Arrival airport is required. Use `--arrivalAirport [code]`")
		return
	}

	// Create the Mattermost channel name (callsign-name)
	// Ensure it's lowercase and replace spaces with dashes
	channelName := strings.ToLower(fmt.Sprintf("%s-%s", callsign, name))
	channelName = strings.ReplaceAll(channelName, " ", "-")

	// Get the team by ID
	team, resp, err := c.client.GetTeam(ctx, teamID, "")
	if err != nil {
		log.Printf("Error getting team with ID %s: %v, status code: %d", teamID, err, resp.StatusCode)
		sendErrorResponse(w, channelID, fmt.Sprintf("Error getting team details: %v", err))
		return
	}

	// Get status emoji for initial status ("stalled")
	initialStatusEmoji := GetStatusEmoji("stalled")

	// Create the channel with status emoji in display name
	channel := &model.Channel{
		TeamId:      team.Id,
		Name:        channelName,
		DisplayName: fmt.Sprintf("%s %s: %s", initialStatusEmoji, callsign, name),
		Type:        model.ChannelTypeOpen,
	}

	createdChannel, resp, err := c.client.CreateChannel(ctx, channel)
	if err != nil {
		log.Printf("Error creating channel: %v, status code: %d", err, resp.StatusCode)
		sendErrorResponse(w, channelID, fmt.Sprintf("Error creating mission channel: %v", err))
		return
	}
	newChannelID := createdChannel.Id

	// Categorize the mission channel into "Active Missions" category
	if err := c.CategorizeMissionChannel(newChannelID); err != nil {
		log.Printf("Error categorizing mission channel: %v", err)
		// Continue anyway, just log the error
	}

	// Get user IDs from usernames
	userIDs := make([]string, 0, len(crew))
	for _, username := range crew {
		// Clean username - remove @ if present
		cleanUsername := username
		if len(username) > 0 && username[0] == '@' {
			cleanUsername = username[1:]
		}

		user, resp, err := c.client.GetUserByUsername(ctx, cleanUsername, "")
		if err != nil {
			log.Printf("Error getting user %s: %v, status code: %d", cleanUsername, err, resp.StatusCode)
			sendErrorResponse(w, channelID, fmt.Sprintf("Error adding crew to channel: %v", err))
			return
		}

		userIDs = append(userIDs, user.Id)
	}

	// Add all users to the channel in a single API call
	_, resp, err = c.client.AddChannelMembers(ctx, newChannelID, "", userIDs)
	if err != nil {
		log.Printf("Error adding users to channel: %v, status code: %d", err, resp.StatusCode)
		sendErrorResponse(w, channelID, fmt.Sprintf("Error adding crew to channel: %v", err))
		return
	}

	// Create the mission
	mission := &Mission{
		Name:             name,
		Callsign:         callsign,
		DepartureAirport: departureAirport,
		ArrivalAirport:   arrivalAirport,
		CreatedBy:        userID,
		CreatedAt:        time.Now(),
		Crew:             userIDs,
		ChannelID:        newChannelID,
		ChannelName:      channelName,
		Status:           "stalled", // Initial status
	}

	// Add the mission to the manager
	missionManager.AddMission(mission)

	// Send a message to the new channel with mission details
	missionDetails := fmt.Sprintf("# Mission Created: %s\n\n"+
		"**Callsign:** %s\n"+
		"**Departure:** %s\n"+
		"**Arrival:** %s\n"+
		"**Status:** %s\n"+
		"**Crew:** %s\n\n",
		name, callsign, departureAirport, arrivalAirport, mission.Status, strings.Join(crew, ", "))

	// Send mission details to the new channel
	_, err = SendPost(ctx, c, newChannelID, missionDetails)
	if err != nil {
		log.Printf("Error sending message to channel: %v", err)
		// Continue anyway, just log the error
	}

	// Send crew papers PDF as a post in the new channel
	crewPapersMsg := "# Crew Papers"

	// Container path for the PDF file
	crewPapersPDFPath := "/app/files/USAF_Flight_Plan_Mock.pdf"

	// Send the crew papers message with file attachment
	_, err = SendPostWithAttachment(ctx, c, newChannelID, crewPapersMsg, crewPapersPDFPath)
	if err != nil {
		// Just log the error and continue - the function will still post a message without the attachment
		log.Printf("Error sending crew papers post: %v", err)
	}

	// Execute weather commands for both airports and send instructional message
	go func(client *Client) {
		// Add a slight delay to ensure the channel message is sent first
		time.Sleep(1 * time.Second)

		// Create context for goroutine
		goroutineCtx := context.Background()

		// First send a message explaining what we're doing
		introMsg := fmt.Sprintf("🌤️ **Checking Weather for Mission** 🌤️\n\nGetting current weather conditions for departure and arrival airports...")
		_, err := SendPost(goroutineCtx, client, newChannelID, introMsg)
		if err != nil {
			log.Printf("Error sending intro message: %v", err)
		}

		// Short delay between posts
		time.Sleep(500 * time.Millisecond)

		// Execute weather command for departure airport
		departureCommand := fmt.Sprintf("/weather %s", departureAirport)
		log.Printf("Executing command in channel %s: %s", newChannelID, departureCommand)
		_, resp, err = client.client.ExecuteCommand(goroutineCtx, newChannelID, departureCommand)
		if err != nil {
			log.Printf("Error executing departure weather command: %v, status code: %d", err, resp.StatusCode)

			// If command execution fails, send a fallback message
			fallbackMsg := fmt.Sprintf("Could not automatically check weather for departure airport (%s). You can check manually with: `/weather %s`", departureAirport, departureAirport)
			SendPost(goroutineCtx, client, newChannelID, fallbackMsg)
			if err != nil {
				log.Printf("Error sending fallback message: %v", err)
			}
		}

		// Add delay between commands
		time.Sleep(2 * time.Second)

		// Execute weather command for arrival airport
		arrivalCommand := fmt.Sprintf("/weather %s", arrivalAirport)
		log.Printf("Executing command in channel %s: %s", newChannelID, arrivalCommand)
		_, resp, err = client.client.ExecuteCommand(goroutineCtx, newChannelID, arrivalCommand)
		if err != nil {
			log.Printf("Error executing arrival weather command: %v, status code: %d", err, resp.StatusCode)

			// If command execution fails, send a fallback message
			fallbackMsg := fmt.Sprintf("Could not automatically check weather for arrival airport (%s). You can check manually with: `/weather %s`", arrivalAirport, arrivalAirport)
			SendPost(goroutineCtx, client, newChannelID, fallbackMsg)
			if err != nil {
				log.Printf("Error sending fallback message: %v", err)
			}
		}
	}(c)

	// Send a message to the mission-planning channel
	planningMsg := fmt.Sprintf("New mission created: **%s** (Callsign: **%s**)\n"+
		"**Departure:** %s\n"+
		"**Arrival:** %s\n"+
		"**Status:** %s\n"+
		"**Crew:** %s\n\n"+
		"[View Channel](~%s)",
		name, callsign, departureAirport, arrivalAirport, mission.Status, strings.Join(crew, ", "), channelName)

	// Get the mission-planning channel ID
	planningChannelID := getPlanningChannelID()
	if planningChannelID != "" {
		_, err := SendPost(ctx, c, planningChannelID, planningMsg)
		if err != nil {
			log.Printf("Error sending message to planning channel: %v", err)
			// Continue anyway, just log the error
		}
	}

	// Send success response back to original channel
	response := MattermostResponse{
		Text:         fmt.Sprintf("✅ Mission **%s** created with callsign **%s**. Channel: ~%s", name, callsign, channelName),
		ResponseType: "in_channel",
		ChannelID:    channelID,
	}
	json.NewEncoder(w).Encode(response)
}

// handleListCommand handles the list command to list all missions
func handleListCommand(c *Client, w http.ResponseWriter, channelID string, missionManager *MissionManager) {
	// Get all missions
	missions := missionManager.GetAllMissions()
	if len(missions) == 0 {
		sendErrorResponse(w, channelID, "No missions found.")
		return
	}

	// Format as a table
	var sb strings.Builder
	sb.WriteString("# Current Missions\n\n")
	sb.WriteString("| Name | Callsign | Departure | Arrival | Status | Channel |\n")
	sb.WriteString("|------|----------|-----------|---------|--------|--------|\n")

	for _, mission := range missions {
		sb.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %s | ~%s |\n",
			mission.Name, mission.Callsign, mission.DepartureAirport, mission.ArrivalAirport,
			mission.Status, mission.ChannelName))
	}

	// Send the response
	response := MattermostResponse{
		Text:         sb.String(),
		ResponseType: "in_channel",
		ChannelID:    channelID,
	}
	json.NewEncoder(w).Encode(response)
}

// handleStatusCommand handles the status command to update a mission's status
func handleStatusCommand(c *Client, w http.ResponseWriter, args []string, channelID string, missionManager *MissionManager, subscriptionManager *SubscriptionManager) {
	ctx := context.Background()

	// Parse arguments
	var missionID, status string

	// First, check if the first arg is a direct status (simplified format)
	if len(args) > 0 && !strings.HasPrefix(args[0], "--") {
		// The first argument is the status itself (simplified format)
		status = args[0]

		// Check remaining args for --id
		for i := 1; i < len(args); i++ {
			if args[i] == "--id" && i+1 < len(args) {
				missionID = args[i+1]
				i++
			}
		}
	} else {
		// Parse in the traditional way with flags
		for i := 0; i < len(args); i++ {
			switch args[i] {
			case "--id":
				if i+1 < len(args) {
					missionID = args[i+1]
					i++
				}
			case "--status":
				if i+1 < len(args) {
					status = args[i+1]
					i++
				}
			}
		}
	}

	// Check if this is a request from a mission channel
	if missionID == "" {
		// Try to find the mission by channel ID
		mission, exists := missionManager.GetMissionByChannelID(channelID)
		if exists {
			missionID = mission.ID
		}
	}

	if missionID == "" {
		sendErrorResponse(w, channelID, "Mission ID is required. Use `--id [id]` or run this command in a mission channel.")
		return
	}

	if status == "" {
		sendErrorResponse(w, channelID, "Status is required. Use `--status [status]`. Valid statuses: stalled, in-air, completed, cancelled")
		return
	}

	// Validate status
	validStatuses := map[string]bool{
		"stalled":   true,
		"in-air":    true,
		"completed": true,
		"cancelled": true,
	}

	if !validStatuses[status] {
		sendErrorResponse(w, channelID, "Invalid status. Valid statuses: stalled, in-air, completed, cancelled")
		return
	}

	// Get the current mission to know the previous status
	mission, exists := missionManager.GetMission(missionID)
	if !exists {
		sendErrorResponse(w, channelID, "Mission not found.")
		return
	}

	oldStatus := mission.Status

	// Update the mission status
	if missionManager.UpdateMissionStatus(missionID, status) {
		// Get the updated mission
		mission, exists = missionManager.GetMission(missionID)
		if !exists {
			sendErrorResponse(w, channelID, "Mission not found after update.")
			return
		}

		// Update the channel display name with the new status emoji
		err := c.UpdateChannelDisplayName(ctx, mission.ChannelID, mission.Callsign, mission.Name, status)
		if err != nil {
			log.Printf("Error updating channel display name: %v", err)
			// Continue anyway, just log the error
		}

		// Send a message to the mission channel
		statusMsg := fmt.Sprintf("Mission status updated to: **%s**", status)
		_, err = SendPost(ctx, c, mission.ChannelID, statusMsg)
		if err != nil {
			log.Printf("Error sending message to mission channel: %v", err)
		}

		// If status changed to "in-air", execute weather commands for both airports
		if status == "in-air" {
			go func(client *Client) {
				// Add a slight delay
				time.Sleep(1 * time.Second)

				// Create context for goroutine
				goroutineCtx := context.Background()

				// First send a message explaining what we're doing
				introMsg := fmt.Sprintf("✈️ **Flight Now In Air** ✈️\n\nUpdating weather conditions for flight path...")
				_, err := SendPost(goroutineCtx, client, mission.ChannelID, introMsg)
				if err != nil {
					log.Printf("Error sending intro message: %v", err)
				}

				// Short delay between posts
				time.Sleep(500 * time.Millisecond)

				// Execute weather command for departure airport
				departureCommand := fmt.Sprintf("/weather %s", mission.DepartureAirport)
				log.Printf("Executing command in channel %s: %s", mission.ChannelID, departureCommand)
				_, resp, err := client.client.ExecuteCommand(goroutineCtx, mission.ChannelID, departureCommand)
				if err != nil {
					log.Printf("Error executing departure weather command: %v, status code: %d", err, resp.StatusCode)
				}

				// Add delay between commands
				time.Sleep(2 * time.Second)

				// Execute weather command for arrival airport
				arrivalCommand := fmt.Sprintf("/weather %s", mission.ArrivalAirport)
				log.Printf("Executing command in channel %s: %s", mission.ChannelID, arrivalCommand)
				_, resp, err = client.client.ExecuteCommand(goroutineCtx, mission.ChannelID, arrivalCommand)
				if err != nil {
					log.Printf("Error executing arrival weather command: %v, status code: %d", err, resp.StatusCode)
				}
			}(c)
		}

		// Send a message to the mission-planning channel
		planningMsg := fmt.Sprintf("Mission **%s** (Callsign: **%s**) status updated to: **%s**\n"+
			"[View Channel](~%s)",
			mission.Name, mission.Callsign, status, mission.ChannelName)

		planningChannelID := getPlanningChannelID()
		if planningChannelID != "" {
			_, err := SendPost(ctx, c, planningChannelID, planningMsg)
			if err != nil {
				log.Printf("Error sending message to planning channel: %v", err)
				// Continue anyway, just log the error
			}
		}

		// Notify subscribed channels if the status changed
		if oldStatus != status {
			go notifySubscribersOfStatusChange(mission, oldStatus, subscriptionManager, c)
		}

		// Send success response
		response := MattermostResponse{
			Text:         fmt.Sprintf("✅ Mission **%s** status updated to **%s**", mission.Name, status),
			ResponseType: "in_channel",
			ChannelID:    channelID,
		}
		json.NewEncoder(w).Encode(response)
	} else {
		sendErrorResponse(w, channelID, "Mission not found.")
	}
}

// Helper functions
func sendErrorResponse(w http.ResponseWriter, channelID, message string) {
	response := MattermostResponse{
		Text:         message,
		ResponseType: "ephemeral", // Only visible to the user
		ChannelID:    channelID,
	}
	json.NewEncoder(w).Encode(response)
}

func sendHelpResponse(w http.ResponseWriter, channelID string) {
	helpText := "**Mission Operations Commands**\n\n" +
		"**Mission Commands:**\n" +
		"- `/mission start --name [name] --callsign [callsign] --departureAirport [code] --arrivalAirport [code] --crew @user1 @user2 ...` - Create a new mission\n" +
		"- `/mission list` - List all missions\n" +
		"- `/mission status [status]` - Update mission status (run in mission channel to skip --id)\n" +
		"- `/mission complete` - Fill out and submit a post-mission report form\n" +
		"- `/mission help` - Show this help message\n\n" +
		"**Subscription Commands:**\n" +
		"- `/mission subscribe --type [status1,status2] --frequency [seconds]` - Subscribe to mission status updates\n" +
		"- `/mission subscribe --type all --frequency [seconds]` - Subscribe to all mission status updates\n" +
		"- `/mission unsubscribe --id [subscription_id]` - Unsubscribe from updates\n" +
		"- `/mission subscriptions` - List all subscriptions in this channel\n\n" +
		"**Valid Statuses:**\n" +
		"- `stalled` - Mission is not active\n" +
		"- `in-air` - Mission is in progress\n" +
		"- `completed` - Mission has been completed successfully\n" +
		"- `cancelled` - Mission has been cancelled\n\n" +
		"**Examples:**\n" +
		"- `/mission start --name Alpha --callsign Eagle1 --departureAirport JFK --arrivalAirport LAX --crew @john @sarah`\n" +
		"- `/mission status in-air`\n" +
		"- `/mission status completed`\n" +
		"- `/mission status cancelled --id [mission_id]` (when not in mission channel)\n" +
		"- `/mission complete` (in a mission channel)\n" +
		"- `/mission subscribe --type stalled,in-air --frequency 3600` (updates hourly)\n" +
		"- `/mission subscribe --type all --frequency 1800` (updates every 30 minutes)"

	response := MattermostResponse{
		Text:         helpText,
		ResponseType: "ephemeral",
		ChannelID:    channelID,
	}
	json.NewEncoder(w).Encode(response)
}

// Helper function to parse command text into arguments
func parseCommand(text string) []string {
	// Split by spaces, but respect quoted strings
	var args []string
	inQuotes := false
	current := ""

	for _, char := range text {
		if char == '"' {
			inQuotes = !inQuotes
		} else if char == ' ' && !inQuotes {
			if current != "" {
				args = append(args, current)
				current = ""
			}
		} else {
			current += string(char)
		}
	}

	if current != "" {
		args = append(args, current)
	}

	return args
}

// Helper function to get the mission-planning channel ID
func getPlanningChannelID() string {
	// Try to get from environment variable
	planningChannelID := os.Getenv("MISSION_PLANNING_CHANNEL_ID")
	if planningChannelID != "" {
		return planningChannelID
	}

	// For now, hardcode the channel ID
	// In a real implementation, we'd look this up from config
	return "mission-planning"
}

// Helper function to get the missionops app URL
func getMissionOpsAppURL() string {
	// Default missionops app URL
	defaultURL := "http://missionops-app:8087"

	// If we have an APP_URL environment variable set, use that instead
	// This would be used for external access configurations like ngrok
	if envURL := os.Getenv("APP_URL"); envURL != "" {
		return envURL
	}

	return defaultURL
}

// SendPost creates and sends a branded post with the mission-ops-bot username
func SendPost(ctx context.Context, client *Client, channelID, message string) (*model.Post, error) {
	post := &model.Post{
		ChannelId: channelID,
		Message:   message,
		Props: map[string]any{
			"from_webhook":      "true",
			"override_username": "mission-ops-bot",
		},
	}
	_, resp, err := client.client.CreatePost(ctx, post)
	if err != nil {
		return &model.Post{}, fmt.Errorf("error sending post: %v, status code: %d", err, resp.StatusCode)
	}

	return post, nil
}

// SendPostWithAttachment creates and sends a post with a file attachment
func SendPostWithAttachment(ctx context.Context, client *Client, channelID, message, filePath string) (*model.Post, error) {
	// Try to upload and send the post with attachment
	result, err := client.SendPostWithAttachment(ctx, channelID, message, filePath)

	// Handle error cases - check if it's a file not found error or any other error
	if err != nil {
		// Check if the original error contains "not found" or is a not exist error
		errText := err.Error()
		if strings.Contains(errText, "not found") ||
			strings.Contains(errText, "no such file") ||
			strings.Contains(errText, "does not exist") {
			log.Printf("Warning: File %s not found, sending message without attachment", filePath)
			// Send just the message without the attachment
			return SendPost(ctx, client, channelID, message+"\n\n*Flight plan attachment not available*")
		}

		// For any other error, we'll log it but still send the message without attachment
		log.Printf("Error sending post with attachment: %v", err)
		return SendPost(ctx, client, channelID, message+"\n\n*Flight plan could not be attached due to an error*")
	}

	return result, nil
}

// handleSubscribeCommand handles the subscribe command
func handleSubscribeCommand(c *Client, w http.ResponseWriter, args []string, channelID, userID string, subscriptionManager *SubscriptionManager) {
	// Check if help was requested
	for _, arg := range args {
		if arg == "--help" || arg == "help" || arg == "-h" {
			subscribeHelpResponse(w, channelID)
			return
		}
	}

	// Parse arguments
	var statusTypes []string
	var frequency int64 = 3600 // Default to hourly updates
	var hasTypeArg bool

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--type", "--types":
			hasTypeArg = true
			if i+1 < len(args) {
				// Parse comma-separated status types
				typesStr := args[i+1]
				if typesStr == "all" {
					// Empty slice means all status types
					statusTypes = []string{}
				} else {
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
							sendErrorResponse(w, channelID, fmt.Sprintf("Invalid status type: %s. Valid types: stalled, in-air, completed, cancelled, or 'all'", status))
							return
						}
					}
				}
				i++
			}
		case "--frequency":
			if i+1 < len(args) {
				var err error
				frequency, err = strconv.ParseInt(args[i+1], 10, 64)
				if err != nil {
					sendErrorResponse(w, channelID, "Invalid frequency format. Please use seconds (e.g., 3600 for hourly).")
					return
				}
				i++
			}
		}
	}

	// If no explicit type argument provided, default to all statuses
	if !hasTypeArg {
		statusTypes = []string{}
	}

	// Validate minimum frequency (5 minutes = 300 seconds)
	if frequency < 300 {
		sendErrorResponse(w, channelID, "Frequency must be at least 300 seconds (5 minutes).")
		return
	}

	// Create a new subscription
	subscription := &MissionSubscription{
		ID:              fmt.Sprintf("mission-sub-%s-%d", channelID, time.Now().Unix()),
		ChannelID:       channelID,
		UserID:          userID,
		StatusTypes:     statusTypes,
		UpdateFrequency: frequency,
		LastUpdated:     time.Now(),
		StopChan:        make(chan struct{}),
	}

	// Add the subscription
	subscriptionManager.AddSubscription(subscription)

	// Create the client here to use for the subscription
	client, err := NewClient()
	if err != nil {
		log.Printf("Error creating Mattermost client for subscription: %v", err)
		sendErrorResponse(w, channelID, "Error setting up subscription. Please try again.")
		return
	}

	// Start the subscription
	go startMissionSubscription(subscription, subscriptionManager, NewMissionManager("/app/data/missions.json"), client)

	// Format status types for display
	statusTypesText := "all mission statuses"
	if len(statusTypes) > 0 {
		statusTypesText = fmt.Sprintf("mission statuses: %s", strings.Join(statusTypes, ", "))
	}

	// Send confirmation
	response := MattermostResponse{
		Text:         fmt.Sprintf("✅ Subscribed to %s. Updates will be sent every %d seconds (ID: `%s`).", statusTypesText, frequency, subscription.ID),
		ResponseType: "in_channel",
		ChannelID:    channelID,
	}
	json.NewEncoder(w).Encode(response)
}

// handleUnsubscribeCommand handles the unsubscribe command
func handleUnsubscribeCommand(c *Client, w http.ResponseWriter, args []string, channelID, userID string, subscriptionManager *SubscriptionManager) {
	// Check if help was requested
	for _, arg := range args {
		if arg == "--help" || arg == "help" || arg == "-h" {
			unsubscribeHelpResponse(w, channelID)
			return
		}
	}

	// Parse arguments
	var subscriptionID string

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--id":
			if i+1 < len(args) {
				subscriptionID = args[i+1]
				i++
			}
		}
	}

	if subscriptionID == "" {
		// If no ID provided, list subscriptions for the channel
		handleListSubscriptionsCommand(c, w, []string{}, channelID, subscriptionManager)
		return
	}

	// Get the subscription
	sub, exists := subscriptionManager.GetSubscription(subscriptionID)
	if !exists {
		sendErrorResponse(w, channelID, fmt.Sprintf("Subscription with ID `%s` not found.", subscriptionID))
		return
	}

	// Check if the subscription belongs to this channel
	if sub.ChannelID != channelID {
		sendErrorResponse(w, channelID, "This subscription does not belong to this channel.")
		return
	}

	// Remove the subscription
	if subscriptionManager.RemoveSubscription(subscriptionID) {
		// Format status types for display
		statusTypesText := "all mission statuses"
		if len(sub.StatusTypes) > 0 {
			statusTypesText = fmt.Sprintf("mission statuses: %s", strings.Join(sub.StatusTypes, ", "))
		}

		response := MattermostResponse{
			Text:         fmt.Sprintf("✅ Unsubscribed from %s.", statusTypesText),
			ResponseType: "in_channel",
			ChannelID:    channelID,
		}
		json.NewEncoder(w).Encode(response)
	} else {
		sendErrorResponse(w, channelID, "Failed to unsubscribe. Please try again.")
	}
}

// subscribeHelpResponse sends specific help for the subscribe command
func subscribeHelpResponse(w http.ResponseWriter, channelID string) {
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

	response := MattermostResponse{
		Text:         helpText,
		ResponseType: "ephemeral",
		ChannelID:    channelID,
	}
	json.NewEncoder(w).Encode(response)
}

// unsubscribeHelpResponse sends specific help for the unsubscribe command
func unsubscribeHelpResponse(w http.ResponseWriter, channelID string) {
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

	response := MattermostResponse{
		Text:         helpText,
		ResponseType: "ephemeral",
		ChannelID:    channelID,
	}
	json.NewEncoder(w).Encode(response)
}

// subscriptionsListHelpResponse sends specific help for the subscriptions command
func subscriptionsListHelpResponse(w http.ResponseWriter, channelID string) {
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

	response := MattermostResponse{
		Text:         helpText,
		ResponseType: "ephemeral",
		ChannelID:    channelID,
	}
	json.NewEncoder(w).Encode(response)
}

// handleListSubscriptionsCommand handles the list subscriptions command
func handleListSubscriptionsCommand(c *Client, w http.ResponseWriter, args []string, channelID string, subscriptionManager *SubscriptionManager) {
	// Check if help was requested
	for _, arg := range args {
		if arg == "--help" || arg == "help" || arg == "-h" {
			subscriptionsListHelpResponse(w, channelID)
			return
		}
	}

	// Get subscriptions for the channel
	subs := subscriptionManager.GetSubscriptionsForChannel(channelID)
	if len(subs) == 0 {
		sendErrorResponse(w, channelID, "No active subscriptions found in this channel.")
		return
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

	response := MattermostResponse{
		Text:         sb.String(),
		ResponseType: "in_channel",
		ChannelID:    channelID,
	}
	json.NewEncoder(w).Encode(response)
}

// handleCompleteCommand creates an interactive form for post-mission reports
func handleCompleteCommand(c *Client, w http.ResponseWriter, r *http.Request, args []string, channelID, userID string, missionManager *MissionManager) {
	// Check if this is a request from a mission channel
	var mission *Mission
	var missionID string

	// Parse arguments if any
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--id":
			if i+1 < len(args) {
				missionID = args[i+1]
				i++
			}
		}
	}

	// If no mission ID provided, try to find the mission based on channel ID
	if missionID == "" {
		var exists bool
		mission, exists = missionManager.GetMissionByChannelID(channelID)
		if exists {
			missionID = mission.ID
		} else {
			sendErrorResponse(w, channelID, "This command must be run in a mission channel, or provide --id [mission_id]")
			return
		}
	} else {
		// Get the mission by ID
		var exists bool
		mission, exists = missionManager.GetMission(missionID)
		if !exists {
			sendErrorResponse(w, channelID, "Mission not found with the provided ID.")
			return
		}
	}

	// Get the trigger ID from the request - this is needed to open a dialog
	triggerID := r.FormValue("trigger_id")
	if triggerID == "" {
		log.Printf("Missing trigger_id in request")
		sendErrorResponse(w, channelID, "Unable to open post-mission report form. Missing trigger ID.")
		return
	}

	// Create the interactive dialog for post-mission report
	// Always use the missionops service URL directly, not the Mattermost URL
	appURL := getMissionOpsAppURL()
	log.Printf("Using missionops app URL: %s", appURL)

	// Use a dedicated endpoint for mission complete submissions
	webhookURL := fmt.Sprintf("%s/mission/complete", appURL)
	log.Printf("Using webhook URL for dialog: %s", webhookURL)

	dialog := map[string]interface{}{
		"trigger_id": triggerID,
		"url":        webhookURL,
		"dialog": map[string]interface{}{
			"callback_id":       "mission_complete_dialog",
			"title":             "Post-Mission Report",
			"introduction_text": fmt.Sprintf("Complete mission report for: **%s** (Callsign: **%s**)", mission.Name, mission.Callsign),
			"submit_label":      "Submit Report",
			"notify_on_cancel":  false,
			"state":             mission.ID,
			"elements": []map[string]interface{}{
				{
					"display_name": "Mission Objectives Completion",
					"name":         "mission_objectives_completion",
					"type":         "select",
					"options": []map[string]string{
						{"text": "Yes - All objectives completed", "value": "all_completed"},
						{"text": "Partial - Some objectives completed", "value": "partial"},
						{"text": "No - Mission objectives not met", "value": "none"},
					},
					"optional": false,
				},
				{
					"display_name": "Mission Duration (hours)",
					"name":         "mission_duration",
					"type":         "text",
					"subtype":      "number",
					"placeholder":  "Enter flight hours",
					"optional":     false,
				},
				{
					"display_name": "Crew Performance",
					"name":         "crew_performance",
					"type":         "select",
					"options": []map[string]string{
						{"text": "Excellent", "value": "excellent"},
						{"text": "Good", "value": "good"},
						{"text": "Satisfactory", "value": "satisfactory"},
						{"text": "Needs Improvement", "value": "needs_improvement"},
					},
					"optional": false,
				},
				{
					"display_name": "Notable Events",
					"name":         "notable_events",
					"type":         "textarea",
					"placeholder":  "Enter any notable events during the mission",
					"optional":     true,
					"max_length":   2000,
				},
			},
		},
	}

	// Send the dialog request to Mattermost
	// This endpoint is used to open an interactive dialog
	dialogURL := fmt.Sprintf("%s/api/v4/actions/dialogs/open", os.Getenv("MM_ServiceSettings_SiteURL"))

	// Convert dialog to JSON
	dialogJSON, err := json.Marshal(dialog)
	if err != nil {
		log.Printf("Error marshaling dialog JSON: %v", err)
		sendErrorResponse(w, channelID, "Error creating post-mission report form")
		return
	}

	// Create HTTP client
	client := &http.Client{}

	// Create POST request
	req, err := http.NewRequest(http.MethodPost, dialogURL, bytes.NewBuffer(dialogJSON))
	if err != nil {
		log.Printf("Error creating dialog request: %v", err)
		sendErrorResponse(w, channelID, "Error creating post-mission report form")
		return
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.client.AuthToken)

	// Send request
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("Error sending dialog request: %v", err)
		sendErrorResponse(w, channelID, "Error opening post-mission report form")
		return
	}
	defer resp.Body.Close()

	// Check response
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("Error from Mattermost API: %s (status: %d)", string(body), resp.StatusCode)
		sendErrorResponse(w, channelID, "Error opening post-mission report form")
		return
	}

	// Send an ephemeral response to confirm
	response := MattermostResponse{
		Text:         "Opening post-mission report form...",
		ResponseType: "ephemeral",
		ChannelID:    channelID,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// Note: We've removed the legacy handleDialogSubmission function since we now use
// the dedicated endpoint handleMissionCompleteSubmission directly

// handleMissionCompleteSubmission handles submissions from the mission complete dialog
// This is a dedicated endpoint that only handles mission complete forms
func handleMissionCompleteSubmission(w http.ResponseWriter, r *http.Request, missionManager *MissionManager, subscriptionManager *SubscriptionManager) {
	ctx := context.Background()

	log.Printf("✅ Received mission complete submission at dedicated endpoint: %s %s (Content-Type: %s)",
		r.Method, r.URL.Path, r.Header.Get("Content-Type"))

	// Read the request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("Error reading request body: %v", err)
		http.Error(w, "Error reading request", http.StatusBadRequest)
		return
	}

	// Log the raw body for debugging
	log.Printf("Mission complete raw body: %s", string(body))

	// Parse the dialog submission JSON
	var dialogSubmission struct {
		CallbackID string                 `json:"callback_id"`
		State      string                 `json:"state"` // Contains the mission ID
		Submission map[string]interface{} `json:"submission"`
		UserID     string                 `json:"user_id"`
		ChannelID  string                 `json:"channel_id"`
		TeamID     string                 `json:"team_id"`
	}

	if err := json.Unmarshal(body, &dialogSubmission); err != nil {
		log.Printf("Error parsing dialog submission: %v", err)
		http.Error(w, "Invalid submission format", http.StatusBadRequest)
		return
	}

	log.Printf("Received dialog submission: %+v", dialogSubmission)
	log.Printf("Submission data: callback_id=%s, mission_id=%s, channelID=%s, userID=%s",
		dialogSubmission.CallbackID, dialogSubmission.State, dialogSubmission.ChannelID, dialogSubmission.UserID)

	if dialogSubmission.CallbackID != "mission_complete_dialog" {
		log.Printf("Unexpected callback_id: %s", dialogSubmission.CallbackID)
		http.Error(w, "Unexpected callback_id", http.StatusBadRequest)
		return
	}

	// Get direct references to the submission values
	submission := dialogSubmission.Submission
	state := dialogSubmission.State
	// channelID := dialogSubmission.ChannelID
	userID := dialogSubmission.UserID

	// Extract values from submission
	objectivesCompletion, _ := submission["mission_objectives_completion"].(string)

	// Mission duration could be a number or string depending on the form handling
	var missionDurationStr string
	if durationNum, ok := submission["mission_duration"].(float64); ok {
		missionDurationStr = fmt.Sprintf("%.1f", durationNum)
	} else if durationStr, ok := submission["mission_duration"].(string); ok {
		missionDurationStr = durationStr
	} else {
		missionDurationStr = "Unknown"
		log.Printf("Mission duration has unexpected type: %T", submission["mission_duration"])
	}

	crewPerformance, _ := submission["crew_performance"].(string)
	notableEvents, _ := submission["notable_events"].(string)

	// Get the mission
	mission, exists := missionManager.GetMission(state)
	if !exists {
		log.Printf("Mission not found: %s", state)
		http.Error(w, "Mission not found", http.StatusBadRequest)
		return
	}

	// Get the old status for notifications
	oldStatus := mission.Status

	// Create Mattermost client
	client, err := NewClient()
	if err != nil {
		log.Printf("Error creating Mattermost client: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Format the mission report message
	reportMsg := fmt.Sprintf("# Post-Mission Report: %s\n\n", mission.Name)
	reportMsg += fmt.Sprintf("**Mission:** %s (Callsign: **%s**)\n", mission.Name, mission.Callsign)
	reportMsg += fmt.Sprintf("**Route:** %s → %s\n", mission.DepartureAirport, mission.ArrivalAirport)
	reportMsg += fmt.Sprintf("**Duration:** %s hours\n", missionDurationStr)

	// Format objectives completion
	var objectivesText string
	switch objectivesCompletion {
	case "all_completed":
		objectivesText = "✅ All objectives completed"
	case "partial":
		objectivesText = "⚠️ Partial objectives completed"
	case "none":
		objectivesText = "❌ Mission objectives not met"
	default:
		objectivesText = "Unknown"
	}
	reportMsg += fmt.Sprintf("**Objectives:** %s\n", objectivesText)

	// Format crew performance
	var performanceText string
	switch crewPerformance {
	case "excellent":
		performanceText = "⭐⭐⭐⭐⭐ Excellent"
	case "good":
		performanceText = "⭐⭐⭐⭐ Good"
	case "satisfactory":
		performanceText = "⭐⭐⭐ Satisfactory"
	case "needs_improvement":
		performanceText = "⭐⭐ Needs Improvement"
	default:
		performanceText = "Unknown"
	}
	reportMsg += fmt.Sprintf("**Crew Performance:** %s\n", performanceText)

	// Add notable events if provided
	if notableEvents != "" {
		reportMsg += fmt.Sprintf("\n## Notable Events\n%s\n", notableEvents)
	}

	// Check if we have a valid user ID
	submitterText := "Unknown user"
	if userID != "" {
		submitterText = fmt.Sprintf("<@%s>", userID)
	}
	reportMsg += fmt.Sprintf("\n*Report submitted by %s on %s*", submitterText, time.Now().Format(time.RFC1123))

	// Send the report to the mission channel (always use the mission's associated channel)
	// This ensures the report goes to the right place even if the dialog was submitted from elsewhere
	_, err = SendPost(ctx, client, mission.ChannelID, reportMsg)
	if err != nil {
		log.Printf("Error sending mission report to channel %s: %v", mission.ChannelID, err)
	}

	// Update the mission status to completed
	if missionManager.UpdateMissionStatus(mission.ID, "completed") {
		// Update the channel display name with the completed status emoji
		err = client.UpdateChannelDisplayName(ctx, mission.ChannelID, mission.Callsign, mission.Name, "completed")
		if err != nil {
			log.Printf("Error updating channel display name: %v", err)
		}

		// Send status update message to the mission channel
		statusMsg := "Mission status updated to: **completed**"
		_, err = SendPost(ctx, client, mission.ChannelID, statusMsg)
		if err != nil {
			log.Printf("Error sending status update to mission channel: %v", err)
		}

		// Notify the mission-planning channel
		planningMsg := fmt.Sprintf("Mission **%s** (Callsign: **%s**) status updated to: **completed**\n"+
			"[View Channel](~%s)",
			mission.Name, mission.Callsign, mission.ChannelName)

		planningChannelID := getPlanningChannelID()
		if planningChannelID != "" {
			_, err := SendPost(ctx, client, planningChannelID, planningMsg)
			if err != nil {
				log.Printf("Error sending message to planning channel: %v", err)
			}
		}

		// Get the updated mission
		mission, _ = missionManager.GetMission(mission.ID)

		// Notify subscribed channels of the status change
		go notifySubscribersOfStatusChange(mission, oldStatus, subscriptionManager, client)
	}

	// Send a success response
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("{}"))
	log.Printf("Mission complete submission processed successfully")
}
