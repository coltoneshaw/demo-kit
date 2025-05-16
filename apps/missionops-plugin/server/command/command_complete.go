package command

import (
	"fmt"
	"time"

	"github.com/mattermost/mattermost/server/public/model"
)

// executeMissionCompleteCommand handles the /mission complete command
func (c *Handler) executeMissionCompleteCommand(args *model.CommandArgs) (*model.CommandResponse, error) {
	// Parse arguments
	commandArgs := parseArgs(args.Command)
	missionID := commandArgs["id"]

	// If no mission ID provided, try to find the mission based on channel ID
	if missionID == "" {
		mission, err := c.mission.GetMissionByChannelID(args.ChannelId)
		if err != nil {
			return &model.CommandResponse{
				ResponseType: model.CommandResponseTypeEphemeral,
				Text:         "This command must be run in a mission channel, or provide --id [mission_id]",
			}, nil
		}
		missionID = mission.ID
	} else {
		// Get the mission by ID
		_, err := c.mission.GetMission(missionID)
		if err != nil {
			return &model.CommandResponse{
				ResponseType: model.CommandResponseTypeEphemeral,
				Text:         "Mission not found with the provided ID.",
			}, nil
		}
	}

	// Set status to completed
	if err := c.mission.UpdateMissionStatus(missionID, "completed"); err != nil {
		c.client.Log.Error("Error updating mission status", "error", err.Error())
		return &model.CommandResponse{
			ResponseType: model.CommandResponseTypeEphemeral,
			Text:         fmt.Sprintf("Error completing mission: %v", err),
		}, nil
	}

	// Get the updated mission
	mission, err := c.mission.GetMission(missionID)
	if err != nil {
		return &model.CommandResponse{
			ResponseType: model.CommandResponseTypeEphemeral,
			Text:         "Mission not found after update.",
		}, nil
	}

	// Format post-mission report
	report := fmt.Sprintf("# Mission Completed: %s\n\n"+
		"**Callsign:** %s\n"+
		"**Departure:** %s\n"+
		"**Arrival:** %s\n"+
		"**Completed At:** %s\n",
		mission.Name, mission.Callsign, mission.DepartureAirport, mission.ArrivalAirport, time.Now().Format(time.RFC1123))

	// Post to mission channel
	_, reportPost := c.bot.PostMessageFromBot(mission.ChannelID, report)

	if reportPost == nil {
		c.client.Log.Error("Error sending report to mission channel", "error", err.Error())
	}

	// Return a success message
	return &model.CommandResponse{
		ResponseType: model.CommandResponseTypeInChannel,
		Text:         fmt.Sprintf("✅ Mission **%s** marked as completed", mission.Name),
	}, nil
}
