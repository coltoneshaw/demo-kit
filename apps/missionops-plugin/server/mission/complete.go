package mission

import (
	"fmt"
	"time"
)

// completeMission is called when the dialog is submitted
func (m *Mission) CompleteMission(missionID, objectivesCompletion, notableEvents, crewPerformance, missionDurationStr, userID string) error {
	// Set status to completed
	if err := m.UpdateMissionStatus(missionID, "completed"); err != nil {
		m.client.Log.Error("Error updating mission status", "error", err.Error())
		return err
	}

	// Get the updated mission
	mission, err := m.GetMission(missionID)
	if err != nil {
		m.client.Log.Error("Mission not found after update", "error", err.Error())
		return err
	}

	// Update the channel name to use the completed emoji (green check)
	completedEmoji := m.GetStatusEmoji("completed")
	// Get the channel to update
	channel, err := m.client.Channel.Get(mission.ChannelID)
	if err != nil {
		m.client.Log.Error("Error getting channel", "error", err.Error())
		// Don't return error, continue with completion
		return err
	}

	// Update channel display name with completed emoji
	updatedName := fmt.Sprintf("%s %s: %s", completedEmoji, mission.Callsign, mission.Name)
	if channel.DisplayName != updatedName {
		channel.DisplayName = updatedName
		err = m.client.Channel.Update(channel)
		if err != nil {
			m.client.Log.Error("Error updating channel name", "error", err.Error())
		}
	}

	// Format the mission report message
	reportMsg := fmt.Sprintf("# Post-Mission Report: %s\n\n", mission.Name)
	reportMsg += fmt.Sprintf("**Mission:** %s (Callsign: **%s**)\n", mission.Name, mission.Callsign)
	reportMsg += fmt.Sprintf("**Route:** %s → %s\n", mission.DepartureAirport, mission.ArrivalAirport)
	reportMsg += fmt.Sprintf("**Duration:** %s hours\n", missionDurationStr)
	// Format objectives completion
	var objectivesText string

	switch objectivesCompletion {
	case "all_completed":
		objectivesText = "✅ All objectives completed"
	case "partial":
		objectivesText = "⚠️ Partial objectives completed"
	case "none":
		objectivesText = "❌ Mission objectives not met"
	default:
		objectivesText = "Unknown"
	}

	reportMsg += fmt.Sprintf("**Objectives:** %s\n", objectivesText)

	// Format crew performance
	var performanceText string
	switch crewPerformance {
	case "excellent":
		performanceText = "⭐⭐⭐⭐⭐ Excellent"
	case "good":
		performanceText = "⭐⭐⭐⭐ Good"
	case "satisfactory":
		performanceText = "⭐⭐⭐ Satisfactory"
	case "needs_improvement":
		performanceText = "⭐⭐ Needs Improvement"
	default:
		performanceText = "Unknown"
	}
	reportMsg += fmt.Sprintf("**Crew Performance:** %s\n", performanceText)

	// Add notable events if provided
	if notableEvents != "" {
		reportMsg += fmt.Sprintf("\n## Notable Events\n%s\n", notableEvents)
	}

	submittingUser, err := m.client.User.Get(userID)
	if err != nil {
		m.client.Log.Error("Error getting user", "error", err.Error())
	}
	reportMsg += fmt.Sprintf("\n*Report submitted by @%s on %s*", submittingUser.Username, time.Now().Format(time.RFC1123))

	// Post to mission channel
	_, err = m.bot.PostMessageFromBot(mission.ChannelID, reportMsg)
	if err != nil {
		m.client.Log.Error("Error sending report to mission channel", "error", err.Error())
		return err
	}

	// Post a success message to the mission channel
	successMsg := fmt.Sprintf("✅ Mission **%s** has been marked as completed!", mission.Name)
	_, err = m.bot.PostMessageFromBot(mission.ChannelID, successMsg)
	if err != nil {
		m.client.Log.Error("Error sending success message", "error", err.Error())
		// Don't return error, continue with completion
	}

	return nil
}
