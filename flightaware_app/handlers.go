package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func handleFlightDepartureRequest(w http.ResponseWriter, r *http.Request) {
	// Get parameters from query string
	airport := r.URL.Query().Get("airport")
	startStr := r.URL.Query().Get("start")
	endStr := r.URL.Query().Get("end")

	if airport == "" {
		http.Error(w, "Airport parameter is required", http.StatusBadRequest)
		return
	}

	// Parse start and end times
	var start, end int64
	var err error
	if startStr != "" {
		start, err = strconv.ParseInt(startStr, 10, 64)
		if err != nil {
			http.Error(w, "Invalid start time format", http.StatusBadRequest)
			return
		}
	} else {
		// Default to 24 hours ago
		start = time.Now().Add(-24 * time.Hour).Unix()
	}

	if endStr != "" {
		end, err = strconv.ParseInt(endStr, 10, 64)
		if err != nil {
			http.Error(w, "Invalid end time format", http.StatusBadRequest)
			return
		}
	} else {
		// Default to current time
		end = time.Now().Unix()
	}

	// Get flight data
	flights, err := getDepartureFlights(airport, start, end)
	if err != nil {
		log.Printf("Error fetching flight data: %v", err)
		http.Error(w, fmt.Sprintf("Error fetching flight data: %v", err), http.StatusInternalServerError)
		return
	}

	// Return the flight data
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(flights)
}

func handleIncomingWebhook(w http.ResponseWriter, r *http.Request) {
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

	log.Printf("Command: %s, Text: %s, ChannelID: %s, UserID: %s", command, text, channelID, userID)

	if channelID == "" {
		log.Printf("Missing channel_id field")
		http.Error(w, "Missing channel_id field", http.StatusBadRequest)
		return
	}

	// If this is the /flights command
	if command == "/flights" {
		// If text is empty or "help", show help
		if text == "" || text == "help" {
			sendHelpResponse(w, channelID)
			return
		}

		// Parse the command arguments
		args := parseCommand(text)

		// Check if this is a departures command
		if len(args) >= 1 && args[0] == "departures" {
			handleDeparturesCommand(w, args[1:], channelID, userID)
			return
		}

		// Unknown subcommand
		sendErrorResponse(w, channelID, fmt.Sprintf("Unknown subcommand: %s. Use `/flights help` for available commands.", text))
		return
	}

	// If we get here, it's an unknown command
	sendHelpResponse(w, channelID)
}

func handleDeparturesCommand(w http.ResponseWriter, args []string, channelID, userID string) {
	// Parse arguments
	var airport string
	var start, end int64
	var err error

	// Default end time to now
	end = time.Now().Unix()
	// Default start time to 24 hours ago
	start = time.Now().Add(-24 * time.Hour).Unix()

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--airport":
			if i+1 < len(args) {
				airport = strings.ToUpper(args[i+1])
				i++
			}
		case "--start":
			if i+1 < len(args) {
				start, err = strconv.ParseInt(args[i+1], 10, 64)
				if err != nil {
					sendErrorResponse(w, channelID, "Invalid start time format. Please use Unix timestamp (seconds since epoch).")
					return
				}
				i++
			}
		case "--end":
			if i+1 < len(args) {
				end, err = strconv.ParseInt(args[i+1], 10, 64)
				if err != nil {
					sendErrorResponse(w, channelID, "Invalid end time format. Please use Unix timestamp (seconds since epoch).")
					return
				}
				i++
			}
		case "--help":
			sendHelpResponse(w, channelID)
			return
		}
	}

	if airport == "" {
		sendErrorResponse(w, channelID, "Airport code is required. Use `--airport [code]`")
		return
	}

	// Get flight data
	flights, err := getDepartureFlights(airport, start, end)
	if err != nil {
		log.Printf("Error fetching flight data: %v", err)
		sendErrorResponse(w, channelID, fmt.Sprintf("Error fetching flight data: %v", err))
		return
	}

	// Format the response
	response := formatFlightResponse(flights, airport, start, end)

	// Send the response
	mattermostResponse := MattermostResponse{
		Text:         response,
		ResponseType: "in_channel", // Make this visible to everyone in the channel
		ChannelID:    channelID,
	}
	json.NewEncoder(w).Encode(mattermostResponse)
}

func sendErrorResponse(w http.ResponseWriter, channelID, message string) {
	response := MattermostResponse{
		Text:         message,
		ResponseType: "ephemeral", // Only visible to the user
		ChannelID:    channelID,
	}
	json.NewEncoder(w).Encode(response)
}

func sendHelpResponse(w http.ResponseWriter, channelID string) {
	helpText := "**Flight Departures Bot Commands**\n\n" +
		"- `/flights departures --airport [code]` - Get departures from an airport (last 24 hours)\n" +
		"- `/flights departures --airport [code] --start [unix_time] --end [unix_time]` - Get departures for a specific time range\n" +
		"- `/flights help` - Show this help message\n\n" +
		"**Examples:**\n" +
		"- `/flights departures --airport KSFO` - Get departures from San Francisco International\n" +
		"- `/flights departures --airport EGLL --start 1714521600 --end 1714608000` - Get departures from London Heathrow for a specific day\n\n" +
		"**Current Unix Time:** " + fmt.Sprintf("%d", time.Now().Unix()) + "\n" +
		"**24 Hours Ago:** " + fmt.Sprintf("%d", time.Now().Add(-24*time.Hour).Unix())

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
