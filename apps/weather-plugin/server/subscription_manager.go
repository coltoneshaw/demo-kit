package main

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/pluginapi"
)

type SubscriptionManager struct {
	client         *pluginapi.Client
	subscriptions  map[string]*Subscription
	mutex          sync.RWMutex
	weatherService *WeatherService
	formatter      *WeatherFormatter
	messageService *MessageService
}

func NewSubscriptionManager(client *pluginapi.Client, weatherService *WeatherService, formatter *WeatherFormatter, messageService *MessageService) *SubscriptionManager {
	sm := &SubscriptionManager{
		client:         client,
		subscriptions:  make(map[string]*Subscription),
		weatherService: weatherService,
		formatter:      formatter,
		messageService: messageService,
	}
	
	sm.loadSubscriptions()
	return sm
}

func (sm *SubscriptionManager) AddSubscription(sub *Subscription) {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()
	
	sm.subscriptions[sub.ID] = sub
	sm.saveSubscriptions()
}

func (sm *SubscriptionManager) RemoveSubscription(id string) bool {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()
	
	sub, exists := sm.subscriptions[id]
	if exists {
		if sub.StopChan != nil {
			close(sub.StopChan)
		}
		delete(sm.subscriptions, id)
		sm.saveSubscriptions()
		return true
	}
	return false
}

func (sm *SubscriptionManager) GetSubscription(id string) (*Subscription, bool) {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()
	
	sub, exists := sm.subscriptions[id]
	return sub, exists
}

func (sm *SubscriptionManager) GetSubscriptionsForChannel(channelID string) []*Subscription {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()
	
	var subs []*Subscription
	for _, sub := range sm.subscriptions {
		if sub.ChannelID == channelID {
			subs = append(subs, sub)
		}
	}
	return subs
}

func (sm *SubscriptionManager) GetAllSubscriptions() []*Subscription {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()
	
	var subs []*Subscription
	for _, sub := range sm.subscriptions {
		subs = append(subs, sub)
	}
	return subs
}



func (sm *SubscriptionManager) StartSubscription(sub *Subscription) {
	// Get initial weather data
	weatherData, err := sm.weatherService.GetWeatherData(sub.Location)
	if err != nil {
		sm.client.Log.Error("Error fetching initial weather data for subscription", "error", err, "subscription_id", sub.ID)
		errorMsg := fmt.Sprintf("⚠️ Could not fetch weather data for subscription to **%s** (ID: `%s`): %v", sub.Location, sub.ID, err)
		
		args := &model.CommandArgs{
			ChannelId: sub.ChannelID,
			UserId: sub.UserID,
		}
		sm.messageService.SendEphemeralResponse(args, errorMsg)
	} else {
		post := sm.formatter.FormatAsAttachment(weatherData, sub.ChannelID, sm.messageService.GetBotUserID())
		args := &model.CommandArgs{
			ChannelId: sub.ChannelID,
			UserId: sub.UserID,
		}
		sm.messageService.SendPublicResponse(args, post)
	}

	ticker := time.NewTicker(time.Duration(sub.UpdateFrequency) * time.Millisecond)
	defer ticker.Stop()

	consecutiveFailures := 0
	maxConsecutiveFailures := 5

	for {
		select {
		case <-ticker.C:
			weatherData, err := sm.weatherService.GetWeatherData(sub.Location)

			if err != nil {
				consecutiveFailures++
				sm.client.Log.Error("Error fetching weather data for subscription", "error", err, "subscription_id", sub.ID, "failures", consecutiveFailures)

				if consecutiveFailures == 1 || consecutiveFailures == maxConsecutiveFailures {
					errorMsg := fmt.Sprintf("⚠️ Error updating weather for **%s**: %v", sub.Location, err)
					args := &model.CommandArgs{
						ChannelId: sub.ChannelID,
						UserId: sub.UserID,
					}
					sm.messageService.SendEphemeralResponse(args, errorMsg)
				}

				if consecutiveFailures >= maxConsecutiveFailures {
					ticker.Reset(time.Duration(sub.UpdateFrequency*2) * time.Millisecond)
				}
				continue
			}

			if consecutiveFailures > 0 {
				sm.client.Log.Info("Successfully recovered subscription after failures", "subscription_id", sub.ID, "failures", consecutiveFailures)
				consecutiveFailures = 0
				ticker.Reset(time.Duration(sub.UpdateFrequency) * time.Millisecond)
			}

			post := sm.formatter.FormatAsAttachment(weatherData, sub.ChannelID, sm.messageService.GetBotUserID())
			args := &model.CommandArgs{
				ChannelId: sub.ChannelID,
				UserId: sub.UserID,
			}
			sm.messageService.SendPublicResponse(args, post)

			sub.LastUpdated = time.Now()

		case <-sub.StopChan:
			sm.client.Log.Info("Stopping subscription", "subscription_id", sub.ID)
			return
		}
	}
}

func (sm *SubscriptionManager) StopAll() {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()
	
	for _, sub := range sm.subscriptions {
		if sub.StopChan != nil {
			close(sub.StopChan)
		}
	}
}

func (sm *SubscriptionManager) saveSubscriptions() {
	data, err := json.Marshal(sm.subscriptions)
	if err != nil {
		sm.client.Log.Error("Failed to marshal subscriptions", "error", err)
		return
	}
	
	if _, err := sm.client.KV.Set("weather_subscriptions", data); err != nil {
		sm.client.Log.Error("Failed to save subscriptions", "error", err)
	}
}

func (sm *SubscriptionManager) loadSubscriptions() {
	var data []byte
	err := sm.client.KV.Get("weather_subscriptions", &data)
	if err != nil {
		sm.client.Log.Warn("Failed to load subscriptions", "error", err)
		return
	}
	
	if data == nil {
		sm.client.Log.Debug("No existing subscriptions found")
		return
	}
	
	var subscriptions map[string]*Subscription
	if err := json.Unmarshal(data, &subscriptions); err != nil {
		sm.client.Log.Error("Failed to unmarshal subscriptions", "error", err)
		return
	}
	
	sm.mutex.Lock()
	sm.subscriptions = subscriptions
	
	for _, sub := range sm.subscriptions {
		sub.StopChan = make(chan struct{})
	}
	sm.mutex.Unlock()
	
	sm.client.Log.Info("Loaded subscriptions", "count", len(subscriptions))
}