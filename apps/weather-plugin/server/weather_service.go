package main

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type WeatherService struct {
	weatherData []WeatherValues
	bundlePath  string
}

type WeatherValues struct {
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
}

func NewWeatherService(bundlePath string) *WeatherService {
	ws := &WeatherService{
		bundlePath: bundlePath,
	}
	ws.loadWeatherData()
	return ws
}

func (ws *WeatherService) loadWeatherData() error {
	weatherFile := filepath.Join(ws.bundlePath, "assets", "weather.json")
	data, err := os.ReadFile(weatherFile)
	if err != nil {
		return fmt.Errorf("failed to read weather.json: %v", err)
	}

	if err := json.Unmarshal(data, &ws.weatherData); err != nil {
		return fmt.Errorf("failed to parse weather.json: %v", err)
	}

	return nil
}

func (ws *WeatherService) GetWeatherData(location string) (*WeatherResponse, error) {
	if len(ws.weatherData) == 0 {
		// Try to reload data if empty
		if err := ws.loadWeatherData(); err != nil {
			return nil, fmt.Errorf("no weather data available: %v", err)
		}
	}

	// Pick a random weather entry
	randomIndex := rand.Intn(len(ws.weatherData))
	weatherValues := ws.weatherData[randomIndex]
	
	// Create a WeatherResponse with the user's requested location
	weatherResponse := &WeatherResponse{
		Data: struct {
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
		}{
			Time: time.Now().Format(time.RFC3339),
			Values: struct {
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
			}{
				Temperature:              weatherValues.Temperature,
				TemperatureApparent:      weatherValues.TemperatureApparent,
				Humidity:                 weatherValues.Humidity,
				PrecipitationProbability: weatherValues.PrecipitationProbability,
				RainIntensity:            weatherValues.RainIntensity,
				WindSpeed:                weatherValues.WindSpeed,
				WindGust:                 weatherValues.WindGust,
				WindDirection:            weatherValues.WindDirection,
				CloudCover:               weatherValues.CloudCover,
				WeatherCode:              weatherValues.WeatherCode,
			},
		},
		Location: struct {
			Lat  float64 `json:"lat"`
			Lon  float64 `json:"lon"`
			Name string  `json:"name"`
			Type string  `json:"type"`
		}{
			Lat:  0,  // Not used anymore
			Lon:  0,  // Not used anymore
			Name: location,
			Type: "city",
		},
	}
	
	return weatherResponse, nil
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