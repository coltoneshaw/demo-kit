package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
)

func main() {
	// Command-line flags for testing
	testMode := flag.Bool("test", false, "Run a test query instead of starting the server")
	testLocation := flag.String("location", "raleigh,nc", "Location to test in test mode")
	flag.Parse()

	// Test mode - just fetch and display weather for a location
	if *testMode {
		weather, err := fetchWeather(*testLocation)
		if err != nil {
			log.Fatalf("Error fetching weather: %v", err)
		}
		
		// Extract and display weather information
		data, ok := weather["data"].(map[string]interface{})
		if !ok {
			log.Fatalf("Invalid response format")
		}
		
		values, ok := data["values"].(map[string]interface{})
		if !ok {
			log.Fatalf("Invalid values format")
		}
		
		temp, ok := values["temperature"].(float64)
		if !ok {
			log.Fatalf("Temperature data not found")
		}
		
		weatherCode, ok := values["weatherCode"].(float64)
		if !ok {
			log.Fatalf("Weather code not found")
		}
		
		code := int(weatherCode)
		condition := mapWeatherCode(code)
		
		fmt.Printf("Weather for %s: %.1f°C and %s\n", *testLocation, temp, condition)
		return
	}

	// Normal server mode
	http.HandleFunc("/weather", weatherHandler)
	http.HandleFunc("/incoming", mattermostHandler)

	// Get port from environment or use default
	port := getPort()
	
	fmt.Printf("Listening on http://localhost:%s\n", port)
	fmt.Println("Press Ctrl+C to stop the server")
	err := http.ListenAndServe(":"+port, nil)
	if err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

// getPort returns the port from environment variable or default 8085
func getPort() string {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8085"
	}
	
	// Validate port is a number
	if _, err := strconv.Atoi(port); err != nil {
		log.Printf("Invalid PORT value: %s, using default 8085", port)
		return "8085"
	}
	
	return port
}

func getAPIKey() (apiKey string, err error) {
	apiKey = os.Getenv("TOMORROW_API_KEY")
	if apiKey == "" {
		return "", fmt.Errorf("API key not set in TOMORROW_API_KEY")
	}

	return apiKey, nil
}

// fetchWeather gets weather data for a location from the Tomorrow.io API
func fetchWeather(location string) (map[string]interface{}, error) {
	apiKey, err := getAPIKey()
	if err != nil {
		return nil, err
	}

	escapedLocation := url.QueryEscape(location)
	apiURL := fmt.Sprintf("https://api.tomorrow.io/v4/weather/realtime?location=%s&apikey=%s", escapedLocation, apiKey)
	
	resp, err := http.Get(apiURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch weather data: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API request failed: %s", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var jsonResponse map[string]interface{}
	if err := json.Unmarshal(body, &jsonResponse); err != nil {
		return nil, fmt.Errorf("failed to parse weather response: %w", err)
	}

	return jsonResponse, nil
}

func weatherHandler(w http.ResponseWriter, r *http.Request) {
	location := r.URL.Query().Get("location")
	if location == "" {
		http.Error(w, "Missing 'location' query parameter", http.StatusBadRequest)
		return
	}

	jsonResponse, err := fetchWeather(location)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(jsonResponse)
}

func mattermostHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	
	// Default location
	location := "wendell,nc"
	
	// Try to parse the Mattermost payload to extract a custom location
	var payload map[string]interface{}
	
	if err := json.NewDecoder(r.Body).Decode(&payload); err == nil {
		// Check if this is a slash command with text
		if text, ok := payload["text"].(string); ok && text != "" {
			trimmedText := strings.TrimSpace(text)
			if trimmedText != "" {
				location = trimmedText
				log.Printf("Using custom location from slash command: %s", location)
			}
		}
	}
	
	jsonResponse, err := fetchWeather(location)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Extract data
	data, ok := jsonResponse["data"].(map[string]interface{})
	if !ok {
		http.Error(w, "Invalid response format", http.StatusInternalServerError)
		return
	}
	
	values, ok := data["values"].(map[string]interface{})
	if !ok {
		http.Error(w, "Invalid values format", http.StatusInternalServerError)
		return
	}
	
	temp, ok := values["temperature"].(float64)
	if !ok {
		http.Error(w, "Temperature data not found", http.StatusInternalServerError)
		return
	}
	
	weatherCode, ok := values["weatherCode"].(float64)
	if !ok {
		http.Error(w, "Weather code not found", http.StatusInternalServerError)
		return
	}
	
	code := int(weatherCode)
	condition := mapWeatherCode(code)

	// Format location for display
	displayLocation := strings.ReplaceAll(location, ",", ", ")
	displayLocation = strings.Title(strings.ToLower(displayLocation))
	
	// Create a response for Mattermost
	response := map[string]interface{}{
		"response_type": "in_channel",
		"text": fmt.Sprintf("🌤️ Weather in %s: %.1f°C and %s", displayLocation, temp, condition),
	}

	// Respond with JSON for Mattermost
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func mapWeatherCode(code int) string {
	switch code {
	case 1000:
		return "clear"
	case 1100:
		return "mostly clear"
	case 1101:
		return "partly cloudy"
	case 1102:
		return "mostly cloudy"
	case 1103:
		return "cloudy"
	case 2000:
		return "fog"
	case 2100:
		return "light fog"
	case 3000:
		return "light wind"
	case 3001:
		return "wind"
	case 3002:
		return "strong wind"
	case 4000:
		return "drizzle"
	case 4001:
		return "rain"
	case 4200:
		return "light rain"
	case 4201:
		return "heavy rain"
	case 5000:
		return "snow"
	case 5001:
		return "flurries"
	case 5100:
		return "light snow"
	case 5101:
		return "heavy snow"
	case 6000:
		return "freezing drizzle"
	case 6001:
		return "freezing rain"
	case 6200:
		return "light freezing rain"
	case 6201:
		return "heavy freezing rain"
	case 7000:
		return "ice pellets"
	case 7101:
		return "heavy ice pellets"
	case 7102:
		return "light ice pellets"
	case 8000:
		return "thunderstorm"
	default:
		return "unknown conditions"
	}
}
