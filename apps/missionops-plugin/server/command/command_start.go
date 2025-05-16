package command

import (
	"bytes"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/coltoneshaw/demokit/missionops-plugin/server/mission"
	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/pluginapi"
	"github.com/pkg/errors"
)

func parseMissionStartArgs(command string, pluginAPI *pluginapi.Client) (*mission.MissionInfo, error) {
	commandArgs := parseArgs(command)

	name := commandArgs["name"]
	callsign := commandArgs["callsign"]
	departureAirport := strings.ToUpper(commandArgs["departureAirport"])
	arrivalAirport := strings.ToUpper(commandArgs["arrivalAirport"])

	// Parse crew members
	var crewUsernames []string
	if crew, ok := commandArgs["crew"]; ok && crew != "" {
		crewUsernames = strings.Fields(crew)
	}

	if len(crewUsernames) == 0 {
		return nil, errors.New("At least one crew member is required. Use `--crew @user1 @user2 ...`")
	}

	crewUserData := []model.User{}
	for _, username := range crewUsernames {
		// TODO - may need to clean the username.
		// Remove the @ symbol if it exists at the beginning of the username
		cleanUsername := username
		if strings.HasPrefix(username, "@") {
			cleanUsername = username[1:]
		}
		user, err := pluginAPI.User.GetByUsername(cleanUsername)
		if err != nil {
			return nil, errors.New(fmt.Sprintf("User not found: %s", username))
		}
		crewUserData = append(crewUserData, *user)
	}

	// Validate required parameters
	if name == "" {
		return nil, errors.New("Mission name is required. Use `--name [name]`")
	}

	if callsign == "" {
		return nil, errors.New("Mission callsign is required. Use `--callsign [callsign]`")
	}

	if departureAirport == "" {
		return nil, errors.New("Departure airport is required. Use `--departureAirport [code]`")
	}

	if arrivalAirport == "" {
		return nil, errors.New("Arrival airport is required. Use `--arrivalAirport [code]`")
	}

	return &mission.MissionInfo{
		Name:             name,
		Callsign:         callsign,
		DepartureAirport: departureAirport,
		ArrivalAirport:   arrivalAirport,
		Crew:             crewUserData,
	}, nil
}

// executeMissionStartCommand handles the /mission start command
func (c *Handler) executeMissionStartCommand(args *model.CommandArgs) (*model.CommandResponse, error) {

	parsedMissionInfo, err := parseMissionStartArgs(args.Command, c.client)
	if err != nil {
		return c.logCommandError(fmt.Sprintf("Error parsing mission start arguments %v", err)), err
	}

	// Create the Mattermost channel name (callsign-name)
	// Ensure it's lowercase and replace spaces with dashes
	channelName := strings.ToLower(fmt.Sprintf("%s-%s", parsedMissionInfo.Callsign, parsedMissionInfo.Callsign))
	channelName = strings.ReplaceAll(channelName, " ", "-")

	// Get status emoji for initial status ("stalled")
	initialStatusEmoji := c.mission.GetStatusEmoji("stalled")

	// Create the channel with status emoji in display name
	channel := &model.Channel{
		TeamId:      args.TeamId,
		Name:        channelName,
		DisplayName: fmt.Sprintf("%s %s: %s", initialStatusEmoji, parsedMissionInfo.Callsign, parsedMissionInfo.Name),
		Type:        model.ChannelTypeOpen,
	}

	err = c.client.Channel.Create(channel)
	if err != nil {
		return c.logCommandError(fmt.Sprintf("Error creating channel: %v", err)), err
	}

	// Categorize the mission channel into "Active Missions" category using Playbooks API
	if err := c.mission.CategorizeMissionChannel(channel.Id, channel.TeamId); err != nil {
		return c.logCommandError(fmt.Sprintf("Error categorizing mission channel", "error", err.Error())), err
	}

	crewIds := make([]string, len(parsedMissionInfo.Crew))
	crewUsernames := make([]string, len(parsedMissionInfo.Crew))
	// Add all users to the channel
	for _, user := range parsedMissionInfo.Crew {
		if _, err := c.client.Channel.AddUser(channel.Id, user.Id, c.bot.GetBotUserInfo().UserId); err != nil {
			return c.logCommandError(fmt.Sprintf("Error adding user to channel", "userId", user.Id, "error", err.Error())), err
		}
		crewIds = append(crewIds, user.Id)
		crewUsernames = append(crewUsernames, user.Username)
	}

	mission := &mission.Mission{
		ID:               model.NewId(),
		Name:             parsedMissionInfo.Name,
		Callsign:         parsedMissionInfo.Callsign,
		DepartureAirport: parsedMissionInfo.DepartureAirport,
		ArrivalAirport:   parsedMissionInfo.ArrivalAirport,
		CreatedBy:        args.UserId,
		CreatedAt:        time.Now(),
		Crew:             crewIds,
		ChannelID:        channel.Id,
		TeamID:           channel.TeamId,
		ChannelName:      channelName,
		Status:           "stalled",
	}

	// Add the mission to the KV store
	if err := c.mission.AddMission(mission); err != nil {
		return c.logCommandError(fmt.Sprintf("Error saving the mission", "error", err.Error())), err
	}

	// Send a message to the new channel with mission details
	missionDetails := fmt.Sprintf("# Mission Created: %s\n\n"+
		"**Callsign:** %s\n"+
		"**Departure:** %s\n"+
		"**Arrival:** %s\n"+
		"**Status:** %s\n"+
		"**Crew:** %s\n\n",
		parsedMissionInfo.Name, parsedMissionInfo.Callsign, parsedMissionInfo.DepartureAirport, parsedMissionInfo.ArrivalAirport, mission.Status, strings.Join(crewUsernames, ", "))

	_, err = c.bot.PostMessageFromBot(channel.Id, missionDetails)
	if err != nil {
		return c.logCommandError(fmt.Sprintf("Error sending message to channel", "error", err.Error())), err
	}

	// Upload the flight plan PDF to the channel
	err = c.UploadFlightPlanPDF(channel.Id)
	if err != nil {
		return c.logCommandError(fmt.Sprintf("Error uploading flight plan PDF", "error", err.Error())), err
	}

	// Execute weather commands for departure and arrival airports
	c.executeWeatherCommands(mission)

	time.Sleep(1 * time.Second)
	// First send a message explaining what we're doing
	introMsg := "🌤️ **Checking Weather for Mission** 🌤️\n\nGetting current weather conditions for departure and arrival airports..."
	_, err = c.bot.PostMessageFromBot(channel.Id, introMsg)
	if err != nil {
		c.client.Log.Error("Error sending intro message", "error", err.Error())
	}

	// Send success response back to original channel
	return &model.CommandResponse{
		ResponseType: model.CommandResponseTypeInChannel,
		Text:         fmt.Sprintf("✅ Mission **%s** created with callsign **%s**. Channel: ~%s", mission.Name, mission.Callsign, channelName),
	}, nil
}

// UploadFlightPlanPDF uploads the embedded flight plan PDF to a channel
func (c *Handler) UploadFlightPlanPDF(channelID string) error {

	// Add a slight delay to ensure the channel message is sent first
	time.Sleep(500 * time.Millisecond)

	// Send a message about the flight plan
	flightPlanMsg := "# Flight Plan Documents"
	post, err := c.bot.PostMessageFromBot(channelID, flightPlanMsg)
	if err != nil {
		return errors.Wrap(err, "failed to send flight plan message")
	}

	flightPlanPDF, err := os.ReadFile(filepath.Join(c.bot.GetBundlePath(), "assets", "USAF_Flight_Plan_Mock.png"))
	if err != nil {
		return errors.Wrap(err, "failed to read profile image")
	}

	// Upload file to Mattermost using the embedded PDF data
	fileInfo, err := c.client.File.Upload(bytes.NewReader(flightPlanPDF), channelID, "USAF_Flight_Plan_Mock.pdf")
	if err != nil {
		return err
	}

	// Attach the file to the post
	post.FileIds = append(post.FileIds, fileInfo.Id)
	c.client.Post.UpdatePost(post)
	return nil
}

func (c *Handler) executeWeatherCommands(mission *mission.Mission) {

	// Execute weather command for departure airport (if weather plugin is installed)
	departureCmd := &model.CommandArgs{
		Command:   fmt.Sprintf("/weather --location %s", mission.DepartureAirport),
		ChannelId: mission.ChannelID,
		UserId:    c.bot.GetBotUserInfo().UserId,
		TeamId:    mission.TeamID,
	}
	if _, err := c.client.SlashCommand.Execute(departureCmd); err != nil {
		c.client.Log.Error("Error executing departure weather command", "error", err.Error())

		// If command execution fails, send a fallback message
		fallbackMsg := fmt.Sprintf("Could not automatically check weather for departure airport (%s). You can check manually with: `/weather %s`", mission.DepartureAirport, mission.DepartureAirport)
		_, err := c.bot.PostMessageFromBot(mission.ChannelID, fallbackMsg)
		if err != nil {
			c.client.Log.Error("Error sending fallback message", "error", err)
		}
	}

	// Add delay between commands
	time.Sleep(2 * time.Second)

	// Execute weather command for arrival airport
	arrivalCmd := &model.CommandArgs{
		Command:   fmt.Sprintf("/weather %s", mission.ArrivalAirport),
		ChannelId: mission.ChannelID,
		UserId:    c.bot.GetBotUserInfo().UserId,
		TeamId:    mission.TeamID,
	}
	if _, err := c.client.SlashCommand.Execute(arrivalCmd); err != nil {
		c.client.Log.Error("Error executing arrival weather command", "error", err.Error())

		// If command execution fails, send a fallback message
		fallbackMsg := fmt.Sprintf("Could not automatically check weather for arrival airport (%s). You can check manually with: `/weather %s`", mission.ArrivalAirport, mission.ArrivalAirport)
		_, err := c.bot.PostMessageFromBot(mission.ChannelID, fallbackMsg)
		if err != nil {
			c.client.Log.Error("Error sending fallback message", "error", err)
		}
	}

}
