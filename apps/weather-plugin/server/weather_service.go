package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

type WeatherService struct {
	apiKey string
}

func NewWeatherService(apiKey string) *WeatherService {
	return &WeatherService{
		apiKey: apiKey,
	}
}

func (ws *WeatherService) UpdateAPIKey(apiKey string) {
	ws.apiKey = apiKey
}

func (ws *WeatherService) GetWeatherData(location string) (*WeatherResponse, error) {
	if ws.apiKey == "" {
		return nil, fmt.Errorf("tomorrow.io API key not configured")
	}

	url := fmt.Sprintf("https://api.tomorrow.io/v4/weather/realtime?location=%s&apikey=%s",
		url.QueryEscape(location), ws.apiKey)

	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch weather data: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("weather API returned status %d", resp.StatusCode)
	}

	var weatherData WeatherResponse
	if err := json.NewDecoder(resp.Body).Decode(&weatherData); err != nil {
		return nil, fmt.Errorf("failed to decode weather data: %v", err)
	}

	return &weatherData, nil
}

func (ws *WeatherService) FormatWeatherResponse(data *WeatherResponse) string {
	description, exists := WeatherCodeDescription[data.Data.Values.WeatherCode]
	if !exists {
		description = "Unknown"
	}

	var windDirection string
	windDir := data.Data.Values.WindDirection
	directions := []string{"N", "NNE", "NE", "ENE", "E", "ESE", "SE", "SSE", "S", "SSW", "SW", "WSW", "W", "WNW", "NW", "NNW"}
	index := int((float64(windDir)+11.25)/22.5) % 16
	windDirection = directions[index]

	locationDisplay := data.Location.Name
	if locationDisplay == "" {
		locationDisplay = fmt.Sprintf("%.2f,%.2f", data.Location.Lat, data.Location.Lon)
	}

	weatherText := fmt.Sprintf("🌤️ **Weather for %s**\n\n", locationDisplay)
	weatherText += fmt.Sprintf("**Condition:** %s\n", description)
	weatherText += fmt.Sprintf("**Temperature:** %.1f°C (feels like %.1f°C)\n",
		data.Data.Values.Temperature, data.Data.Values.TemperatureApparent)
	weatherText += fmt.Sprintf("**Humidity:** %d%%\n", data.Data.Values.Humidity)
	weatherText += fmt.Sprintf("**Wind:** %.1f km/h %s (gusts up to %.1f km/h)\n",
		data.Data.Values.WindSpeed, windDirection, data.Data.Values.WindGust)
	weatherText += fmt.Sprintf("**Cloud Cover:** %d%%\n", data.Data.Values.CloudCover)
	weatherText += fmt.Sprintf("**Precipitation Chance:** %d%%\n", data.Data.Values.PrecipitationProbability)

	if data.Data.Values.RainIntensity > 0 {
		weatherText += fmt.Sprintf("**Rain Intensity:** %.1f mm/h\n", data.Data.Values.RainIntensity)
	}

	return strings.TrimSpace(weatherText)
}