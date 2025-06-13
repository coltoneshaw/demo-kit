package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/mattermost/mattermost/server/public/model"
)

type WeatherFormatter struct{}

func NewWeatherFormatter() *WeatherFormatter {
	return &WeatherFormatter{}
}

func (wf *WeatherFormatter) getWindDirection(windDir int) string {
	directions := []string{"N", "NNE", "NE", "ENE", "E", "ESE", "SE", "SSE", "S", "SSW", "SW", "WSW", "W", "WNW", "NW", "NNW"}
	index := int((float64(windDir)+11.25)/22.5) % 16
	return directions[index]
}

func (wf *WeatherFormatter) getWeatherDescription(weatherCode int) string {
	description, exists := WeatherCodeDescription[weatherCode]
	if !exists {
		return "Unknown"
	}
	return description
}

func (wf *WeatherFormatter) getLocationDisplay(weatherData *WeatherResponse) string {
	locationDisplay := weatherData.Location.Name
	if locationDisplay == "" {
		locationDisplay = fmt.Sprintf("%.2f,%.2f", weatherData.Location.Lat, weatherData.Location.Lon)
	}
	return locationDisplay
}

func (wf *WeatherFormatter) getAttachmentColor(weatherCode int) string {
	switch {
	case weatherCode >= 8000: // Thunderstorm
		return "#ff4444"
	case weatherCode >= 6000: // Freezing rain/ice
		return "#8844ff"
	case weatherCode >= 5000: // Snow
		return "#4488ff"
	case weatherCode >= 4000: // Rain
		return "#4488aa"
	case weatherCode >= 2000: // Fog
		return "#888888"
	case weatherCode >= 1100: // Cloudy
		return "#aaaaaa"
	default: // Clear
		return "#36a64f"
	}
}

func (wf *WeatherFormatter) FormatAsText(weatherData *WeatherResponse) string {
	locationDisplay := wf.getLocationDisplay(weatherData)
	description := wf.getWeatherDescription(weatherData.Data.Values.WeatherCode)
	windDirection := wf.getWindDirection(weatherData.Data.Values.WindDirection)

	weatherText := fmt.Sprintf("🌤️ **Weather for %s**\n\n", locationDisplay)
	weatherText += fmt.Sprintf("**Condition:** %s\n", description)
	weatherText += fmt.Sprintf("**Temperature:** %.1f°C (feels like %.1f°C)\n",
		weatherData.Data.Values.Temperature, weatherData.Data.Values.TemperatureApparent)
	weatherText += fmt.Sprintf("**Humidity:** %d%%\n", weatherData.Data.Values.Humidity)
	weatherText += fmt.Sprintf("**Wind:** %.1f km/h %s (gusts up to %.1f km/h)\n",
		weatherData.Data.Values.WindSpeed, windDirection, weatherData.Data.Values.WindGust)
	weatherText += fmt.Sprintf("**Cloud Cover:** %d%%\n", weatherData.Data.Values.CloudCover)
	weatherText += fmt.Sprintf("**Precipitation Chance:** %d%%\n", weatherData.Data.Values.PrecipitationProbability)

	if weatherData.Data.Values.RainIntensity > 0 {
		weatherText += fmt.Sprintf("**Rain Intensity:** %.1f mm/h\n", weatherData.Data.Values.RainIntensity)
	}

	return strings.TrimSpace(weatherText)
}

func (wf *WeatherFormatter) FormatAsAttachment(weatherData *WeatherResponse, channelID, botUserID string) *model.Post {
	locationDisplay := wf.getLocationDisplay(weatherData)
	description := wf.getWeatherDescription(weatherData.Data.Values.WeatherCode)
	windDirection := wf.getWindDirection(weatherData.Data.Values.WindDirection)

	fields := []*model.SlackAttachmentField{
		{
			Title: "Temperature",
			Value: fmt.Sprintf("%.1f°C (feels like %.1f°C)", weatherData.Data.Values.Temperature, weatherData.Data.Values.TemperatureApparent),
			Short: true,
		},
		{
			Title: "Condition",
			Value: description,
			Short: true,
		},
		{
			Title: "Humidity",
			Value: fmt.Sprintf("%d%%", weatherData.Data.Values.Humidity),
			Short: true,
		},
		{
			Title: "Wind",
			Value: fmt.Sprintf("%.1f km/h %s", weatherData.Data.Values.WindSpeed, windDirection),
			Short: true,
		},
		{
			Title: "Cloud Cover",
			Value: fmt.Sprintf("%d%%", weatherData.Data.Values.CloudCover),
			Short: true,
		},
		{
			Title: "Precipitation Chance",
			Value: fmt.Sprintf("%d%%", weatherData.Data.Values.PrecipitationProbability),
			Short: true,
		},
	}

	if weatherData.Data.Values.RainIntensity > 0 {
		fields = append(fields, &model.SlackAttachmentField{
			Title: "Rain Intensity",
			Value: fmt.Sprintf("%.1f mm/h", weatherData.Data.Values.RainIntensity),
			Short: true,
		})
	}

	if weatherData.Data.Values.WindGust > 0 {
		fields = append(fields, &model.SlackAttachmentField{
			Title: "Wind Gusts",
			Value: fmt.Sprintf("%.1f km/h", weatherData.Data.Values.WindGust),
			Short: true,
		})
	}

	attachment := &model.SlackAttachment{
		Title:     fmt.Sprintf("🌤️ Weather for %s", locationDisplay),
		Color:     wf.getAttachmentColor(weatherData.Data.Values.WeatherCode),
		Fields:    fields,
		Footer:    "Weather Bot",
		Timestamp: time.Now().Format(time.RFC3339),
	}

	return &model.Post{
		ChannelId: channelID,
		UserId:    botUserID,
		Props: map[string]any{
			"attachments": []*model.SlackAttachment{attachment},
		},
	}
}