package command

import (
	"fmt"
	"strings"

	"github.com/mattermost/mattermost/server/public/model"
)

// executeMissionListCommand handles the /mission list command
func (c *Handler) executeMissionListCommand(args *model.CommandArgs) (*model.CommandResponse, error) {
	// Get all missions
	missions, err := c.mission.GetAllMissions()
	if err != nil {
		c.client.Log.Error("Error getting missions", "error", err.Error())
		return &model.CommandResponse{
			ResponseType: model.CommandResponseTypeEphemeral,
			Text:         fmt.Sprintf("Error getting missions: %v", err),
		}, nil
	}

	if len(missions) == 0 {
		return &model.CommandResponse{
			ResponseType: model.CommandResponseTypeEphemeral,
			Text:         "No missions found.",
		}, nil
	}

	// Format as a table
	var sb strings.Builder
	sb.WriteString("# Current Missions\n\n")
	sb.WriteString("| Name | Callsign | Departure | Arrival | Status | Channel |\n")
	sb.WriteString("|------|----------|-----------|---------|--------|--------|\n")

	for _, mission := range missions {
		sb.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %s | ~%s |\n",
			mission.Name, mission.Callsign, mission.DepartureAirport, mission.ArrivalAirport,
			mission.Status, mission.ChannelName))
	}

	_, err = c.bot.PostMessageFromBot(args.ChannelId, sb.String())

	// Send the response
	return &model.CommandResponse{
		ResponseType: model.CommandResponseTypeEphemeral,
		Text:         "",
	}, nil
}
