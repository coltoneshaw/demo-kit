package main

import (
	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/pluginapi"
)

type MessageService struct {
	client    *pluginapi.Client
	botUserID string
}

func NewMessageService(client *pluginapi.Client, botUserID string) *MessageService {
	return &MessageService{
		client:    client,
		botUserID: botUserID,
	}
}

func (ms *MessageService) GetBotUserID() string {
	return ms.botUserID
}

func (ms *MessageService) SendEphemeralResponse(args *model.CommandArgs, message string) (*model.CommandResponse, error) {
	post := &model.Post{
		ChannelId: args.ChannelId,
		Message:   message,
	}
	return ms.sendResponse(post, args.UserId, true)
}

func (ms *MessageService) SendPublicResponse(args *model.CommandArgs, post *model.Post) (*model.CommandResponse, error) {
	return ms.sendResponse(post, args.UserId, false)
}

func (ms *MessageService) sendResponse(post *model.Post, userID string, isEphemeral bool) (*model.CommandResponse, error) {
	ms.sendBotPost(post, userID, isEphemeral)

	return &model.CommandResponse{
		ResponseType: model.CommandResponseTypeEphemeral,
		Text:         "",
	}, nil
}

func (ms *MessageService) sendBotPost(post *model.Post, userID string, isEphemeral bool) *model.Post {
	post.UserId = ms.botUserID

	if isEphemeral {
		ms.client.Post.SendEphemeralPost(userID, post)
		return post
	}

	ms.client.Post.CreatePost(post)
	return post
}