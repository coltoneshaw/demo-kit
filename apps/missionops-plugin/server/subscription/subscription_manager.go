package subscription

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/coltoneshaw/demokit/missionops-plugin/server/bot"
	"github.com/coltoneshaw/demokit/missionops-plugin/server/mission"
	"github.com/mattermost/mattermost/server/public/pluginapi"
	"github.com/pkg/errors"
)

const SubscriptionPrefix = "subscription_"
const SubscriptionsListKey = "subscriptions_list"

// MissionSubscription represents a subscription to mission status updates
type MissionSubscription struct {
	ID              string    `json:"id"`
	ChannelID       string    `json:"channelId"`
	UserID          string    `json:"userId"`
	StatusTypes     []string  `json:"statusTypes"`     // Empty means all status types
	UpdateFrequency int64     `json:"updateFrequency"` // In seconds
	LastUpdated     time.Time `json:"lastUpdated"`
}

// SubscriptionInterface defines methods for managing mission subscriptions
type SubscriptionInterface interface {
	AddSubscription(sub *MissionSubscription) error
	GetSubscription(id string) (*MissionSubscription, error)
	RemoveSubscription(id string) error
	GetSubscriptionsForChannel(channelID string) ([]*MissionSubscription, error)
	GetSubscriptionsForStatus(status string) ([]*MissionSubscription, error)
	StartSubscriptionJob(sub *MissionSubscription) error
	StopSubscriptionJob(id string) error
	NotifySubscribersOfStatusChange(mission *mission.Mission, oldStatus string)
}

// SubscriptionManager manages mission subscriptions using the plugin KV store
type SubscriptionManager struct {
	client    *pluginapi.Client
	mission   mission.MissionInterface
	bot       bot.BotInterface
	mutex     sync.RWMutex
	jobsMutex sync.RWMutex
	jobs      map[string]chan struct{} // Map of subscription ID to stop channel
}

// NewSubscriptionManager creates a new subscription manager
func NewSubscriptionManager(client *pluginapi.Client, bot bot.BotInterface, mission mission.MissionInterface) SubscriptionInterface {
	return &SubscriptionManager{
		client:  client,
		bot:     bot,
		mission: mission,
		jobs:    make(map[string]chan struct{}),
	}
}

// AddSubscription adds a subscription to the KV store
func (s *SubscriptionManager) AddSubscription(sub *MissionSubscription) error {
	s.client.Log.Debug("Adding subscription", "id", sub.ID, "channelId", sub.ChannelID)

	// Generate ID if not provided
	if sub.ID == "" {
		sub.ID = fmt.Sprintf("mission-sub-%s-%d", sub.ChannelID, time.Now().Unix())
	}

	// Store in KV store
	subJSON, err := json.Marshal(sub)
	if err != nil {
		return errors.Wrap(err, "failed to marshal subscription")
	}

	// Use key with prefix for organization
	key := SubscriptionPrefix + sub.ID
	isSet, err := s.client.KV.Set(key, subJSON)
	if !isSet {
		return errors.Wrap(err, "failed to store subscription in KV store")
	}

	// Add to list of subscriptions
	return s.addSubscriptionToList(sub.ID)
}

// GetSubscription retrieves a subscription from the KV store
func (s *SubscriptionManager) GetSubscription(id string) (*MissionSubscription, error) {
	s.client.Log.Debug("Getting subscription", "id", id)

	key := SubscriptionPrefix + id
	var data []byte
	err := s.client.KV.Get(key, &data)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get subscription from KV store")
	}

	if data == nil {
		return nil, fmt.Errorf("subscription not found: %s", id)
	}

	var sub MissionSubscription
	if err := json.Unmarshal(data, &sub); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal subscription")
	}

	return &sub, nil
}

// RemoveSubscription removes a subscription from the KV store
func (s *SubscriptionManager) RemoveSubscription(id string) error {
	s.client.Log.Debug("Removing subscription", "id", id)

	// Stop the subscription job if running
	if err := s.StopSubscriptionJob(id); err != nil {
		s.client.Log.Error("Failed to stop subscription job", "error", err.Error())
		// Continue with removal anyway
	}

	// Remove from KV store
	key := SubscriptionPrefix + id
	if err := s.client.KV.Delete(key); err != nil {
		return errors.Wrap(err, "failed to remove subscription from KV store")
	}

	// Remove from list of subscriptions
	return s.removeSubscriptionFromList(id)
}

// GetSubscriptionsForChannel gets all subscriptions for a channel
func (s *SubscriptionManager) GetSubscriptionsForChannel(channelID string) ([]*MissionSubscription, error) {
	s.client.Log.Debug("Getting subscriptions for channel", "channelId", channelID)

	// Get all subscription IDs
	subIDs, err := s.getSubscriptionsList()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get subscriptions list")
	}

	var subs []*MissionSubscription
	for _, id := range subIDs {
		sub, err := s.GetSubscription(id)
		if err != nil {
			s.client.Log.Error("Failed to get subscription", "id", id, "error", err.Error())
			continue
		}

		if sub.ChannelID == channelID {
			subs = append(subs, sub)
		}
	}

	return subs, nil
}

// GetSubscriptionsForStatus gets all subscriptions interested in a specific status
func (s *SubscriptionManager) GetSubscriptionsForStatus(status string) ([]*MissionSubscription, error) {
	s.client.Log.Debug("Getting subscriptions for status", "status", status)

	// Get all subscription IDs
	subIDs, err := s.getSubscriptionsList()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get subscriptions list")
	}

	var subs []*MissionSubscription
	for _, id := range subIDs {
		sub, err := s.GetSubscription(id)
		if err != nil {
			s.client.Log.Error("Failed to get subscription", "id", id, "error", err.Error())
			continue
		}

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

	return subs, nil
}

// RestartSubscriptions restarts all subscription jobs
func (s *SubscriptionManager) RestartSubscriptions() error {
	s.client.Log.Debug("Restarting subscription jobs")

	// Get all subscription IDs
	subIDs, err := s.getSubscriptionsList()
	if err != nil {
		return errors.Wrap(err, "failed to get subscriptions list")
	}

	s.client.Log.Debug("Found subscriptions to restart", "count", len(subIDs))

	// Restart each subscription
	for _, id := range subIDs {
		sub, err := s.GetSubscription(id)
		if err != nil {
			s.client.Log.Error("Failed to get subscription for restart", "id", id, "error", err.Error())
			continue
		}

		if err := s.StartSubscriptionJob(sub); err != nil {
			s.client.Log.Error("Failed to restart subscription job", "id", id, "error", err.Error())
			continue
		}
	}

	return nil
}

// StartSubscriptionJob starts a job for a subscription
func (s *SubscriptionManager) StartSubscriptionJob(sub *MissionSubscription) error {
	s.jobsMutex.Lock()
	defer s.jobsMutex.Unlock()

	// Check if job is already running
	if _, exists := s.jobs[sub.ID]; exists {
		return fmt.Errorf("subscription job already running: %s", sub.ID)
	}

	// Create stop channel
	stopChan := make(chan struct{})
	s.jobs[sub.ID] = stopChan

	// Start the job
	go s.runSubscriptionJob(sub, stopChan)

	return nil
}

// StopSubscriptionJob stops a job for a subscription
func (s *SubscriptionManager) StopSubscriptionJob(id string) error {
	s.jobsMutex.Lock()
	defer s.jobsMutex.Unlock()

	// Check if job exists
	stopChan, exists := s.jobs[id]
	if !exists {
		return nil // Job not running
	}

	// Signal job to stop
	close(stopChan)
	delete(s.jobs, id)

	return nil
}

// getSubscriptionsList retrieves the list of all subscription IDs
func (s *SubscriptionManager) getSubscriptionsList() ([]string, error) {
	var data []byte
	err := s.client.KV.Get(SubscriptionsListKey, &data)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get subscriptions list from KV store")
	}

	if data == nil {
		return []string{}, nil // No subscriptions yet
	}

	var ids []string
	if err := json.Unmarshal(data, &ids); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal subscriptions list")
	}

	return ids, nil
}

// addSubscriptionToList adds a subscription ID to the list
func (s *SubscriptionManager) addSubscriptionToList(id string) error {
	ids, err := s.getSubscriptionsList()
	if err != nil {
		return errors.Wrap(err, "failed to get subscriptions list")
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
		return errors.Wrap(err, "failed to marshal subscriptions list")
	}

	isSet, err := s.client.KV.Set(SubscriptionsListKey, data)
	if !isSet {
		return errors.Wrap(err, "failed to save subscriptions list to KV store")
	}

	return nil
}

// removeSubscriptionFromList removes a subscription ID from the list
func (s *SubscriptionManager) removeSubscriptionFromList(id string) error {
	ids, err := s.getSubscriptionsList()
	if err != nil {
		return errors.Wrap(err, "failed to get subscriptions list")
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
		return errors.Wrap(err, "failed to marshal subscriptions list")
	}

	isSet, err := s.client.KV.Set(SubscriptionsListKey, data)
	if !isSet {
		return errors.Wrap(err, "failed to save subscriptions list to KV store")
	}

	return nil
}

// runSubscriptionJob runs a subscription job in the background
func (s *SubscriptionManager) runSubscriptionJob(sub *MissionSubscription, stopChan chan struct{}) {
	s.client.Log.Debug("Starting subscription job", "id", sub.ID, "channelId", sub.ChannelID)

	// Set up ticker for periodic updates
	ticker := time.NewTicker(time.Duration(sub.UpdateFrequency) * time.Second)
	defer ticker.Stop()

	// Function to fetch and send mission updates
	fetchAndSendMissionUpdates := func() {
		// Get current time
		now := time.Now()

		s.client.Log.Debug("Fetching mission updates for subscription",
			"id", sub.ID, "channelId", sub.ChannelID, "statusTypes", sub.StatusTypes)

		// Get missions based on subscription status types
		var missions []*mission.Mission
		var err error

		if len(sub.StatusTypes) == 0 {
			// If no status types specified, get all missions
			missions, err = s.mission.GetAllMissions()
			if err != nil {
				s.client.Log.Error("Failed to get all missions for subscription", "error", err.Error())
				return
			}
		} else {
			// Otherwise, get missions for each status type
			for _, statusType := range sub.StatusTypes {
				statusMissions, err := s.mission.GetMissionsByStatus(statusType)
				if err != nil {
					s.client.Log.Error("Failed to get missions by status", "status", statusType, "error", err.Error())
					continue
				}
				missions = append(missions, statusMissions...)
			}
		}

		// If no missions found, log and return
		if len(missions) == 0 {
			s.client.Log.Debug("No missions found for subscription", "id", sub.ID)
			return
		}

		// Format the mission updates
		message := fmt.Sprintf("# Mission Status Update (%s)\n\n", now.Format(time.RFC1123))
		message += "| Name | Callsign | Departure | Arrival | Status | Channel |\n"
		message += "|------|----------|-----------|---------|--------|--------|\n"

		for _, mission := range missions {
			statusEmoji := s.mission.GetStatusEmoji(mission.Status)
			message += fmt.Sprintf("| %s | %s | %s | %s | %s %s | ~%s |\n",
				mission.Name, mission.Callsign, mission.DepartureAirport, mission.ArrivalAirport,
				statusEmoji, mission.Status, mission.ChannelName)
		}

		// Add a note about subscription
		statusTypesText := "all statuses"
		if len(sub.StatusTypes) > 0 {
			statusTypesText = ""
			for i, status := range sub.StatusTypes {
				if i > 0 {
					statusTypesText += ", "
				}
				statusTypesText += status
			}
		}

		message += fmt.Sprintf("\n\n*This is an automated update for mission statuses: %s. Updates every %d seconds.*",
			statusTypesText, sub.UpdateFrequency)

		// Send to Mattermost with the subscription's channel ID
		_, err = s.bot.PostMessageFromBot(sub.ChannelID, message)
		if err != nil {
			s.client.Log.Error("Failed to send mission update", "error", err.Error())
			return
		}

		// Update last updated time
		sub.LastUpdated = now
		err = s.AddSubscription(sub) // Save updated subscription
		if err != nil {
			s.client.Log.Error("Failed to update subscription last updated time", "error", err.Error())
		}
	}

	// Fetch and send initial data
	fetchAndSendMissionUpdates()

	// Wait for updates or cancellation
	for {
		select {
		case <-ticker.C:
			fetchAndSendMissionUpdates()
		case <-stopChan:
			s.client.Log.Debug("Stopping subscription job", "id", sub.ID)
			return
		}
	}
}

// notifySubscribersOfStatusChange notifies all relevant subscribers when a mission status changes
func (c *SubscriptionManager) NotifySubscribersOfStatusChange(mission *mission.Mission, oldStatus string) {
	// Find all subscriptions that care about this status change
	subs, err := c.GetSubscriptionsForStatus(mission.Status)
	if err != nil {
		c.client.Log.Error("Error getting subscriptions for status", "status", mission.Status, "error", err.Error())
		return
	}

	if len(subs) == 0 {
		c.client.Log.Info("No subscriptions found for status", "status", mission.Status)
		return
	}

	// Create the notification message
	statusChangeMsg := fmt.Sprintf("# Mission Status Change Alert\n\n"+
		"**Mission:** %s (Callsign: **%s**)\n"+
		"**Status Changed:** %s → %s\n"+
		"**Departure:** %s\n"+
		"**Arrival:** %s\n\n"+
		"[View Mission Channel](~%s)",
		mission.Name, mission.Callsign, oldStatus, mission.Status,
		mission.DepartureAirport, mission.ArrivalAirport, mission.ChannelName)

	// Send notification to each subscribed channel
	for _, sub := range subs {
		// Skip the mission's own channel (it already gets notifications)
		if sub.ChannelID == mission.ChannelID {
			continue
		}

		c.client.Log.Debug("Sending status change notification", "subscriptionId", sub.ID, "channelId", sub.ChannelID)
		_, err := c.bot.PostMessageFromBot(sub.ChannelID, statusChangeMsg)
		if err != nil {
			c.client.Log.Error("Error sending status change notification", "subscriptionId", sub.ID, "error", err.Error())
		}
	}
}
