package main

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"
)

// startMissionSubscription starts a subscription to mission status updates
func startMissionSubscription(sub *MissionSubscription, sm *SubscriptionManager, mm *MissionManager, client *Client) {
	log.Printf("Starting mission subscription %s for channel %s", sub.ID, sub.ChannelID)

	// Set up ticker for periodic updates
	ticker := time.NewTicker(time.Duration(sub.UpdateFrequency) * time.Second)
	defer ticker.Stop()

	// Function to fetch and send mission updates
	fetchAndSendMissionUpdates := func() {
		// Get current time
		now := time.Now()

		// Log subscription details
		log.Printf("Fetching mission updates for subscription %s (Channel: %s, Status Types: %v)", 
			sub.ID, sub.ChannelID, sub.StatusTypes)

		// Create context for the operation
		ctx := context.Background()

		// Get missions based on subscription status types
		var missions []*Mission
		if len(sub.StatusTypes) == 0 {
			// If no status types specified, get all missions
			missions = mm.GetAllMissions()
		} else {
			// Otherwise, get missions for each status type
			for _, statusType := range sub.StatusTypes {
				statusMissions := mm.GetMissionsByStatus(statusType)
				missions = append(missions, statusMissions...)
			}
		}

		// If no missions found, log and return
		if len(missions) == 0 {
			log.Printf("No missions found for subscription %s", sub.ID)
			return
		}

		// Format the mission updates
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("# Mission Status Update (%s)\n\n", now.Format(time.RFC1123)))
		sb.WriteString("| Name | Callsign | Departure | Arrival | Status | Channel |\n")
		sb.WriteString("|------|----------|-----------|---------|--------|--------|\n")

		for _, mission := range missions {
			sb.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %s | ~%s |\n",
				mission.Name, mission.Callsign, mission.DepartureAirport, mission.ArrivalAirport,
				mission.Status, mission.ChannelName))
		}

		// Add a note about subscription
		statusTypesText := "all statuses"
		if len(sub.StatusTypes) > 0 {
			statusTypesText = strings.Join(sub.StatusTypes, ", ")
		}
		sb.WriteString(fmt.Sprintf("\n\n*This is an automated update for mission statuses: %s. Updates every %d seconds.*", 
			statusTypesText, sub.UpdateFrequency))

		// Send to Mattermost with the subscription's channel ID
		log.Printf("Sending mission update to channel ID: %s", sub.ChannelID)
		_, err := SendPost(ctx, client, sub.ChannelID, sb.String())
		if err != nil {
			log.Printf("Error sending message to Mattermost for subscription %s: %v", sub.ID, err)
			return
		}

		// Update last updated time
		sub.LastUpdated = now
		sm.SaveToFile()
	}

	// Fetch and send initial data
	fetchAndSendMissionUpdates()

	// Wait for updates or cancellation
	for {
		select {
		case <-ticker.C:
			fetchAndSendMissionUpdates()
		case <-sub.StopChan:
			log.Printf("Stopping subscription %s", sub.ID)
			return
		}
	}
}

// restartSubscriptions restarts all existing subscriptions at startup
func restartSubscriptions(sm *SubscriptionManager, mm *MissionManager, client *Client) {
	subCount := len(sm.Subscriptions)
	if subCount > 0 {
		log.Printf("Restarting %d existing mission subscriptions...", subCount)
		for id, sub := range sm.Subscriptions {
			log.Printf("Restarting subscription %s for channel %s", id, sub.ChannelID)
			go startMissionSubscription(sub, sm, mm, client)
		}
	}
}

// notifySubscribersOfStatusChange notifies all relevant subscribers when a mission status changes
func notifySubscribersOfStatusChange(mission *Mission, oldStatus string, sm *SubscriptionManager, client *Client) {
	// Find all subscriptions that care about this status change
	subs := sm.GetSubscriptionsForStatus(mission.Status)
	
	if len(subs) == 0 {
		log.Printf("No subscriptions found for status: %s", mission.Status)
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

	// Create context for the operation
	ctx := context.Background()

	// Send notification to each subscribed channel
	for _, sub := range subs {
		// Skip the mission's own channel (it already gets notifications)
		if sub.ChannelID == mission.ChannelID {
			continue
		}

		log.Printf("Sending status change notification to channel ID: %s for subscription %s", 
			sub.ChannelID, sub.ID)
		_, err := SendPost(ctx, client, sub.ChannelID, statusChangeMsg)
		if err != nil {
			log.Printf("Error sending status change notification for subscription %s: %v", sub.ID, err)
		}
	}
}