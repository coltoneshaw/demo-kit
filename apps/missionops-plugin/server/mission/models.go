package mission

import (
	"time"

	"github.com/coltoneshaw/demokit/missionops-plugin/server/bot"
	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/pluginapi"
)

// Mission represents a mission with its properties
type Mission struct {
	ID               string    `json:"id"`
	Name             string    `json:"name"`
	Callsign         string    `json:"callsign"`
	DepartureAirport string    `json:"departureAirport"`
	ArrivalAirport   string    `json:"arrivalAirport"`
	CreatedBy        string    `json:"createdBy"`
	CreatedAt        time.Time `json:"createdAt"`
	Crew             []string  `json:"crew"`
	ChannelID        string    `json:"channelId"`
	TeamID           string    `json:"teamId"`
	ChannelName      string    `json:"channelName"`
	Status           string    `json:"status"`
	CompletedAt      time.Time `json:"completedAt,omitempty"`

	client *pluginapi.Client
	bot    bot.BotInterface
}

type MissionInfo struct {
	Name             string
	Callsign         string
	DepartureAirport string
	ArrivalAirport   string
	Crew             []model.User
}

// MattermostResponse is a response to send back to Mattermost
type MattermostResponse struct {
	Text         string `json:"text"`
	ResponseType string `json:"response_type"`
	ChannelID    string `json:"channel_id,omitempty"`
	Username     string `json:"username,omitempty"`
	IconURL      string `json:"icon_url,omitempty"`
}

// KVStorePrefixes for organization
const (
	MissionPrefix   = "mission_"
	MissionsListKey = "missions_list"
)

// GetStatusEmoji returns an emoji for a given status
func (*Mission) GetStatusEmoji(status string) string {
	switch status {
	case "stalled":
		return "🔴"
	case "in-air":
		return "✈️"
	case "completed":
		return "✅"
	case "cancelled":
		return "❌"
	default:
		return "❓"
	}
}
