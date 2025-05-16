package bot

import (
	"fmt"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/pluginapi"
	"github.com/pkg/errors"
)

type MissionBot struct {
	Bot *model.Bot
	// BotToken is the token for the bot account.
	Token string

	client *pluginapi.Client

	bundlepath string
}

type BotInterface interface {
	// SendBotDM sends a direct message from the bot to a user
	PostMessageFromBot(channelID, message string) (*model.Post, error)
	// GetBotToken returns the bot token
	GetBotToken() string
	// GetBotUserID returns the bot's user ID
	GetBotUserInfo() *model.Bot

	SetBundlePath(path string)

	// GetBundlePath returns the bundle path
	GetBundlePath() string

	// EnsureTeamMember makes sure the bot is a member of the specified team
	EnsureTeamMember(teamID string) error
}

func NewBotHandler(client *pluginapi.Client) BotInterface {

	botID, err := client.Bot.EnsureBot(
		&model.Bot{
			Username:    "missionops",
			DisplayName: "Mission Ops Bot",
			Description: "A bot for managing mission operations.",
		},
	)

	botUser := &model.Bot{
		UserId: botID,
	}

	if err != nil {
		return nil
	}

	newBot := &MissionBot{
		Bot:    botUser,
		client: client,
	}

	if err := newBot.fetchOrCreateBotToken(); err != nil {
		client.Log.Error("Error fetching or creating bot token", "error", err.Error())
		return nil
	}

	return newBot
}

// SetBundlePath sets the bundle path for the bot
func (b *MissionBot) SetBundlePath(path string) {
	b.bundlepath = path
}

// GetBundlePath returns the bundle path for the bot
func (b *MissionBot) GetBundlePath() string {
	return b.bundlepath
}

// SendBotDM sends a message from the bot to a channel
func (b *MissionBot) PostMessageFromBot(channelID, message string) (*model.Post, error) {
	post := &model.Post{
		UserId:    b.Bot.UserId,
		ChannelId: channelID,
		Message:   message,
	}
	err := b.client.Post.CreatePost(post)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create post")
	}

	return post, nil
}

// GetBotUserInfo returns the bot's user ID
func (b *MissionBot) GetBotUserInfo() *model.Bot {
	return b.Bot
}

// EnsureTeamMember makes sure the bot is a member of the specified team
func (b *MissionBot) EnsureTeamMember(teamID string) error {
	if teamID == "" {
		return fmt.Errorf("team ID is required")
	}

	// Try to get the team member to check if the bot is already in the team
	_, err := b.client.Team.GetMember(teamID, b.Bot.UserId)
	if err != nil {
		// Bot is not in the team, try to add it
		b.client.Log.Info("Bot is not in the team, adding it now", "teamId", teamID)

		_, err = b.client.Team.CreateMember(teamID, b.Bot.UserId)
		if err != nil {
			return errors.Wrap(err, "failed to add bot to team")
		}
		b.client.Log.Info("Successfully added bot to team", "teamId", teamID)
	}

	return nil
}

func (b *MissionBot) fetchOrCreateBotToken() error {
	var kvToken []byte
	err := b.client.KV.Get("bot_token", &kvToken)
	if err != nil {
		return errors.Wrap(err, "failed to get bot token from KV store")
	}

	if kvToken != nil {
		b.Token = string(kvToken)
		return nil
	}

	token, err := b.client.User.CreateAccessToken(b.Bot.UserId, "Mission Ops Bot Token")
	if err != nil {
		return errors.Wrap(err, "failed to create bot token")
	}

	kvSet, err := b.client.KV.Set("bot_token", []byte(token.Token))
	if !kvSet {
		return errors.Wrap(err, "failed to set bot token in KV store")
	}

	b.Token = token.Token
	return nil
}

func (b *MissionBot) GetBotToken() string {
	if b.Token == "" {
		if err := b.fetchOrCreateBotToken(); err != nil {
			b.client.Log.Error("Error fetching bot token", "error", err.Error())
		}
	}
	return b.Token
}
