package main

import (
	"net/http"
	"sync"

	"github.com/coltoneshaw/demokit/flightaware-plugin/server/command"
	"github.com/coltoneshaw/demokit/flightaware-plugin/server/flight"
	"github.com/coltoneshaw/demokit/flightaware-plugin/server/subscription"
	"github.com/gorilla/mux"
	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"
	"github.com/mattermost/mattermost/server/public/pluginapi"
	"github.com/pkg/errors"
)

type Plugin struct {
	plugin.MattermostPlugin

	configurationLock sync.RWMutex
	configuration     *configuration

	client           *pluginapi.Client
	commandClient    command.Command
	flightService    flight.FlightInterface
	subscriptionMgr  subscription.SubscriptionInterface
	messageService   *MessageService
	botUserID        string
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

	// Initialize services with bot user ID
	flightService, err := flight.NewFlightService(bundlePath)
	if err != nil {
		return errors.Wrap(err, "failed to initialize flight service")
	}
	p.flightService = flightService
	
	p.messageService = NewMessageService(p.client, p.botUserID)
	
	subscriptionMgr, err := subscription.NewSubscriptionManager(p.client, p.flightService, p.messageService)
	if err != nil {
		return errors.Wrap(err, "failed to initialize subscription manager")
	}
	p.subscriptionMgr = subscriptionMgr
	
	p.commandClient = command.NewCommandHandler(p.client, p.flightService, p.subscriptionMgr, p.messageService)

	p.client.Log.Info("Flight plugin activated", "bundle_path", bundlePath, "bot_user_id", p.botUserID)
	return nil
}

func (p *Plugin) ensureBotUser() (string, error) {
	botID, err := p.client.Bot.EnsureBot(&model.Bot{
		Username:    "flightsbot",
		DisplayName: "Flights Bot",
		Description: "A bot that provides flight departure information and updates",
	}, pluginapi.ProfileImagePath("/assets/bot_icon.png"))

	if err != nil {
		return "", errors.Wrap(err, "failed to ensure bot user")
	}

	p.client.Log.Info("Flights bot ensured", "bot_id", botID)
	return botID, nil
}

func (p *Plugin) OnDeactivate() error {
	if p.subscriptionMgr != nil {
		p.subscriptionMgr.StopAll()
	}
	return nil
}

func (p *Plugin) ExecuteCommand(c *plugin.Context, args *model.CommandArgs) (*model.CommandResponse, *model.AppError) {
	response, err := p.commandClient.Handle(args)
	if err != nil {
		p.client.Log.Error("Error executing command", "error", err.Error())
		return &model.CommandResponse{
			ResponseType: model.CommandResponseTypeEphemeral,
			Text:         err.Error(),
		}, nil
	}
	return response, nil
}

func (p *Plugin) ServeHTTP(c *plugin.Context, w http.ResponseWriter, r *http.Request) {
	router := mux.NewRouter()
	router.ServeHTTP(w, r)
}

func main() {
	plugin.ClientMain(&Plugin{})
}