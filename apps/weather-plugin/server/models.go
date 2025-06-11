package main

import (
	"time"
)

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

type Subscription struct {
	ID              string        `json:"id"`
	Location        string        `json:"location"`
	ChannelID       string        `json:"channel_id"`
	UserID          string        `json:"user_id"`
	UpdateFrequency int64         `json:"update_frequency"`
	LastUpdated     time.Time     `json:"last_updated"`
	StopChan        chan struct{} `json:"-"`
}

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