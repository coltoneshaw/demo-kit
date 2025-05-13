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
	"path/filepath"
	"strings"
	"sync"
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
	ID               string        // Unique identifier for the subscription
	Location         string        // Location to get weather for
	ChannelID        string        // Channel to post updates to
	UserID           string        // User who created the subscription
	UpdateFrequency  time.Duration // How often to update
	LastUpdated      time.Time     // When the subscription was last updated
	StopChan         chan struct{} // Channel to signal stopping the subscription
}

// SubscriptionManager manages all active subscriptions
type SubscriptionManager struct {
	Subscriptions map[string]*Subscription // Map of subscription ID to subscription
	Mutex         sync.RWMutex             // Mutex to protect the map
}

// NewSubscriptionManager creates a new subscription manager
func NewSubscriptionManager() *SubscriptionManager {
	return &SubscriptionManager{
		Subscriptions: make(map[string]*Subscription),
	}
}

// AddSubscription adds a new subscription
func (sm *SubscriptionManager) AddSubscription(sub *Subscription) {
	sm.Mutex.Lock()
	defer sm.Mutex.Unlock()
	sm.Subscriptions[sub.ID] = sub
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
	
	// Create subscription manager
	subscriptionManager := NewSubscriptionManager()

	// Set up HTTP server
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
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
		
		// Parse command flags
		var location string
		var subscribe bool
		var unsubscribe bool
		var updateFrequency time.Duration
		var subscriptionID string
		
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
					var err error
					updateFrequency, err = time.ParseDuration(words[i+1])
					if err != nil {
						log.Printf("Invalid update frequency: %s", words[i+1])
						response := MattermostResponse{
							Text:         fmt.Sprintf("Invalid update frequency: %s. Please use a valid duration like 30s, 5m, 1h", words[i+1]),
							ResponseType: "ephemeral",
							ChannelID:    channelID,
						}
						json.NewEncoder(w).Encode(response)
						return
					}
					i++ // Skip the frequency value
				}
			case "--id":
				if i+1 < len(words) {
					subscriptionID = words[i+1]
					i++ // Skip the ID value
				}
			}
		}
		
		// If no flags were used, treat the entire text as location
		if location == "" && !strings.Contains(text, "--") {
			location = text
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
					subList.WriteString(fmt.Sprintf("ID: `%s`\nLocation: %s\nFrequency: %s\n\n", 
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
				Text:         "Please provide a location. Example: `/weather New York` or `/weather --location London, UK --subscribe --update-frequency 30m`",
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
				updateFrequency = 1 * time.Hour // Default to hourly updates
			}
			
			// Validate minimum update frequency
			if updateFrequency < 30*time.Second {
				response := MattermostResponse{
					Text:         "Update frequency must be at least 30 seconds.",
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
				
				// Create a ticker for periodic updates
				ticker := time.NewTicker(sub.UpdateFrequency)
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
					case <-sub.StopChan:
						log.Printf("Stopping subscription %s", sub.ID)
						return
					}
				}
			}(subscription)
			
			// Send confirmation
			response := MattermostResponse{
				Text:         fmt.Sprintf("Successfully subscribed to weather updates for %s every %s. Subscription ID: `%s`", location, updateFrequency, subID),
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
	// Create the message payload
	response := MattermostResponse{
		Text:         text,
		ResponseType: "in_channel",
		ChannelID:    channelID,
	}
	
	// Convert to JSON
	payload, err := json.Marshal(response)
	if err != nil {
		return fmt.Errorf("error marshaling message: %v", err)
	}
	
	// Get the Mattermost webhook URL from environment variable
	webhookURL := os.Getenv("MATTERMOST_WEBHOOK_URL")
	if webhookURL == "" {
		return fmt.Errorf("MATTERMOST_WEBHOOK_URL environment variable not set")
	}
	
	// Send the request
	resp, err := http.Post(webhookURL, "application/json", bytes.NewBuffer(payload))
	if err != nil {
		return fmt.Errorf("error sending message: %v", err)
	}
	defer resp.Body.Close()
	
	// Check response status
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("error from Mattermost: status %d, body: %s", resp.StatusCode, string(body))
	}
	
	log.Printf("Successfully sent message to channel %s", channelID)
	return nil
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
