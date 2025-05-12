package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
)

// WeatherResponse represents the response from the Tomorrow.io API
type WeatherResponse struct {
	Data struct {
		Time   string `json:"time"`
		Values struct {
			Temperature            float64 `json:"temperature"`
			TemperatureApparent    float64 `json:"temperatureApparent"`
			Humidity               float64 `json:"humidity"`
			PrecipitationProbability int     `json:"precipitationProbability"`
			RainIntensity          float64 `json:"rainIntensity"`
			WindSpeed              float64 `json:"windSpeed"`
			WindGust               float64 `json:"windGust"`
			WindDirection          int     `json:"windDirection"`
			CloudCover             int     `json:"cloudCover"`
			WeatherCode            int     `json:"weatherCode"`
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
}

func main() {
	// Get API key from environment variable
	apiKey := os.Getenv("WEATHER_API_KEY")
	if apiKey == "" {
		apiKey = "c5AeEo7A30nZmTHZkCs0fQXT8JcUFWJC" // Fallback to default if not set
		log.Println("Warning: WEATHER_API_KEY environment variable not set, using default")
	}

	// Set up HTTP server
	http.HandleFunc("/weather", func(w http.ResponseWriter, r *http.Request) {
		// Get location from query parameter, default to Wendell, NC
		location := r.URL.Query().Get("location")
		if location == "" {
			location = "27591 us" // Default to Wendell, NC
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
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Parse the incoming webhook payload
		var payload MattermostPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, "Invalid request payload", http.StatusBadRequest)
			return
		}

		// Get weather data for Wendell, NC
		weatherData, err := getWeatherData("27591 us", apiKey)
		if err != nil {
			http.Error(w, fmt.Sprintf("Error fetching weather data: %v", err), http.StatusInternalServerError)
			return
		}

		// Create a formatted response
		weatherText := formatWeatherResponse(weatherData)
		response := MattermostResponse{
			Text:         weatherText,
			ResponseType: "in_channel", // Make the response visible to everyone in the channel
		}

		// Set response headers
		w.Header().Set("Content-Type", "application/json")
		
		// Return the response
		json.NewEncoder(w).Encode(response)
	})

	// Start the server
	port := "8085"
	log.Printf("Server starting on port %s...", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}

// getWeatherData fetches weather data from the Tomorrow.io API
func getWeatherData(location, apiKey string) (*WeatherResponse, error) {
	// Construct the API URL
	url := fmt.Sprintf("https://api.tomorrow.io/v4/weather/realtime?location=%s&apikey=%s", location, apiKey)

	// Create a new HTTP client
	client := &http.Client{}

	// Create a new request
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	// Add headers
	req.Header.Add("accept", "application/json")
	req.Header.Add("accept-encoding", "deflate, gzip, br")

	// Send the request
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Read the response body
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// Check if the response status code is not 200 OK
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned status code %d: %s", resp.StatusCode, string(body))
	}

	// Parse the response
	var weatherData WeatherResponse
	if err := json.Unmarshal(body, &weatherData); err != nil {
		return nil, err
	}

	return &weatherData, nil
}

// formatWeatherResponse creates a human-readable weather report
func formatWeatherResponse(weather *WeatherResponse) string {
	return fmt.Sprintf("Weather for %s:\n"+
		"Temperature: %.1f°C (Feels like: %.1f°C)\n"+
		"Humidity: %d%%\n"+
		"Precipitation Probability: %d%%\n"+
		"Wind: %.1f km/h (Gusts: %.1f km/h)\n"+
		"Cloud Cover: %d%%",
		weather.Location.Name,
		weather.Data.Values.Temperature,
		weather.Data.Values.TemperatureApparent,
		weather.Data.Values.Humidity,
		weather.Data.Values.PrecipitationProbability,
		weather.Data.Values.WindSpeed,
		weather.Data.Values.WindGust,
		weather.Data.Values.CloudCover)
}
