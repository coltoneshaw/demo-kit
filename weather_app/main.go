package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// WeatherResponse represents the response from the Tomorrow.io API
type WeatherResponse struct {
	Data struct {
		Time   string `json:"time"`
		Values struct {
			Temperature              float64 `json:"temperature"`
			TemperatureApparent      float64 `json:"temperatureApparent"`
			Humidity                 int     `json:"humidity"`
			PrecipitationProbability int     `json:"precipitationProbability"`
			RainIntensity            float64 `json:"rainIntensity"`
			WindSpeed                float64 `json:"windSpeed"`
			WindGust                 float64 `json:"windGust"`
			WindDirection            int     `json:"windDirection"`
			CloudCover               int     `json:"cloudCover"`
			WeatherCode              int     `json:"weatherCode"`
		} `json:"values"`
	} `json:"data"`
	Location struct {
		Lat  float64 `json:"lat"`
		Lon  float64 `json:"lon"`
		Name string  `json:"name"`
		Type string  `json:"type"`
	} `json:"location"`
}

// MattermostPayload represents the incoming webhook payload from Mattermost
type MattermostPayload struct {
	Text    string `json:"text"`
	UserID  string `json:"user_id"`
	Channel string `json:"channel_name"`
}

// MattermostResponse represents the response to send back to Mattermost
type MattermostResponse struct {
	Text         string `json:"text"`
	ResponseType string `json:"response_type"`
	ChannelID    string `json:"channel_id,omitempty"`
}

// Subscription represents a weather subscription for a channel
type Subscription struct {
	ID              string        `json:"id"`               // Unique identifier for the subscription
	Location        string        `json:"location"`         // Location to get weather for
	ChannelID       string        `json:"channel_id"`       // Channel to post updates to
	UserID          string        `json:"user_id"`          // User who created the subscription
	UpdateFrequency int64         `json:"update_frequency"` // How often to update (in milliseconds)
	LastUpdated     time.Time     `json:"last_updated"`     // When the subscription was last updated
	StopChan        chan struct{} `json:"-"`                // Channel to signal stopping the subscription (not serialized)
}

// SubscriptionManager manages all active subscriptions
type SubscriptionManager struct {
	Subscriptions map[string]*Subscription `json:"subscriptions"` // Map of subscription ID to subscription
	Mutex         sync.RWMutex             `json:"-"`             // Mutex to protect the map (not serialized)
	FilePath      string                   `json:"-"`             // Path to the subscription file (not serialized)
	HourlyLimit   int                      `json:"hourly_limit"`  // Maximum API calls per hour
	DailyLimit    int                      `json:"daily_limit"`   // Maximum API calls per day
}

// NewSubscriptionManager creates a new subscription manager
func NewSubscriptionManager(filePath string) *SubscriptionManager {
	sm := &SubscriptionManager{
		Subscriptions: make(map[string]*Subscription),
		FilePath:      filePath,
		HourlyLimit:   25,  // 25 requests per hour limit
		DailyLimit:    500, // 500 requests per day limit
	}

	// Load existing subscriptions from file
	if err := sm.LoadFromFile(); err != nil {
		log.Printf("Failed to load subscriptions from file: %v", err)
	}

	return sm
}

// SaveToFile saves all subscriptions to a JSON file
func (sm *SubscriptionManager) SaveToFile() error {
	sm.Mutex.RLock()
	defer sm.Mutex.RUnlock()

	// Create directory if it doesn't exist
	dir := filepath.Dir(sm.FilePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %v", err)
	}

	// Marshal to JSON
	data, err := json.MarshalIndent(sm, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal subscriptions: %v", err)
	}

	// Write to file
	if err := os.WriteFile(sm.FilePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write subscriptions file: %v", err)
	}
	
	// Only log during initial load or when explicitly requested
	if len(sm.Subscriptions) > 0 && os.Getenv("DEBUG") == "true" {
		log.Printf("Saved %d subscriptions to %s", len(sm.Subscriptions), sm.FilePath)
	}
	return nil
}

// LoadFromFile loads subscriptions from a JSON file
func (sm *SubscriptionManager) LoadFromFile() error {
	sm.Mutex.Lock()
	defer sm.Mutex.Unlock()

	// Check if file exists
	if _, err := os.Stat(sm.FilePath); os.IsNotExist(err) {
		log.Printf("Subscriptions file does not exist at %s", sm.FilePath)
		return nil // Not an error, just no file yet
	}

	// Read file
	data, err := os.ReadFile(sm.FilePath)
	if err != nil {
		return fmt.Errorf("failed to read subscriptions file: %v", err)
	}

	// Unmarshal JSON
	var loadedManager SubscriptionManager
	if err := json.Unmarshal(data, &loadedManager); err != nil {
		return fmt.Errorf("failed to unmarshal subscriptions: %v", err)
	}

	// Copy subscriptions to current manager
	sm.Subscriptions = loadedManager.Subscriptions

	// Initialize StopChan for each subscription
	for _, sub := range sm.Subscriptions {
		sub.StopChan = make(chan struct{})
	}

	log.Printf("Loaded %d subscriptions from %s", len(sm.Subscriptions), sm.FilePath)
	return nil
}

// AddSubscription adds a new subscription
func (sm *SubscriptionManager) AddSubscription(sub *Subscription) {
	sm.Mutex.Lock()
	defer sm.Mutex.Unlock()
	sm.Subscriptions[sub.ID] = sub

	// Save to file after adding
	go sm.SaveToFile()
}

// RemoveSubscription removes a subscription
func (sm *SubscriptionManager) RemoveSubscription(id string) bool {
	sm.Mutex.Lock()
	defer sm.Mutex.Unlock()

	sub, exists := sm.Subscriptions[id]
	if exists {
		// Signal the subscription to stop
		close(sub.StopChan)
		// Remove from map
		delete(sm.Subscriptions, id)
		// Save to file after removing
		go sm.SaveToFile()
		return true
	}
	return false
}

// GetSubscription gets a subscription by ID
func (sm *SubscriptionManager) GetSubscription(id string) (*Subscription, bool) {
	sm.Mutex.RLock()
	defer sm.Mutex.RUnlock()
	sub, exists := sm.Subscriptions[id]
	return sub, exists
}

// GetSubscriptionsForChannel gets all subscriptions for a channel
func (sm *SubscriptionManager) GetSubscriptionsForChannel(channelID string) []*Subscription {
	sm.Mutex.RLock()
	defer sm.Mutex.RUnlock()

	var subs []*Subscription
	for _, sub := range sm.Subscriptions {
		if sub.ChannelID == channelID {
			subs = append(subs, sub)
		}
	}
	return subs
}

// GetSubscriptionsForUser gets all subscriptions created by a user
func (sm *SubscriptionManager) GetSubscriptionsForUser(userID string) []*Subscription {
	sm.Mutex.RLock()
	defer sm.Mutex.RUnlock()

	var subs []*Subscription
	for _, sub := range sm.Subscriptions {
		if sub.UserID == userID {
			subs = append(subs, sub)
		}
	}
	return subs
}

// CalculateAPIUsage calculates the current API usage per hour and per day
func (sm *SubscriptionManager) CalculateAPIUsage() (hourlyUsage, dailyUsage int) {
	sm.Mutex.RLock()
	defer sm.Mutex.RUnlock()
	
	for _, sub := range sm.Subscriptions {
		// Calculate how many requests this subscription makes per hour
		// (3600000 milliseconds in an hour)
		requestsPerHour := 3600000 / sub.UpdateFrequency
		if requestsPerHour < 1 {
			requestsPerHour = 1 // Minimum 1 request per hour
		}
		hourlyUsage += int(requestsPerHour)
		
		// Calculate how many requests this subscription makes per day
		// (86400000 milliseconds in a day)
		requestsPerDay := 86400000 / sub.UpdateFrequency
		if requestsPerDay < 1 {
			requestsPerDay = 1 // Minimum 1 request per day
		}
		dailyUsage += int(requestsPerDay)
	}
	
	return hourlyUsage, dailyUsage
}

// CheckSubscriptionLimits checks if adding a new subscription would exceed API limits
func (sm *SubscriptionManager) CheckSubscriptionLimits(updateFrequency int64) (bool, string) {
	// Calculate current usage
	currentHourlyUsage, currentDailyUsage := sm.CalculateAPIUsage()
	
	// Calculate new subscription's usage
	newHourlyUsage := 3600000 / updateFrequency
	if newHourlyUsage < 1 {
		newHourlyUsage = 1
	}
	
	newDailyUsage := 86400000 / updateFrequency
	if newDailyUsage < 1 {
		newDailyUsage = 1
	}
	
	// Check if adding this subscription would exceed limits
	if currentHourlyUsage + int(newHourlyUsage) > sm.HourlyLimit {
		return false, fmt.Sprintf(
			"Adding this subscription would exceed the hourly API limit of %d requests. Current usage: %d, New subscription would add: %d requests per hour.",
			sm.HourlyLimit, currentHourlyUsage, newHourlyUsage)
	}
	
	if currentDailyUsage + int(newDailyUsage) > sm.DailyLimit {
		return false, fmt.Sprintf(
			"Adding this subscription would exceed the daily API limit of %d requests. Current usage: %d, New subscription would add: %d requests per day.",
			sm.DailyLimit, currentDailyUsage, newDailyUsage)
	}
	
	return true, ""
}

// WeatherCodeDescription maps weather codes to human-readable descriptions
var WeatherCodeDescription = map[int]string{
	1000: "Clear",
	1100: "Mostly Clear",
	1101: "Partly Cloudy",
	1102: "Mostly Cloudy",
	1001: "Cloudy",
	2000: "Fog",
	2100: "Light Fog",
	4000: "Drizzle",
	4001: "Rain",
	4200: "Light Rain",
	4201: "Heavy Rain",
	5000: "Snow",
	5001: "Flurries",
	5100: "Light Snow",
	5101: "Heavy Snow",
	6000: "Freezing Drizzle",
	6001: "Freezing Rain",
	6200: "Light Freezing Rain",
	6201: "Heavy Freezing Rain",
	7000: "Ice Pellets",
	7101: "Heavy Ice Pellets",
	7102: "Light Ice Pellets",
	8000: "Thunderstorm",
}

func main() {
	// Get API key from environment variable
	apiKey := os.Getenv("WEATHER_API_KEY")
	if apiKey == "" {
		log.Println("Warning: WEATHER_API_KEY environment variable not set, using default")
		apiKey = "c5AeEo7A30nZmTHZkCs0fQXT8JcUFWJC" // Fallback to default if not set
	}
	log.Printf("Using API key: %s", apiKey)

	// Create subscription manager with file path
	subscriptionsFile := "./subscriptions.json"
	subscriptionManager := NewSubscriptionManager(subscriptionsFile)
	log.Printf("Using subscriptions file: %s", subscriptionsFile)

	// Set up HTTP server
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})
	
	http.HandleFunc("/api-usage", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		
		hourlyUsage, dailyUsage := subscriptionManager.CalculateAPIUsage()
		
		usage := map[string]interface{}{
			"hourly_usage": hourlyUsage,
			"daily_usage": dailyUsage,
			"hourly_limit": subscriptionManager.HourlyLimit,
			"daily_limit": subscriptionManager.DailyLimit,
			"hourly_remaining": subscriptionManager.HourlyLimit - hourlyUsage,
			"daily_remaining": subscriptionManager.DailyLimit - dailyUsage,
			"subscription_count": len(subscriptionManager.Subscriptions),
		}
		
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(usage)
	})

	http.HandleFunc("/weather", func(w http.ResponseWriter, r *http.Request) {
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
	})

	http.HandleFunc("/incoming", func(w http.ResponseWriter, r *http.Request) {
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
		if text == "--help" {
			helpText := "**Weather Bot Commands**\n\n" +
				"**Basic Commands:**\n" +
				"- `/weather <location>` - Get current weather for a location\n" +
				"- `/weather help` - Show this help message\n" +
				"- `/weather limits` - Show API usage limits and current usage\n\n" +
				
				"**Subscription Commands:**\n" +
				"- `/weather --subscribe --location <location> --update-frequency <ms>` - Subscribe to weather updates\n" +
				"- `/weather --unsubscribe` - List your active subscriptions\n" +
				"- `/weather --unsubscribe --id <subscription_id>` - Unsubscribe from specific weather updates\n\n" +
				
				"**Parameters:**\n" +
				"- `location` - City name, zip code, or coordinates (e.g., 'New York', '10001', '40.7128,-74.0060')\n" +
				"- `update-frequency` - How often to send updates in milliseconds (e.g., 3600000 for hourly) or duration (e.g., 1h, 30m)\n" +
				"- `subscription_id` - ID of an active subscription\n\n" +
				
				"**Examples:**\n" +
				"- `/weather London` - Get current weather for London\n" +
				"- `/weather --subscribe --location Tokyo --update-frequency 3600000` - Get hourly weather updates for Tokyo\n" +
				"- `/weather --subscribe --location \"San Francisco\" --update-frequency 1h` - Get hourly weather updates for San Francisco"
			
			response := MattermostResponse{
				Text:         helpText,
				ResponseType: "ephemeral",
				ChannelID:    channelID,
			}
			json.NewEncoder(w).Encode(response)
			return
		} else if text == "limits" {
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
		// But don't treat "help" as a location
		if location == "" && !strings.Contains(text, "--") && text != "help" {
			location = text
		} else if text == "help" {
			// Handle help command
			helpText := "**Weather Bot Commands**\n\n" +
				"**Basic Commands:**\n" +
				"- `/weather <location>` - Get current weather for a location\n" +
				"- `/weather --help` - Show this help message\n" +
				"- `/weather limits` - Show API usage limits and current usage\n\n" +
				
				"**Subscription Commands:**\n" +
				"- `/weather --subscribe --location <location> --update-frequency <ms>` - Subscribe to weather updates\n" +
				"- `/weather --unsubscribe` - List your active subscriptions\n" +
				"- `/weather --unsubscribe --id <subscription_id>` - Unsubscribe from specific weather updates\n\n" +
				
				"**Parameters:**\n" +
				"- `location` - City name, zip code, or coordinates (e.g., 'New York', '10001', '40.7128,-74.0060')\n" +
				"- `update-frequency` - How often to send updates in milliseconds (e.g., 3600000 for hourly) or duration (e.g., 1h, 30m)\n" +
				"- `subscription_id` - ID of an active subscription\n\n" +
				
				"**Examples:**\n" +
				"- `/weather London` - Get current weather for London\n" +
				"- `/weather --subscribe --location Tokyo --update-frequency 3600000` - Get hourly weather updates for Tokyo\n" +
				"- `/weather --subscribe --location \"San Francisco\" --update-frequency 1h` - Get hourly weather updates for San Francisco"
			
			response := MattermostResponse{
				Text:         helpText,
				ResponseType: "ephemeral",
				ChannelID:    channelID,
			}
			json.NewEncoder(w).Encode(response)
			return
		}

		// Handle limits request (for backward compatibility with --limits flag)
		if showLimits {
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
			return
		}
		
		// Handle unsubscribe request
		if unsubscribe {
			if subscriptionID == "" {
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
			go func(sub *Subscription) {
				// Get initial weather data
				weatherData, err := getWeatherData(sub.Location, apiKey)
				if err != nil {
					log.Printf("Error fetching initial weather data for subscription %s: %v", sub.ID, err)
					return
				}

				// Send initial weather update
				weatherText := formatWeatherResponse(weatherData)
				sendMattermostMessage(sub.ChannelID, weatherText)

				// Create a ticker for periodic updates (convert milliseconds to duration)
				ticker := time.NewTicker(time.Duration(sub.UpdateFrequency) * time.Millisecond)
				defer ticker.Stop()

				for {
					select {
					case <-ticker.C:
						// Get updated weather data
						weatherData, err := getWeatherData(sub.Location, apiKey)
						if err != nil {
							log.Printf("Error fetching weather data for subscription %s: %v", sub.ID, err)
							continue
						}

						// Send weather update
						weatherText := formatWeatherResponse(weatherData)
						sendMattermostMessage(sub.ChannelID, weatherText)

						// Update last updated time
						sub.LastUpdated = time.Now()
						
						// Only save to file occasionally (every 6 hours) to reduce disk I/O
						if time.Since(sub.LastUpdated).Hours() >= 6 {
							subscriptionManager.SaveToFile()
						}
					case <-sub.StopChan:
						log.Printf("Stopping subscription %s", sub.ID)
						return
					}
				}
			}(subscription)

			// Send confirmation
			response := MattermostResponse{
				Text:         fmt.Sprintf("Successfully subscribed to weather updates for %s every %d ms. Subscription ID: `%s`", location, updateFrequency, subID),
				ResponseType: "ephemeral",
				ChannelID:    channelID,
			}
			json.NewEncoder(w).Encode(response)
			return
		}

		// Handle regular weather request
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

		log.Printf("Successfully sent weather response to channel: %s (%s)", channelName, channelID)
	})

	// Create a directory for static files if it doesn't exist
	staticDir := "./static"
	if _, err := os.Stat(staticDir); os.IsNotExist(err) {
		if err := os.MkdirAll(staticDir, 0755); err != nil {
			log.Fatalf("Failed to create static directory: %v", err)
		}
	}

	// Serve static files
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir(staticDir))))

	// Special handler for bot.png
	http.HandleFunc("/bot.png", func(w http.ResponseWriter, r *http.Request) {
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
	})

	// Start active subscriptions
	log.Printf("Starting %d saved subscriptions...", len(subscriptionManager.Subscriptions))
	for _, sub := range subscriptionManager.Subscriptions {
		// Initialize the stop channel
		sub.StopChan = make(chan struct{})

		// Start the subscription in a goroutine
		go func(sub *Subscription) {
			log.Printf("Starting saved subscription %s for location %s", sub.ID, sub.Location)

			// Get initial weather data
			weatherData, err := getWeatherData(sub.Location, apiKey)
			if err != nil {
				log.Printf("Error fetching initial weather data for subscription %s: %v", sub.ID, err)
				return
			}

			// Send initial weather update
			weatherText := formatWeatherResponse(weatherData)
			sendMattermostMessage(sub.ChannelID, weatherText)

			// Create a ticker for periodic updates (convert milliseconds to duration)
			ticker := time.NewTicker(time.Duration(sub.UpdateFrequency) * time.Millisecond)
			defer ticker.Stop()

			for {
				select {
				case <-ticker.C:
					// Get updated weather data
					weatherData, err := getWeatherData(sub.Location, apiKey)
					if err != nil {
						log.Printf("Error fetching weather data for subscription %s: %v", sub.ID, err)
						continue
					}

					// Send weather update
					weatherText := formatWeatherResponse(weatherData)
					sendMattermostMessage(sub.ChannelID, weatherText)

					// Update last updated time
					sub.LastUpdated = time.Now()
					
					// Only save to file occasionally (every 6 hours) to reduce disk I/O
					if time.Since(sub.LastUpdated).Hours() >= 6 {
						subscriptionManager.SaveToFile()
					}
				case <-sub.StopChan:
					log.Printf("Stopping subscription %s", sub.ID)
					return
				}
			}
		}(sub)
	}

	// Set up graceful shutdown to save subscriptions
	setupGracefulShutdown(subscriptionManager)

	// Start the server
	port := "8085"
	serverURL := fmt.Sprintf("http://0.0.0.0:%s", port)
	log.Printf("Server starting on port %s (listening on all interfaces)...", port)
	log.Printf("Bot image will be available at %s/bot.png", serverURL)
	if err := http.ListenAndServe("0.0.0.0:"+port, nil); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}

// getWeatherData fetches weather data from the Tomorrow.io API
func getWeatherData(location, apiKey string) (*WeatherResponse, error) {
	// Trim any leading/trailing whitespace from location
	location = strings.TrimSpace(location)

	// Construct the API URL with proper URL encoding
	encodedLocation := url.QueryEscape(location)
	apiURL := fmt.Sprintf("https://api.tomorrow.io/v4/weather/realtime?location=%s&apikey=%s", encodedLocation, apiKey)

	// Log the URL with the API key partially masked for security
	maskedKey := "****" + apiKey[len(apiKey)-4:]
	maskedURL := fmt.Sprintf("https://api.tomorrow.io/v4/weather/realtime?location=%s&apikey=%s", encodedLocation, maskedKey)
	log.Printf("Requesting weather data from: %s", maskedURL)

	// Create a new HTTP client
	client := &http.Client{}

	// Create a new request
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, err
	}

	// Add required headers
	req.Header.Add("accept", "application/json")

	// Send the request
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Read the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// Check if the response status code is not 200 OK
	if resp.StatusCode != http.StatusOK {
		log.Printf("API error: status code %d, body: %s", resp.StatusCode, string(body))
		return nil, fmt.Errorf("API returned status code %d: %s", resp.StatusCode, string(body))
	}

	// Parse the response
	var weatherData WeatherResponse
	if err := json.Unmarshal(body, &weatherData); err != nil {
		return nil, err
	}

	return &weatherData, nil
}

// sendMattermostMessage sends a message to a Mattermost channel
func sendMattermostMessage(channelID, text string) error {
	// Get the Mattermost webhook URL from environment variable
	webhookURL := os.Getenv("MATTERMOST_WEBHOOK_URL")
	if webhookURL == "" {
		return fmt.Errorf("MATTERMOST_WEBHOOK_URL environment variable not set")
	}
	
	// Create the webhook payload with channel parameter
	payload := map[string]interface{}{
		"text":         text,
		"channel":      channelID,  // This is the key parameter that controls where the message goes
		"username":     "Weather Bot",
		"icon_url":     os.Getenv("BOT_ICON_URL"),
	}
	
	// Convert to JSON
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("error marshaling webhook payload: %v", err)
	}
	
	// Send the request
	log.Printf("Sending webhook message to channel %s", channelID)
	resp, err := http.Post(webhookURL, "application/json", bytes.NewBuffer(payloadBytes))
	if err != nil {
		return fmt.Errorf("error sending webhook: %v", err)
	}
	defer resp.Body.Close()
	
	// Check response status
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("error from Mattermost webhook: status %d, body: %s", resp.StatusCode, string(body))
	}
	
	log.Printf("Successfully sent message to channel %s via webhook", channelID)
	return nil
}

// setupGracefulShutdown sets up signal handling for graceful shutdown
func setupGracefulShutdown(sm *SubscriptionManager) {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-c
		log.Println("Received shutdown signal, saving subscriptions...")
		if err := sm.SaveToFile(); err != nil {
			log.Printf("Error saving subscriptions: %v", err)
		} else {
			log.Println("Subscriptions saved successfully")
		}
		os.Exit(0)
	}()
}

// formatWeatherResponse creates a human-readable weather report
func formatWeatherResponse(weather *WeatherResponse) string {
	// Get weather condition description
	weatherDesc, ok := WeatherCodeDescription[weather.Data.Values.WeatherCode]
	if !ok {
		weatherDesc = "Unknown"
	}

	return fmt.Sprintf("Weather for %s:\n"+
		"Condition: %s\n"+
		"Temperature: %.1f°C (Feels like: %.1f°C)\n"+
		"Humidity: %d%%\n"+
		"Precipitation Probability: %d%%\n"+
		"Wind: %.1f km/h (Gusts: %.1f km/h)\n"+
		"Cloud Cover: %d%%",
		weather.Location.Name,
		weatherDesc,
		weather.Data.Values.Temperature,
		weather.Data.Values.TemperatureApparent,
		weather.Data.Values.Humidity,
		weather.Data.Values.PrecipitationProbability,
		weather.Data.Values.WindSpeed,
		weather.Data.Values.WindGust,
		weather.Data.Values.CloudCover)
}
