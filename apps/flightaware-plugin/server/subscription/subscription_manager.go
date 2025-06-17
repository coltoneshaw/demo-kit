package subscription

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/coltoneshaw/demokit/flightaware-plugin/server/flight"
	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/pluginapi"
)

type FlightSubscription struct {
	ID              string        `json:"id"`
	Airport         string        `json:"airport"`
	ChannelID       string        `json:"channel_id"`
	UserID          string        `json:"user_id"`
	UpdateFrequency int64         `json:"update_frequency"`
	LastUpdated     time.Time     `json:"last_updated"`
}

type SubscriptionInterface interface {
	AddSubscription(sub *FlightSubscription) error
	RemoveSubscription(id string) bool
	GetSubscription(id string) (*FlightSubscription, bool)
	GetSubscriptionsForChannel(channelID string) []*FlightSubscription
	GetAllSubscriptions() []*FlightSubscription
	StopAll()
}

type SubscriptionManager struct {
	client         *pluginapi.Client
	flightService  flight.FlightInterface
	messageService MessageServiceInterface
	subscriptions  map[string]*FlightSubscription
	jobs           map[string]chan struct{} // Track running subscription jobs
	mutex          sync.RWMutex
}

type MessageServiceInterface interface {
	SendPublicMessage(channelID, message string) error
	GetBotUserID() string
}

func NewSubscriptionManager(client *pluginapi.Client, flightService flight.FlightInterface, messageService MessageServiceInterface) (SubscriptionInterface, error) {
	sm := &SubscriptionManager{
		client:         client,
		flightService:  flightService,
		messageService: messageService,
		subscriptions:  make(map[string]*FlightSubscription),
		jobs:           make(map[string]chan struct{}),
	}
	if err := sm.loadSubscriptions(); err != nil {
		return nil, fmt.Errorf("failed to initialize subscription manager: %w", err)
	}
	return sm, nil
}

func (sm *SubscriptionManager) AddSubscription(sub *FlightSubscription) error {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	sm.subscriptions[sub.ID] = sub

	go sm.startSubscription(sub)
	sm.saveSubscriptions()

	return nil
}

func (sm *SubscriptionManager) RemoveSubscription(id string) bool {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	_, exists := sm.subscriptions[id]
	if exists {
		// Stop the subscription job if running
		sm.stopSubscriptionJob(id)
		delete(sm.subscriptions, id)
		sm.saveSubscriptions()
		return true
	}
	return false
}

// stopSubscriptionJob stops a subscription job if it's running
func (sm *SubscriptionManager) stopSubscriptionJob(id string) {
	// Check if job exists
	stopChan, exists := sm.jobs[id]
	if !exists {
		return // Job not running
	}

	// Signal job to stop
	close(stopChan)
	delete(sm.jobs, id)
}

func (sm *SubscriptionManager) GetSubscription(id string) (*FlightSubscription, bool) {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()
	sub, exists := sm.subscriptions[id]
	return sub, exists
}

func (sm *SubscriptionManager) GetSubscriptionsForChannel(channelID string) []*FlightSubscription {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()

	var subs []*FlightSubscription
	for _, sub := range sm.subscriptions {
		if sub.ChannelID == channelID {
			subs = append(subs, sub)
		}
	}
	return subs
}

func (sm *SubscriptionManager) GetAllSubscriptions() []*FlightSubscription {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()

	var subs []*FlightSubscription
	for _, sub := range sm.subscriptions {
		subs = append(subs, sub)
	}
	return subs
}

func (sm *SubscriptionManager) StopAll() {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	for id := range sm.jobs {
		sm.stopSubscriptionJob(id)
	}
}

func (sm *SubscriptionManager) startSubscription(sub *FlightSubscription) {
	// Create and store the stop channel for this job
	sm.mutex.Lock()
	stopChan := make(chan struct{})
	sm.jobs[sub.ID] = stopChan
	sm.mutex.Unlock()

	ticker := time.NewTicker(time.Duration(sub.UpdateFrequency) * time.Second)
	defer ticker.Stop()

	fetchAndSendFlights := func() {
		now := time.Now()

		flights, err := sm.flightService.GetDepartureFlights(sub.Airport)
		if err != nil {
			sm.client.Log.Error("Failed to fetch flight data for subscription", 
				"subscription_id", sub.ID, 
				"airport", sub.Airport, 
				"channel_id", sub.ChannelID, 
				"error", err.Error())
			return
		}

		response := sm.flightService.FormatFlightResponse(flights, sub.Airport)

		// Check if channel still exists before sending update
		if !sm.isChannelValid(sub.ChannelID) {
			sm.client.Log.Info("Channel no longer exists, removing flight subscription", "channel_id", sub.ChannelID, "subscription_id", sub.ID)
			sm.cleanupInvalidSubscription(sub, "channel no longer exists")
			return
		}

		if err := sm.messageService.SendPublicMessage(sub.ChannelID, response); err != nil {
			sm.client.Log.Error("Failed to send flight update to channel", 
				"subscription_id", sub.ID, 
				"airport", sub.Airport, 
				"channel_id", sub.ChannelID, 
				"error", err.Error())
			return
		}

		sub.LastUpdated = now
		sm.saveSubscriptions()
	}

	fetchAndSendFlights()

	for {
		select {
		case <-ticker.C:
			fetchAndSendFlights()
		case <-stopChan:
			return
		}
	}
}

func (sm *SubscriptionManager) loadSubscriptions() error {
	var data []byte
	if appErr := sm.client.KV.Get("flight_subscriptions", &data); appErr != nil {
		sm.client.Log.Error("Error loading subscriptions from KV store", "error", appErr.Error())
		return fmt.Errorf("failed to load subscriptions from KV store: %w", appErr)
	}

	if data == nil {
		sm.client.Log.Info("No existing subscriptions found, starting with empty state")
		return nil
	}

	var subs map[string]*FlightSubscription
	if err := json.Unmarshal(data, &subs); err != nil {
		sm.client.Log.Error("Error unmarshaling subscription data", "error", err.Error())
		return fmt.Errorf("failed to parse subscription data: %w", err)
	}

	sm.mutex.Lock()
	sm.subscriptions = subs
	sm.mutex.Unlock()

	// Start subscriptions for all loaded subscriptions
	for _, sub := range subs {
		go sm.startSubscription(sub)
	}
	
	sm.client.Log.Info("Successfully loaded subscriptions", "count", len(subs))
	return nil
}

func (sm *SubscriptionManager) saveSubscriptions() {
	data, err := json.Marshal(sm.subscriptions)
	if err != nil {
		sm.client.Log.Error("Failed to marshal subscriptions for persistence", 
			"subscription_count", len(sm.subscriptions), 
			"error", err.Error())
		return
	}

	if _, appErr := sm.client.KV.Set("flight_subscriptions", data); appErr != nil {
		sm.client.Log.Error("Failed to save subscriptions to KV store", 
			"subscription_count", len(sm.subscriptions), 
			"error", appErr.Error())
	} else {
		sm.client.Log.Debug("Successfully saved subscriptions", 
			"subscription_count", len(sm.subscriptions))
	}
}

// isChannelValid checks if a channel still exists
func (sm *SubscriptionManager) isChannelValid(channelID string) bool {
	_, err := sm.client.Channel.Get(channelID)
	return err == nil
}

// cleanupInvalidSubscription removes a subscription and logs the reason
func (sm *SubscriptionManager) cleanupInvalidSubscription(sub *FlightSubscription, reason string) {
	sm.client.Log.Info("Cleaning up invalid flight subscription", 
		"subscription_id", sub.ID, 
		"channel_id", sub.ChannelID, 
		"airport", sub.Airport,
		"reason", reason)
	
	// Remove from subscriptions map (this will also stop the job)
	sm.RemoveSubscription(sub.ID)
	
	// Try to notify the user about the cleanup if possible
	sm.tryNotifyUserOfCleanup(sub, reason)
}

// tryNotifyUserOfCleanup attempts to send a DM to the user about subscription cleanup
func (sm *SubscriptionManager) tryNotifyUserOfCleanup(sub *FlightSubscription, reason string) {
	// Create a DM channel with the user
	dmChannel, err := sm.client.Channel.GetDirect(sm.messageService.GetBotUserID(), sub.UserID)
	if err != nil {
		sm.client.Log.Debug("Could not create DM channel for cleanup notification", "user_id", sub.UserID, "error", err)
		return
	}
	
	message := fmt.Sprintf("🧹 **Flight Subscription Cleanup**\n\n"+
		"Your flight subscription for **%s** airport (ID: `%s`) has been automatically removed because %s.\n\n"+
		"If you need flight updates, please set up a new subscription in an active channel.",
		sub.Airport, sub.ID, reason)
	
	post := &model.Post{
		ChannelId: dmChannel.Id,
		Message:   message,
		UserId:    sm.messageService.GetBotUserID(),
	}
	
	if err := sm.client.Post.CreatePost(post); err != nil {
		sm.client.Log.Debug("Could not send cleanup notification to user", "user_id", sub.UserID, "error", err)
	}
}
