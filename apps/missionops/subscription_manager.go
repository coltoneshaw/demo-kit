package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"

	"github.com/google/uuid"
)

// SubscriptionManager manages mission subscriptions
type SubscriptionManager struct {
	Subscriptions map[string]*MissionSubscription `json:"subscriptions"` // Map of subscription ID to subscription
	Mutex         sync.RWMutex                    `json:"-"`             // Mutex to protect the map
	FilePath      string                          `json:"-"`             // Path to the subscription file
}

// NewSubscriptionManager creates a new subscription manager
func NewSubscriptionManager(filePath string) *SubscriptionManager {
	sm := &SubscriptionManager{
		Subscriptions: make(map[string]*MissionSubscription),
		FilePath:      filePath,
	}

	// Load existing subscriptions from file
	if err := sm.LoadFromFile(); err != nil {
		log.Printf("Failed to load subscriptions from file: %v", err)
	}

	return sm
}

// SaveToFile saves all subscriptions to a JSON file
func (sm *SubscriptionManager) SaveToFile() error {
	sm.Mutex.RLock()
	defer sm.Mutex.RUnlock()

	// Create directory if it doesn't exist
	dir := filepath.Dir(sm.FilePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %v", err)
	}

	// Marshal to JSON
	data, err := json.MarshalIndent(sm, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal subscriptions: %v", err)
	}

	// Write to file
	if err := os.WriteFile(sm.FilePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write subscriptions file: %v", err)
	}

	log.Printf("Saved %d subscriptions to %s", len(sm.Subscriptions), sm.FilePath)
	return nil
}

// LoadFromFile loads subscriptions from a JSON file
func (sm *SubscriptionManager) LoadFromFile() error {
	sm.Mutex.Lock()
	defer sm.Mutex.Unlock()

	// Check if file exists
	if _, err := os.Stat(sm.FilePath); os.IsNotExist(err) {
		log.Printf("Subscriptions file does not exist at %s", sm.FilePath)
		return nil // Not an error, just no file yet
	}

	// Read file
	data, err := os.ReadFile(sm.FilePath)
	if err != nil {
		return fmt.Errorf("failed to read subscriptions file: %v", err)
	}

	// If file is empty or too small, no valid subscriptions to load
	if len(data) < 5 { // Minimum valid JSON would be {}
		log.Printf("Empty or invalid subscriptions file at %s", sm.FilePath)
		return nil
	}

	// Unmarshal JSON
	var loadedManager SubscriptionManager
	if err := json.Unmarshal(data, &loadedManager); err != nil {
		log.Printf("Failed to unmarshal subscriptions file: %v", err)
		// Create backup of corrupted file
		backupPath := sm.FilePath + ".corrupted"
		os.WriteFile(backupPath, data, 0644)
		log.Printf("Backed up corrupted file to %s", backupPath)
		return err
	}

	// Copy subscriptions to current manager
	sm.Subscriptions = loadedManager.Subscriptions

	// Initialize StopChan for each subscription
	for _, sub := range sm.Subscriptions {
		sub.StopChan = make(chan struct{})
	}

	log.Printf("Loaded %d subscriptions from %s", len(sm.Subscriptions), sm.FilePath)
	return nil
}

// AddSubscription adds a subscription
func (sm *SubscriptionManager) AddSubscription(sub *MissionSubscription) {
	sm.Mutex.Lock()
	defer sm.Mutex.Unlock()

	// Generate ID if not provided
	if sub.ID == "" {
		sub.ID = uuid.New().String()
	}

	sm.Subscriptions[sub.ID] = sub

	// Save to file after adding
	go sm.SaveToFile()
}

// RemoveSubscription removes a subscription
func (sm *SubscriptionManager) RemoveSubscription(id string) bool {
	sm.Mutex.Lock()
	defer sm.Mutex.Unlock()

	sub, exists := sm.Subscriptions[id]
	if exists {
		// Signal the subscription to stop
		close(sub.StopChan)
		// Remove from map
		delete(sm.Subscriptions, id)
		// Save to file after removing
		go sm.SaveToFile()
		return true
	}
	return false
}

// GetSubscription gets a subscription by ID
func (sm *SubscriptionManager) GetSubscription(id string) (*MissionSubscription, bool) {
	sm.Mutex.RLock()
	defer sm.Mutex.RUnlock()
	sub, exists := sm.Subscriptions[id]
	return sub, exists
}

// GetSubscriptionsForChannel gets all subscriptions for a channel
func (sm *SubscriptionManager) GetSubscriptionsForChannel(channelID string) []*MissionSubscription {
	sm.Mutex.RLock()
	defer sm.Mutex.RUnlock()

	var subs []*MissionSubscription
	for _, sub := range sm.Subscriptions {
		if sub.ChannelID == channelID {
			subs = append(subs, sub)
		}
	}
	return subs
}

// GetSubscriptionsForStatus gets all subscriptions that are interested in a specific status
func (sm *SubscriptionManager) GetSubscriptionsForStatus(status string) []*MissionSubscription {
	sm.Mutex.RLock()
	defer sm.Mutex.RUnlock()

	var subs []*MissionSubscription
	for _, sub := range sm.Subscriptions {
		// If the subscription has no specified status types, it subscribes to all
		if len(sub.StatusTypes) == 0 {
			subs = append(subs, sub)
			continue
		}

		// Check if this status is in the subscription's list
		for _, statusType := range sub.StatusTypes {
			if statusType == status {
				subs = append(subs, sub)
				break
			}
		}
	}
	return subs
}
