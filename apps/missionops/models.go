package main

import (
	"time"
)

// Mission represents a mission with all its details
type Mission struct {
	ID              string    `json:"id"`              // Unique identifier for the mission
	Name            string    `json:"name"`            // Mission name
	Callsign        string    `json:"callsign"`        // Mission callsign
	DepartureAirport string   `json:"departureAirport"` // Departure airport code
	ArrivalAirport  string    `json:"arrivalAirport"`  // Arrival airport code
	CreatedBy       string    `json:"createdBy"`       // User ID who created the mission
	CreatedAt       time.Time `json:"createdAt"`       // When the mission was created
	Crew            []string  `json:"crew"`            // List of user IDs in the crew
	ChannelID       string    `json:"channelID"`       // Channel ID for the mission
	ChannelName     string    `json:"channelName"`     // Channel name for the mission
	Status          string    `json:"status"`          // Mission status (stalled, in air, completed, cancelled)
}

// MissionSubscription represents a subscription to mission status updates
type MissionSubscription struct {
	ID              string        `json:"id"`               // Unique identifier for the subscription
	ChannelID       string        `json:"channel_id"`       // Channel to post updates to
	UserID          string        `json:"user_id"`          // User who created the subscription
	StatusTypes     []string      `json:"status_types"`     // Types of mission statuses to receive updates for (empty means all)
	UpdateFrequency int64         `json:"update_frequency"` // How often to update (in seconds)
	LastUpdated     time.Time     `json:"last_updated"`     // When the subscription was last updated
	StopChan        chan struct{} `json:"-"`                // Channel to signal stopping the subscription
}

// MattermostPayload represents the incoming webhook payload from Mattermost
type MattermostPayload struct {
	Text       string `json:"text"`
	UserID     string `json:"user_id"`
	Username   string `json:"user_name"`
	ChannelID  string `json:"channel_id"`
	ChannelName string `json:"channel_name"`
	TeamName   string `json:"team_name"`
	Command    string `json:"command"`
	TeamDomain string `json:"team_domain"`
	Token      string `json:"token"`
}

// MattermostResponse represents the response to send back to Mattermost
type MattermostResponse struct {
	Text         string `json:"text"`
	ResponseType string `json:"response_type"`
	ChannelID    string `json:"channel_id,omitempty"`
}