package command

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/gorilla/mux"
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
	}

	// Get the mission by ID to display details in the dialog
	mission, err := c.mission.GetMission(missionID)
	if err != nil {
		return &model.CommandResponse{
			ResponseType: model.CommandResponseTypeEphemeral,
			Text:         "Mission not found with the provided ID.",
		}, nil
	}

	if mission.Status == "completed" {
		return &model.CommandResponse{
			ResponseType: model.CommandResponseTypeEphemeral,
			Text:         "This mission has already been completed.",
		}, nil
	}

	// Create the interactive dialog
	dialog := model.OpenDialogRequest{
		TriggerId: args.TriggerId,
		URL:       fmt.Sprintf("/plugins/com.coltoneshaw.missionops/api/v1/missions/%s/complete", missionID),
		Dialog: model.Dialog{
			CallbackId:       "mission_complete_dialog",
			Title:            "Post-Mission Report",
			IntroductionText: fmt.Sprintf("Complete mission report for: **%s** (Callsign: **%s**)", mission.Name, mission.Callsign),
			SubmitLabel:      "Submit Report",
			NotifyOnCancel:   false,
			State:            missionID,
			Elements: []model.DialogElement{
				{
					DisplayName: "Mission Objectives Completion",
					Name:        "mission_objectives_completion",
					Type:        "select",
					Optional:    true,
					Options: []*model.PostActionOptions{
						{Text: "Yes - All objectives completed", Value: "all_completed"},
						{Text: "Partial - Some objectives completed", Value: "partial"},
						{Text: "No - Mission objectives not met", Value: "none"},
					},
				},
				{
					DisplayName: "Mission Duration",
					Name:        "mission_duration",
					Type:        "text",
					SubType:     "number",
					Optional:    false,
					Placeholder: "Enter flight hours",
				},
				{
					DisplayName: "Crew Performance",
					Name:        "crew_performance",
					Type:        "select",
					Optional:    true,
					Options: []*model.PostActionOptions{
						{Text: "Excellent", Value: "excellent"},
						{Text: "Good", Value: "good"},
						{Text: "Satisfactory", Value: "satisfactory"},
						{Text: "Needs Improvement", Value: "needs_improvement"},
					},
				},
				{
					DisplayName: "Notable Events",
					Name:        "notable_events",
					Type:        "textarea",
					Optional:    true,
					Placeholder: "Describe any notable events during the mission",
					MaxLength:   2000,
				},
			},
		},
	}

	// Open the dialog
	if err := c.client.Frontend.OpenInteractiveDialog(dialog); err != nil {
		c.client.Log.Error("Error opening interactive dialog", "error", err.Error())
		return &model.CommandResponse{
			ResponseType: model.CommandResponseTypeEphemeral,
			Text:         "Error opening mission completion dialog. Please try again.",
		}, nil
	}

	// Return an empty response, as the dialog will handle the interaction
	return &model.CommandResponse{
		ResponseType: model.CommandResponseTypeEphemeral,
		Text:         "",
	}, nil
}

// handleMissionComplete handles the mission completion dialog submission
func (h *Handler) HandleMissionComplete(w http.ResponseWriter, r *http.Request) {
	// Parse request body
	var request model.SubmitDialogRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		h.client.Log.Error("Error decoding dialog submission", "error", err.Error())
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Get mission ID from URL path
	vars := mux.Vars(r)
	missionID := vars["mission_id"]
	if missionID == "" {
		h.client.Log.Error("Missing mission ID in URL")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Extract submission data
	var objectivesCompletion, notableEvents, crewPerformance string
	if objectivesValue, ok := request.Submission["mission_objectives_completion"].(string); ok {
		objectivesCompletion = fmt.Sprintf("%v", objectivesValue)
	}
	if notableEventsValue, ok := request.Submission["notable_events"]; ok {
		notableEvents = fmt.Sprintf("%v", notableEventsValue)
	}
	if crewPerformanceValue, ok := request.Submission["crew_performance"].(string); ok {
		crewPerformance = fmt.Sprintf("%v", crewPerformanceValue)
	}

	// Mission duration could be a number or string depending on the form handling
	var missionDurationStr string
	if durationNum, ok := request.Submission["mission_duration"].(float64); ok {
		missionDurationStr = fmt.Sprintf("%.1f", durationNum)
	} else if durationStr, ok := request.Submission["mission_duration"].(string); ok {
		missionDurationStr = durationStr
	} else {
		missionDurationStr = "Unknown"
		log.Printf("Mission duration has unexpected type: %T", request.Submission["mission_duration"])
	}

	// Get the mission
	mission, err := h.mission.GetMission(missionID)
	if err != nil {
		log.Printf("Mission not found: %s", request.State)
		http.Error(w, "Mission not found", http.StatusBadRequest)
		return
	}

	// Log submission data for debugging
	h.client.Log.Debug("Mission completion data",
		"missionID", missionID,
		"objectives", objectivesCompletion,
		"crewPerformance", crewPerformance,
		"duration", missionDurationStr)

	err = h.mission.CompleteMission(missionID, objectivesCompletion, notableEvents, crewPerformance, missionDurationStr, request.UserId)
	if err != nil {
		h.client.Log.Error("Error completing mission", "error", err.Error())
		response := model.SubmitDialogResponse{
			Error: "Error completing mission: " + err.Error(),
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
		return
	}

	go h.subscription.NotifySubscribersOfStatusChange(mission, mission.Status)

	// Send success response
	response := model.SubmitDialogResponse{
		Error: "",
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
