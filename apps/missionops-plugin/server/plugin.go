package main

import (
	"net/http"
	"sync"

	"github.com/coltoneshaw/demokit/missionops-plugin/server/bot"
	"github.com/coltoneshaw/demokit/missionops-plugin/server/command"
	"github.com/coltoneshaw/demokit/missionops-plugin/server/mission"
	"github.com/coltoneshaw/demokit/missionops-plugin/server/subscription"
	"github.com/gorilla/mux"
	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"
	"github.com/mattermost/mattermost/server/public/pluginapi"
	"github.com/pkg/errors"
)

// Plugin implements the interface expected by the Mattermost server to communicate between the server and plugin processes.
type Plugin struct {
	plugin.MattermostPlugin

	// configurationLock synchronizes access to the configuration.
	configurationLock sync.RWMutex

	// configuration is the active plugin configuration. Consult getConfiguration and
	// setConfiguration for usage.
	configuration *configuration

	client *pluginapi.Client

	commandClient command.Command

	bot          bot.BotInterface
	mission      mission.MissionInterface
	subscription subscription.SubscriptionInterface
}

// OnActivate is invoked when the plugin is activated.
func (p *Plugin) OnActivate() error {
	pluginAPIClient := pluginapi.NewClient(p.API, p.Driver)
	p.client = pluginAPIClient

	p.bot = bot.NewBotHandler(p.client)
	if p.bot == nil {
		return errors.New("failed to create bot handler")
	}

	bundlePath, err := p.API.GetBundlePath()

	if err != nil {
		return errors.Wrap(err, "failed to get bundle path")
	}

	p.bot.SetBundlePath(bundlePath)

	p.mission = mission.NewMissionHandler(p.client, p.bot)
	p.subscription = subscription.NewSubscriptionManager(p.client, p.bot, p.mission)
	p.commandClient = command.NewCommandHandler(p.client, p.mission, p.bot, p.subscription)

	// // Initialize subscription manager
	// if err := p.initSubscriptionManager(); err != nil {
	// 	return errors.Wrap(err, "failed to initialize subscription manager")
	// }

	return nil
}

// OnDeactivate is invoked when the plugin is deactivated.
func (p *Plugin) OnDeactivate() error {

	return nil
}

// ExecuteCommand handles slash commands registered by this plugin
func (p *Plugin) ExecuteCommand(c *plugin.Context, args *model.CommandArgs) (*model.CommandResponse, *model.AppError) {
	// Forward the command to your command handler
	response, err := p.commandClient.Handle(args)

	if err != nil {
		p.client.Log.Error("Error executing command", "error", err.Error())

		// Return a user-friendly error message instead of a 500 error
		return &model.CommandResponse{
			ResponseType: model.CommandResponseTypeEphemeral,
			Text:         err.Error(),
		}, nil
	}
	return response, nil
}

// ServeHTTP implements the http.Handler interface for the plugin
func (p *Plugin) ServeHTTP(c *plugin.Context, w http.ResponseWriter, r *http.Request) {
	router := mux.NewRouter()

	// API routes
	router.HandleFunc("/api/v1/missions/{mission_id}/complete", p.commandClient.HandleMissionComplete).Methods("POST")

	router.ServeHTTP(w, r)
}

func main() {
	plugin.ClientMain(&Plugin{})
}
