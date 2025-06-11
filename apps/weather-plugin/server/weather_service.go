package main

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
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
	weatherResponse := ws.buildWeatherResponse(weatherValues, location)
	
	return weatherResponse, nil
}

func (ws *WeatherService) buildWeatherResponse(values WeatherValues, location string) *WeatherResponse {
	return &WeatherResponse{
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
			Time:   time.Now().Format(time.RFC3339),
			Values: values,
		},
		Location: struct {
			Lat  float64 `json:"lat"`
			Lon  float64 `json:"lon"`
			Name string  `json:"name"`
			Type string  `json:"type"`
		}{
			Lat:  0,
			Lon:  0,
			Name: location,
			Type: "city",
		},
	}
}

