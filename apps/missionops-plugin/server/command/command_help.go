package command

import "github.com/mattermost/mattermost/server/public/model"

// executeMissionHelpCommand handles the /mission help command
func (p *Handler) executeMissionHelpCommand(args *model.CommandArgs) (*model.CommandResponse, error) {
	helpText := "**Mission Operations Commands**\n\n" +
		"**Mission Commands:**\n" +
		"- `/mission start --name [name] --callsign [callsign] --departureAirport [code] --arrivalAirport [code] --crew @user1 @user2 ...` - Create a new mission\n" +
		"- `/mission list` - List all missions\n" +
		"- `/mission status [status]` - Update mission status (run in mission channel to skip --id)\n" +
		"- `/mission complete` - Fill out and submit a post-mission report form\n" +
		"- `/mission help` - Show this help message\n\n" +
		"**Subscription Commands:**\n" +
		"- `/mission subscribe --type [status1,status2] --frequency [seconds]` - Subscribe to mission status updates\n" +
		"- `/mission subscribe --type all --frequency [seconds]` - Subscribe to all mission status updates\n" +
		"- `/mission unsubscribe --id [subscription_id]` - Unsubscribe from updates\n" +
		"- `/mission subscriptions` - List all subscriptions in this channel\n\n" +
		"**Valid Statuses:**\n" +
		"- `stalled` - Mission is not active\n" +
		"- `in-air` - Mission is in progress\n" +
		"- `completed` - Mission has been completed successfully\n" +
		"- `cancelled` - Mission has been cancelled\n\n" +
		"**Examples:**\n" +
		"- `/mission start --name Alpha --callsign Eagle1 --departureAirport JFK --arrivalAirport LAX --crew @john @sarah`\n" +
		"- `/mission status in-air`\n" +
		"- `/mission status completed`\n" +
		"- `/mission status cancelled --id [mission_id]` (when not in mission channel)\n" +
		"- `/mission complete` (in a mission channel)\n" +
		"- `/mission subscribe --type stalled,in-air --frequency 3600` (updates hourly)\n" +
		"- `/mission subscribe --type all --frequency 1800` (updates every 30 minutes)"

	return &model.CommandResponse{
		ResponseType: model.CommandResponseTypeEphemeral,
		Text:         helpText,
	}, nil
}
