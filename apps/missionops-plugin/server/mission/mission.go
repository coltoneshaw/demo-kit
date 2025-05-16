package mission

import (
	"github.com/coltoneshaw/demokit/missionops-plugin/server/bot"
	"github.com/mattermost/mattermost/server/public/pluginapi"
)

type MissionInterface interface {
	// AddMission adds a new mission
	AddMission(mission *Mission) error
	// GetMission retrieves a mission by ID
	GetMission(id string) (*Mission, error)
	GetMissionByChannelID(channelID string) (*Mission, error)
	UpdateMissionStatus(id string, status string) error
	GetAllMissions() ([]*Mission, error)
	GetMissionsByStatus(status string) ([]*Mission, error)
	GetStatusEmoji(status string) string
	CategorizeMissionChannel(channelID, teamID string) error
	CompleteMission(missionID, objectivesCompletion, notableEvents, crewPerformance, missionDurationStr, userID string) error
}

func NewMissionHandler(client *pluginapi.Client, bot bot.BotInterface) MissionInterface {
	return &Mission{
		client: client,
		bot:    bot,
	}
}
