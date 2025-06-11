package main

import (
	"encoding/json"
	"sync"

	"github.com/mattermost/mattermost/server/public/pluginapi"
)

type SubscriptionManager struct {
	client        *pluginapi.Client
	subscriptions map[string]*Subscription
	mutex         sync.RWMutex
}

func NewSubscriptionManager(client *pluginapi.Client) *SubscriptionManager {
	sm := &SubscriptionManager{
		client:        client,
		subscriptions: make(map[string]*Subscription),
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