package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// FlightSubscription represents a subscription to flight departures
type FlightSubscription struct {
	ID              string    `json:"id"`               // Unique identifier for the subscription
	Airport         string    `json:"airport"`          // Airport to get departures for (ICAO code)
	ChannelID       string    `json:"channel_id"`       // Channel to post updates to
	UserID          string    `json:"user_id"`          // User who created the subscription
	UpdateFrequency int64     `json:"update_frequency"` // How often to update (in seconds)
	LastUpdated     time.Time `json:"last_updated"`     // When the subscription was last updated
	StopChan        chan struct{} `json:"-"`            // Channel to signal stopping the subscription
}

// SubscriptionManager manages flight subscriptions
type SubscriptionManager struct {
	Subscriptions map[string]*FlightSubscription `json:"subscriptions"` // Map of subscription ID to subscription
	Mutex         sync.RWMutex                   `json:"-"`             // Mutex to protect the map
	FilePath      string                         `json:"-"`             // Path to the subscription file
}

// NewSubscriptionManager creates a new subscription manager
func NewSubscriptionManager(filePath string) *SubscriptionManager {
	sm := &SubscriptionManager{
		Subscriptions: make(map[string]*FlightSubscription),
		FilePath:      filePath,
	}

	// Load existing subscriptions from file
	if err := sm.LoadFromFile(); err != nil {
		log.Printf("Error loading subscriptions: %v", err)
	}

	return sm
}

// SaveToFile saves subscriptions to a file
func (sm *SubscriptionManager) SaveToFile() error {
	sm.Mutex.RLock()
	defer sm.Mutex.RUnlock()

	// Create directory if it doesn't exist
	dir := filepath.Dir(sm.FilePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %v", err)
	}

	// Open file for writing
	file, err := os.Create(sm.FilePath)
	if err != nil {
		return fmt.Errorf("failed to create file: %v", err)
	}
	defer file.Close()

	// Encode subscriptions to JSON
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(sm); err != nil {
		return fmt.Errorf("failed to encode subscriptions: %v", err)
	}

	return nil
}

// LoadFromFile loads subscriptions from a file
func (sm *SubscriptionManager) LoadFromFile() error {
	sm.Mutex.Lock()
	defer sm.Mutex.Unlock()

	// Check if file exists
	if _, err := os.Stat(sm.FilePath); os.IsNotExist(err) {
		log.Printf("Subscriptions file does not exist at %s", sm.FilePath)
		return nil // Not an error, just no file yet
	}

	// Open file for reading
	file, err := os.Open(sm.FilePath)
	if err != nil {
		return fmt.Errorf("failed to open file: %v", err)
	}
	defer file.Close()

	// Decode subscriptions from JSON
	var loadedManager SubscriptionManager
	if err := json.NewDecoder(file).Decode(&loadedManager); err != nil {
		return fmt.Errorf("failed to decode subscriptions: %v", err)
	}

	// Copy subscriptions to this manager
	sm.Subscriptions = loadedManager.Subscriptions

	return nil
}

// AddSubscription adds a subscription
func (sm *SubscriptionManager) AddSubscription(sub *FlightSubscription) {
	sm.Mutex.Lock()
	defer sm.Mutex.Unlock()
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
func (sm *SubscriptionManager) GetSubscription(id string) (*FlightSubscription, bool) {
	sm.Mutex.RLock()
	defer sm.Mutex.RUnlock()
	sub, exists := sm.Subscriptions[id]
	return sub, exists
}

// GetSubscriptionsForChannel gets all subscriptions for a channel
func (sm *SubscriptionManager) GetSubscriptionsForChannel(channelID string) []*FlightSubscription {
	sm.Mutex.RLock()
	defer sm.Mutex.RUnlock()

	var subs []*FlightSubscription
	for _, sub := range sm.Subscriptions {
		if sub.ChannelID == channelID {
			subs = append(subs, sub)
		}
	}
	return subs
}

// GetSubscriptionsForUser gets all subscriptions for a user
func (sm *SubscriptionManager) GetSubscriptionsForUser(userID string) []*FlightSubscription {
	sm.Mutex.RLock()
	defer sm.Mutex.RUnlock()

	var subs []*FlightSubscription
	for _, sub := range sm.Subscriptions {
		if sub.UserID == userID {
			subs = append(subs, sub)
		}
	}
	return subs
}
