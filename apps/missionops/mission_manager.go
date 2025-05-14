package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
)

// MissionManager manages missions
type MissionManager struct {
	Missions map[string]*Mission `json:"missions"` // Map of mission ID to mission
	Mutex    sync.RWMutex        `json:"-"`        // Mutex to protect the map
	FilePath string              `json:"-"`        // Path to the missions file
}

// NewMissionManager creates a new mission manager
func NewMissionManager(filePath string) *MissionManager {
	mm := &MissionManager{
		Missions: make(map[string]*Mission),
		FilePath: filePath,
	}

	// Load existing missions from file
	if err := mm.LoadFromFile(); err != nil {
		log.Printf("Failed to load missions from file: %v", err)
	}

	return mm
}

// SaveToFile saves all missions to a JSON file
func (mm *MissionManager) SaveToFile() error {
	mm.Mutex.RLock()
	defer mm.Mutex.RUnlock()

	// Create directory if it doesn't exist
	dir := filepath.Dir(mm.FilePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %v", err)
	}

	// Marshal to JSON
	data, err := json.MarshalIndent(mm, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal missions: %v", err)
	}

	// Write to file
	if err := os.WriteFile(mm.FilePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write missions file: %v", err)
	}

	log.Printf("Saved %d missions to %s", len(mm.Missions), mm.FilePath)
	return nil
}

// LoadFromFile loads missions from a JSON file
func (mm *MissionManager) LoadFromFile() error {
	mm.Mutex.Lock()
	defer mm.Mutex.Unlock()

	// Check if file exists
	if _, err := os.Stat(mm.FilePath); os.IsNotExist(err) {
		log.Printf("Missions file does not exist at %s", mm.FilePath)
		return nil // Not an error, just no file yet
	}

	// Read file
	data, err := os.ReadFile(mm.FilePath)
	if err != nil {
		return fmt.Errorf("failed to read missions file: %v", err)
	}

	// If file is empty or too small, no valid missions to load
	if len(data) < 5 { // Minimum valid JSON would be {}
		log.Printf("Empty or invalid missions file at %s", mm.FilePath)
		return nil
	}

	// Unmarshal JSON
	var loadedManager MissionManager
	if err := json.Unmarshal(data, &loadedManager); err != nil {
		log.Printf("Failed to unmarshal missions file: %v", err)
		// Create backup of corrupted file
		backupPath := mm.FilePath + ".corrupted"
		os.WriteFile(backupPath, data, 0644)
		log.Printf("Backed up corrupted file to %s", backupPath)
		return err
	}

	// Copy missions to current manager
	mm.Missions = loadedManager.Missions

	log.Printf("Loaded %d missions from %s", len(mm.Missions), mm.FilePath)
	return nil
}

// AddMission adds a mission
func (mm *MissionManager) AddMission(mission *Mission) {
	mm.Mutex.Lock()
	defer mm.Mutex.Unlock()

	// Generate ID if not provided
	if mission.ID == "" {
		mission.ID = uuid.New().String()
	}

	// Set creation time if not provided
	if mission.CreatedAt.IsZero() {
		mission.CreatedAt = time.Now()
	}

	// Default status to 'stalled' if not provided
	if mission.Status == "" {
		mission.Status = "stalled"
	}

	mm.Missions[mission.ID] = mission

	// Create a copy of the missions map to avoid race conditions
	missionsCopy := make(map[string]*Mission)
	for id, m := range mm.Missions {
		missionsCopy[id] = m
	}

	// Save to file after adding using copied data to avoid race conditions
	go func(missions map[string]*Mission, filePath string) {
		// Create a temporary manager with the copied data
		tempManager := &MissionManager{
			Missions: missions,
			FilePath: filePath,
		}
		
		if err := tempManager.SaveToFile(); err != nil {
			log.Printf("Error saving missions to file: %v", err)
		}
	}(missionsCopy, mm.FilePath)
}

// UpdateMission updates an existing mission
func (mm *MissionManager) UpdateMission(id string, updatedMission *Mission) bool {
	mm.Mutex.Lock()
	defer mm.Mutex.Unlock()

	if _, exists := mm.Missions[id]; exists {
		mm.Missions[id] = updatedMission
		
		// Create a copy of the missions map to avoid race conditions
		missionsCopy := make(map[string]*Mission)
		for id, m := range mm.Missions {
			missionsCopy[id] = m
		}

		// Save to file after updating using copied data to avoid race conditions
		go func(missions map[string]*Mission, filePath string) {
			// Create a temporary manager with the copied data
			tempManager := &MissionManager{
				Missions: missions,
				FilePath: filePath,
			}
			
			if err := tempManager.SaveToFile(); err != nil {
				log.Printf("Error saving missions to file: %v", err)
			}
		}(missionsCopy, mm.FilePath)
		
		return true
	}
	return false
}

// GetMission gets a mission by ID
func (mm *MissionManager) GetMission(id string) (*Mission, bool) {
	mm.Mutex.RLock()
	defer mm.Mutex.RUnlock()
	mission, exists := mm.Missions[id]
	return mission, exists
}

// GetAllMissions gets all missions
func (mm *MissionManager) GetAllMissions() []*Mission {
	mm.Mutex.RLock()
	defer mm.Mutex.RUnlock()

	missions := make([]*Mission, 0, len(mm.Missions))
	for _, mission := range mm.Missions {
		missions = append(missions, mission)
	}
	return missions
}

// GetMissionsByStatus gets all missions with a specific status
func (mm *MissionManager) GetMissionsByStatus(status string) []*Mission {
	mm.Mutex.RLock()
	defer mm.Mutex.RUnlock()

	var missions []*Mission
	for _, mission := range mm.Missions {
		if mission.Status == status {
			missions = append(missions, mission)
		}
	}
	return missions
}

// GetMissionByChannelID gets a mission by its channel ID
func (mm *MissionManager) GetMissionByChannelID(channelID string) (*Mission, bool) {
	mm.Mutex.RLock()
	defer mm.Mutex.RUnlock()

	for _, mission := range mm.Missions {
		if mission.ChannelID == channelID {
			return mission, true
		}
	}
	return nil, false
}

// UpdateMissionStatus updates the status of a mission
func (mm *MissionManager) UpdateMissionStatus(id string, status string) bool {
	mm.Mutex.Lock()
	defer mm.Mutex.Unlock()

	mission, exists := mm.Missions[id]
	if exists {
		mission.Status = status
		
		// Create a copy of the missions map to avoid race conditions
		missionsCopy := make(map[string]*Mission)
		for id, m := range mm.Missions {
			missionsCopy[id] = m
		}

		// Save to file after updating using copied data to avoid race conditions
		go func(missions map[string]*Mission, filePath string) {
			// Create a temporary manager with the copied data
			tempManager := &MissionManager{
				Missions: missions,
				FilePath: filePath,
			}
			
			if err := tempManager.SaveToFile(); err != nil {
				log.Printf("Error saving missions to file: %v", err)
			}
		}(missionsCopy, mm.FilePath)
		
		return true
	}
	return false
}
