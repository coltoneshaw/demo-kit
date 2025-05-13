package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
)

// SubscriptionManager manages all active subscriptions
type SubscriptionManager struct {
	Subscriptions map[string]*Subscription `json:"subscriptions"` // Map of subscription ID to subscription
	Mutex         sync.RWMutex             `json:"-"`             // Mutex to protect the map (not serialized)
	FilePath      string                   `json:"-"`             // Path to the subscription file (not serialized)
	HourlyLimit   int                      `json:"hourly_limit"`  // Maximum API calls per hour
	DailyLimit    int                      `json:"daily_limit"`   // Maximum API calls per day
}

// NewSubscriptionManager creates a new subscription manager
func NewSubscriptionManager(filePath string) *SubscriptionManager {
	sm := &SubscriptionManager{
		Subscriptions: make(map[string]*Subscription),
		FilePath:      filePath,
		HourlyLimit:   25,  // 25 requests per hour limit
		DailyLimit:    500, // 500 requests per day limit
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

	// Only log during initial load or when explicitly requested
	if len(sm.Subscriptions) > 0 && os.Getenv("DEBUG") == "true" {
		log.Printf("Saved %d subscriptions to %s", len(sm.Subscriptions), sm.FilePath)
	}
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

	// Unmarshal JSON
	var loadedManager SubscriptionManager
	if err := json.Unmarshal(data, &loadedManager); err != nil {
		return fmt.Errorf("failed to unmarshal subscriptions: %v", err)
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

// AddSubscription adds a new subscription
func (sm *SubscriptionManager) AddSubscription(sub *Subscription) {
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
func (sm *SubscriptionManager) GetSubscription(id string) (*Subscription, bool) {
	sm.Mutex.RLock()
	defer sm.Mutex.RUnlock()
	sub, exists := sm.Subscriptions[id]
	return sub, exists
}

// GetSubscriptionsForChannel gets all subscriptions for a channel
func (sm *SubscriptionManager) GetSubscriptionsForChannel(channelID string) []*Subscription {
	sm.Mutex.RLock()
	defer sm.Mutex.RUnlock()

	var subs []*Subscription
	for _, sub := range sm.Subscriptions {
		if sub.ChannelID == channelID {
			subs = append(subs, sub)
		}
	}
	return subs
}

// GetSubscriptionsForUser gets all subscriptions created by a user
func (sm *SubscriptionManager) GetSubscriptionsForUser(userID string) []*Subscription {
	sm.Mutex.RLock()
	defer sm.Mutex.RUnlock()

	var subs []*Subscription
	for _, sub := range sm.Subscriptions {
		if sub.UserID == userID {
			subs = append(subs, sub)
		}
	}
	return subs
}

// CalculateAPIUsage calculates the current API usage per hour and per day
func (sm *SubscriptionManager) CalculateAPIUsage() (hourlyUsage, dailyUsage int) {
	sm.Mutex.RLock()
	defer sm.Mutex.RUnlock()

	for _, sub := range sm.Subscriptions {
		// Calculate how many requests this subscription makes per hour
		// (3600000 milliseconds in an hour)
		requestsPerHour := 3600000 / sub.UpdateFrequency
		if requestsPerHour < 1 {
			requestsPerHour = 1 // Minimum 1 request per hour
		}
		hourlyUsage += int(requestsPerHour)

		// Calculate how many requests this subscription makes per day
		// (86400000 milliseconds in a day)
		requestsPerDay := 86400000 / sub.UpdateFrequency
		if requestsPerDay < 1 {
			requestsPerDay = 1 // Minimum 1 request per day
		}
		dailyUsage += int(requestsPerDay)
	}

	return hourlyUsage, dailyUsage
}

// CheckSubscriptionLimits checks if adding a new subscription would exceed API limits
func (sm *SubscriptionManager) CheckSubscriptionLimits(updateFrequency int64) (bool, string) {
	// Calculate current usage
	currentHourlyUsage, currentDailyUsage := sm.CalculateAPIUsage()

	// Calculate new subscription's usage
	newHourlyUsage := 3600000 / updateFrequency
	if newHourlyUsage < 1 {
		newHourlyUsage = 1
	}

	newDailyUsage := 86400000 / updateFrequency
	if newDailyUsage < 1 {
		newDailyUsage = 1
	}

	// Check if adding this subscription would exceed limits
	if currentHourlyUsage+int(newHourlyUsage) > sm.HourlyLimit {
		return false, fmt.Sprintf(
			"Adding this subscription would exceed the hourly API limit of %d requests. Current usage: %d, New subscription would add: %d requests per hour.",
			sm.HourlyLimit, currentHourlyUsage, newHourlyUsage)
	}

	if currentDailyUsage+int(newDailyUsage) > sm.DailyLimit {
		return false, fmt.Sprintf(
			"Adding this subscription would exceed the daily API limit of %d requests. Current usage: %d, New subscription would add: %d requests per day.",
			sm.DailyLimit, currentDailyUsage, newDailyUsage)
	}

	return true, ""
}
