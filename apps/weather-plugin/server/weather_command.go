package main

import (
	"fmt"

	"github.com/mattermost/mattermost/server/public/model"
)

type WeatherCommand struct {
	weatherService *WeatherService
	formatter      *WeatherFormatter
	messageService *MessageService
}

func NewWeatherCommand(weatherService *WeatherService, formatter *WeatherFormatter, messageService *MessageService) *WeatherCommand {
	return &WeatherCommand{
		weatherService: weatherService,
		formatter:      formatter,
		messageService: messageService,
	}
}

func (wc *WeatherCommand) Execute(args *model.CommandArgs, location string) (*model.CommandResponse, error) {
	if location == "" {
		return wc.messageService.SendEphemeralResponse(args, "Please provide a location. Example: `/weather New York` or use `/weather help` for more commands.")
	}

	weatherData, err := wc.weatherService.GetWeatherData(location)
	if err != nil {
		return wc.messageService.SendEphemeralResponse(args, fmt.Sprintf("Error fetching weather data: %v", err))
	}

	post := wc.formatter.FormatAsAttachment(weatherData, args.ChannelId, wc.messageService.GetBotUserID())
	return wc.messageService.SendPublicResponse(args, post)
}