package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
)

func main() {
	http.HandleFunc("/weather", weatherHandler)
	http.HandleFunc("/incoming", mattermostHandler)

	fmt.Println("Listening on http://localhost:8085")
	err := http.ListenAndServe(":8085", nil)
	if err != nil {
		fmt.Println("Failed to start server:", err)
		os.Exit(1)
	}
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
	// We'll ignore the actual POST payload for now
	location := "wendell,nc"
	
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

	// Create a response for Mattermost
	response := map[string]interface{}{
		"response_type": "in_channel",
		"text": fmt.Sprintf("🌤️ Weather in Wendell, NC: %.1f°C and %s", temp, condition),
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
	case 4000:
		return "raining"
	default:
		return "unknown conditions"
	}
}
