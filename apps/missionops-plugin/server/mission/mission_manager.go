package mission

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/pkg/errors"
)

// AddMission adds a mission to the KV store
func (m *Mission) AddMission(mission *Mission) error {
	m.client.Log.Info("Adding mission", "name", mission.Name, "callsign", mission.Callsign)

	// Store in KV store
	missionJSON, err := json.Marshal(mission)
	if err != nil {
		return errors.Wrap(err, "failed to marshal mission")
	}

	// Use key with prefix for organization
	key := MissionPrefix + mission.ID

	// Check if mission already exists
	kvSet, err := m.client.KV.Set(key, missionJSON)

	if !kvSet {
		return errors.Wrap(err, "failed to store mission in KV store")
	}

	// Add to list of missions
	return m.addMissionToList(mission.ID)
}

// GetMission retrieves a mission from the KV store
func (m *Mission) GetMission(id string) (*Mission, error) {
	m.client.Log.Info("Getting mission", "id", id)

	key := MissionPrefix + id
	data := []byte{}
	err := m.client.KV.Get(key, data)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get mission from KV store")
	}

	if data == nil {
		return nil, fmt.Errorf("mission not found: %s", id)
	}

	var mission Mission
	if err := json.Unmarshal(data, &mission); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal mission")
	}

	return &mission, nil
}

// GetMissionByChannelID retrieves a mission by its channel ID
func (m *Mission) GetMissionByChannelID(channelID string) (*Mission, error) {
	m.client.Log.Info("Getting mission by channel ID", "channelId", channelID)

	// Get all mission IDs
	missionIDs, err := m.getMissionsList()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get missions list")
	}

	for _, id := range missionIDs {
		mission, err := m.GetMission(id)
		if err != nil {
			m.client.Log.Error("Failed to get mission", "id", id, "error", err.Error())
			continue
		}

		if mission.ChannelID == channelID {
			return mission, nil
		}
	}

	return nil, fmt.Errorf("no mission found for channel: %s", channelID)
}

// UpdateMissionStatus updates a mission's status
func (m *Mission) UpdateMissionStatus(id string, status string) error {
	m.client.Log.Debug("Updating mission status", "id", id, "status", status)

	mission, err := m.GetMission(id)
	if err != nil {
		return errors.Wrap(err, "failed to get mission")
	}

	// Update status
	mission.Status = status

	// If completing or cancelling, set completed time
	if status == "completed" || status == "cancelled" {
		mission.CompletedAt = time.Now()
	}

	// Save the updated mission
	return m.AddMission(mission)
}

// GetAllMissions gets all missions
func (m *Mission) GetAllMissions() ([]*Mission, error) {
	m.client.Log.Debug("Getting all missions")

	// Get all mission IDs
	missionIDs, err := m.getMissionsList()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get missions list")
	}

	var missions []*Mission
	for _, id := range missionIDs {
		mission, err := m.GetMission(id)
		if err != nil {
			m.client.Log.Error("Failed to get mission", "id", id, "error", err.Error())
			continue
		}

		missions = append(missions, mission)
	}

	return missions, nil
}

// GetMissionsByStatus gets all missions with a specific status
func (m *Mission) GetMissionsByStatus(status string) ([]*Mission, error) {
	m.client.Log.Debug("Getting missions by status", "status", status)

	// Get all missions
	allMissions, err := m.GetAllMissions()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get all missions")
	}

	// Filter by status
	var filteredMissions []*Mission
	for _, mission := range allMissions {
		if mission.Status == status {
			filteredMissions = append(filteredMissions, mission)
		}
	}

	return filteredMissions, nil
}

// getMissionsList retrieves the list of all mission IDs
func (m *Mission) getMissionsList() ([]string, error) {
	missionLists := []byte{}
	appErr := m.client.KV.Get(MissionsListKey, missionLists)
	if appErr != nil {
		return nil, errors.Wrap(appErr, "failed to get missions list from KV store")
	}

	if missionLists == nil {
		return []string{}, nil // No missions yet
	}

	var ids []string
	if err := json.Unmarshal(missionLists, &ids); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal missions list")
	}

	return ids, nil
}

// addMissionToList adds a mission ID to the list
func (m *Mission) addMissionToList(id string) error {
	ids, err := m.getMissionsList()
	if err != nil {
		return errors.Wrap(err, "failed to get missions list")
	}

	// Check if ID already exists
	for _, existingID := range ids {
		if existingID == id {
			return nil // Already in the list
		}
	}

	// Add to list
	ids = append(ids, id)

	// Save updated list
	data, err := json.Marshal(ids)
	if err != nil {
		return errors.Wrap(err, "failed to marshal missions list")
	}

	kvSet, err := m.client.KV.Set(MissionsListKey, data)
	if !kvSet {
		return errors.Wrap(err, "failed to save missions list to KV store")
	}

	return nil
}

// removeMissionFromList removes a mission ID from the list
func (m *Mission) removeMissionFromList(id string) error {
	ids, err := m.getMissionsList()
	if err != nil {
		return errors.Wrap(err, "failed to get missions list")
	}

	// Create a new list without the ID to remove
	var newIDs []string
	for _, existingID := range ids {
		if existingID != id {
			newIDs = append(newIDs, existingID)
		}
	}

	// Save updated list
	data, err := json.Marshal(newIDs)
	if err != nil {
		return errors.Wrap(err, "failed to marshal missions list")
	}

	kvSet, err := m.client.KV.Set(MissionsListKey, data)
	if !kvSet {
		return errors.Wrap(err, "failed to save missions list to KV store")
	}

	return nil
}
