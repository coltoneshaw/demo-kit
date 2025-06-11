package main

import (
	"sync"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"
	"github.com/mattermost/mattermost/server/public/pluginapi"
	"github.com/pkg/errors"
)

type Plugin struct {
	plugin.MattermostPlugin

	configurationLock sync.RWMutex
	configuration     *configuration
	client            *pluginapi.Client

	weatherService      *WeatherService
	subscriptionManager *SubscriptionManager
	commandHandler      *CommandHandler
	formatter           *WeatherFormatter
	messageService      *MessageService
	botUserID           string
}

func (p *Plugin) OnActivate() error {
	pluginAPIClient := pluginapi.NewClient(p.API, p.Driver)
	p.client = pluginAPIClient

	bundlePath, err := p.API.GetBundlePath()
	if err != nil {
		return errors.Wrap(err, "failed to get bundle path")
	}

	// Create or get the bot user
	botUserID, err := p.ensureBotUser()
	if err != nil {
		return errors.Wrap(err, "failed to ensure bot user")
	}
	p.botUserID = botUserID

	p.weatherService = NewWeatherService(bundlePath)
	p.formatter = NewWeatherFormatter()
	p.messageService = NewMessageService(p.client, p.botUserID)
	p.subscriptionManager = NewSubscriptionManager(p.client, p.weatherService, p.formatter, p.messageService)
	p.commandHandler = NewCommandHandler(p.client, p.weatherService, p.subscriptionManager, p.formatter, p.messageService)

	p.client.Log.Info("Weather plugin activated", "bundle_path", bundlePath, "bot_user_id", p.botUserID)
	return nil
}

func (p *Plugin) OnDeactivate() error {
	if p.subscriptionManager != nil {
		p.subscriptionManager.StopAll()
	}
	return nil
}

func (p *Plugin) ExecuteCommand(c *plugin.Context, args *model.CommandArgs) (*model.CommandResponse, *model.AppError) {
	response, err := p.commandHandler.Handle(args)
	if err != nil {
		p.client.Log.Error("Error executing command", "error", err.Error())
		return &model.CommandResponse{
			ResponseType: model.CommandResponseTypeEphemeral,
			Text:         err.Error(),
		}, nil
	}
	return response, nil
}

func (p *Plugin) OnConfigurationChange() error {
	var configuration = new(configuration)

	if err := p.API.LoadPluginConfiguration(configuration); err != nil {
		return errors.Wrap(err, "failed to load plugin configuration")
	}

	p.setConfiguration(configuration)

	return nil
}


func (p *Plugin) setConfiguration(configuration *configuration) {
	p.configurationLock.Lock()
	defer p.configurationLock.Unlock()

	p.configuration = configuration
}

func (p *Plugin) ensureBotUser() (string, error) {
	botID, err := p.client.Bot.EnsureBot(&model.Bot{
		Username:    "weatherbot",
		DisplayName: "Weather Bot",
		Description: "A bot that provides weather information and updates",
	}, pluginapi.ProfileImagePath("/assets/bot.png"))

	if err != nil {
		return "", errors.Wrap(err, "failed to ensure bot user")
	}

	p.client.Log.Info("Weather bot ensured", "bot_id", botID)
	return botID, nil
}

func main() {
	plugin.ClientMain(&Plugin{})
}