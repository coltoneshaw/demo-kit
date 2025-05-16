package command

import (
	"fmt"
	"strings"
	"time"

	"github.com/mattermost/mattermost/server/public/model"
)

// executeMissionStatusCommand handles the /mission status command
func (c *Handler) executeMissionStatusCommand(args *model.CommandArgs) (*model.CommandResponse, error) {
	// Parse arguments
	commandArgs := parseArgs(args.Command)

	var missionID string
	var status string

	// Check if first argument after "status" is the status itself (simplified format)
	split := strings.Fields(args.Command)
	if len(split) > 2 && !strings.HasPrefix(split[2], "--") {
		status = split[2]
		missionID = commandArgs["id"]
	} else {
		status = commandArgs["status"]
		missionID = commandArgs["id"]
	}

	// Check if this is a request from a mission channel
	if missionID == "" {
		// Try to find the mission by channel ID
		mission, err := c.mission.GetMissionByChannelID(args.ChannelId)
		if err == nil {
			missionID = mission.ID
		}
	}

	if missionID == "" {
		return &model.CommandResponse{
			ResponseType: model.CommandResponseTypeEphemeral,
			Text:         "Mission ID is required. Use `--id [id]` or run this command in a mission channel.",
		}, nil
	}

	if status == "" {
		return &model.CommandResponse{
			ResponseType: model.CommandResponseTypeEphemeral,
			Text:         "Status is required. Use `--status [status]`. Valid statuses: stalled, in-air, completed, cancelled",
		}, nil
	}

	// Validate status
	validStatuses := map[string]bool{
		"stalled":   true,
		"in-air":    true,
		"completed": true,
		"cancelled": true,
	}

	if !validStatuses[status] {
		return &model.CommandResponse{
			ResponseType: model.CommandResponseTypeEphemeral,
			Text:         "Invalid status. Valid statuses: stalled, in-air, completed, cancelled",
		}, nil
	}

	// Get the current mission to know the previous status
	mission, err := c.mission.GetMission(missionID)
	if err != nil {
		return &model.CommandResponse{
			ResponseType: model.CommandResponseTypeEphemeral,
			Text:         "Mission not found.",
		}, nil
	}

	oldStatus := mission.Status

	// Update the mission status
	if err := c.mission.UpdateMissionStatus(missionID, status); err != nil {
		c.client.Log.Error("Error updating mission status", "error", err.Error())
		return &model.CommandResponse{
			ResponseType: model.CommandResponseTypeEphemeral,
			Text:         fmt.Sprintf("Error updating mission status: %v", err),
		}, nil
	}

	// Get the updated mission
	mission, err = c.mission.GetMission(missionID)
	if err != nil {
		return &model.CommandResponse{
			ResponseType: model.CommandResponseTypeEphemeral,
			Text:         "Mission not found after update.",
		}, nil
	}

	channel, err := c.client.Channel.Get(mission.ChannelID)
	if err != nil {
		c.client.Log.Error("Error getting channel", "error", err.Error())
		return &model.CommandResponse{
			ResponseType: model.CommandResponseTypeEphemeral,
			Text:         fmt.Sprintf("Error getting mission channel: %v", err),
		}, nil
	}

	statusEmoji := c.mission.GetStatusEmoji(status)
	newDisplayName := fmt.Sprintf("%s %s: %s", statusEmoji, mission.Callsign, mission.Name)

	channel.DisplayName = newDisplayName
	if err := c.client.Channel.Update(channel); err != nil {
		c.client.Log.Error("Error updating channel display name", "error", err.Error())
	}

	// Send a message to the mission channel
	statusMsg := fmt.Sprintf("Mission status updated to: **%s**", status)
	_, err = c.bot.PostMessageFromBot(mission.ChannelID, statusMsg)
	if err != nil {
		c.client.Log.Error("Error sending message to mission channel", "error", err.Error())
	}

	// If status changed to "in-air", execute weather commands for both airports
	if status == "in-air" {
		go func() {
			// Add a slight delay
			time.Sleep(1 * time.Second)

			// First send a message explaining what we're doing
			introMsg := "✈️ **Flight Now In Air** ✈️\n\nUpdating weather conditions for flight path..."
			_, err = c.bot.PostMessageFromBot(mission.ChannelID, introMsg)

			if err != nil {
				c.client.Log.Error("Error sending intro message", "error", err.Error())
			}

			// Execute weather command for departure airport
			departureCmd := &model.CommandArgs{
				Command:   fmt.Sprintf("/weather --location %s", mission.DepartureAirport),
				ChannelId: mission.ChannelID,
				UserId:    args.UserId,
				TeamId:    args.TeamId,
			}
			if _, err := c.client.SlashCommand.Execute(departureCmd); err != nil {
				c.client.Log.Error("Error executing departure weather command", "error", err.Error())
			}

			// Add delay between commands
			time.Sleep(2 * time.Second)

			// Execute weather command for arrival airport
			arrivalCmd := &model.CommandArgs{
				Command:   fmt.Sprintf("/weather --location %s", mission.ArrivalAirport),
				ChannelId: mission.ChannelID,
				UserId:    args.UserId,
				TeamId:    args.TeamId,
			}
			if _, err := c.client.SlashCommand.Execute(arrivalCmd); err != nil {
				c.client.Log.Error("Error executing arrival weather command", "error", err.Error())
			}
		}()
	}

	// Notify subscribed channels if the status changed
	if oldStatus != status {
		go c.subscription.NotifySubscribersOfStatusChange(mission, oldStatus)
	}

	// Send success response
	return &model.CommandResponse{
		ResponseType: model.CommandResponseTypeInChannel,
		Text:         fmt.Sprintf("✅ Mission **%s** status updated to **%s**", mission.Name, status),
	}, nil
}
