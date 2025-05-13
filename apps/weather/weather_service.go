package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
)

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
