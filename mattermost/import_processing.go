package mattermost

import (
	"archive/zip"
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/sirupsen/logrus"
)

// Global storage for channel memberships during import processing
// Simple map: username -> list of channel names
var globalChannelMemberships = make(map[string][]string)

// Global storage for teams during import processing
var globalImportedTeams = make([]string, 0)

// Global storage for current import file path
var globalCurrentImportPath string

// Global variables for timestamp adjustment
var (
	timestampOffset  int64 = 0
	offsetCalculated bool  = false
)

// JSON helper functions for clean, readable code

// getNestedString safely gets a string value from nested JSON data
func getNestedString(data map[string]any, keys ...string) string {
	current := data
	for i, key := range keys {
		if i == len(keys)-1 {
			// Last key - get the string value
			if val, ok := current[key].(string); ok {
				return val
			}
			return ""
		}
		// Navigate deeper
		if next, ok := current[key].(map[string]any); ok {
			current = next
		} else {
			return ""
		}
	}
	return ""
}

// extractAllChannelNames gets all channel names from the user data
func extractAllChannelNames(data map[string]any) []string {
	var channels []string

	user, ok := data["user"].(map[string]any)
	if !ok {
		return channels
	}

	teams, ok := user["teams"].([]any)
	if !ok {
		return channels
	}

	for _, teamIntf := range teams {
		if team, ok := teamIntf.(map[string]any); ok {
			if channelsIntf, ok := team["channels"].([]any); ok {
				for _, chIntf := range channelsIntf {
					if ch, ok := chIntf.(map[string]any); ok {
						if name, ok := ch["name"].(string); ok {
							channels = append(channels, name)
						}
					}
				}
			}
		}
	}

	return channels
}

// setDefaultChannels replaces all channel arrays with default channels
func setDefaultChannels(data map[string]any) {
	user, ok := data["user"].(map[string]any)
	if !ok {
		return
	}

	teams, ok := user["teams"].([]any)
	if !ok {
		return
	}

	defaultChannels := []any{
		map[string]any{"name": "town-square", "roles": "channel_user"},
		map[string]any{"name": "off-topic", "roles": "channel_user"},
	}

	for _, teamIntf := range teams {
		if team, ok := teamIntf.(map[string]any); ok {
			team["channels"] = defaultChannels
		}
	}
}

// extractChannelMemberships extracts channel memberships from user JSON and returns cleaned user JSON
func extractChannelMemberships(userLine string) (string, error) {
	// Parse the JSON
	var data map[string]any
	if err := json.Unmarshal([]byte(userLine), &data); err != nil {
		return "", fmt.Errorf("failed to parse user JSON: %w", err)
	}

	// Extract username using helper
	username := getNestedString(data, "user", "username")
	if username == "" {
		return "", fmt.Errorf("missing username")
	}

	// Extract all channels using helper
	channels := extractAllChannelNames(data)

	// Store channels for API processing
	if len(channels) > 0 {
		globalChannelMemberships[username] = channels
	}

	// Replace with default channels using helper
	setDefaultChannels(data)

	// Marshal back to JSON
	cleanedJSON, err := json.Marshal(data)
	if err != nil {
		return "", fmt.Errorf("failed to marshal cleaned user JSON: %w", err)
	}

	Log.WithFields(logrus.Fields{
		"username":        username,
		"channels_stored": len(channels),
	}).Debug("✅ Extracted and cleaned user")

	return string(cleanedJSON), nil
}

// adjustPostTimestamps adjusts post timestamps to be recent while preserving relative order
func adjustPostTimestamps(postLine string) (string, error) {
	// Calculate offset once on first post
	if !offsetCalculated {
		if err := calculateTimestampOffset(); err != nil {
			Log.WithFields(logrus.Fields{"error": err.Error()}).Warn("⚠️ Failed to calculate timestamp offset")
			return postLine, nil
		}
		offsetCalculated = true
		Log.WithFields(logrus.Fields{"offset_hours": timestampOffset / (1000 * 60 * 60)}).Info("📅 Calculated timestamp offset for recent posts")
	}

	// Parse and adjust timestamps
	var data map[string]any
	if err := json.Unmarshal([]byte(postLine), &data); err != nil {
		return "", fmt.Errorf("failed to parse post JSON: %w", err)
	}

	adjustAllTimestamps(data)

	// Marshal back to JSON
	adjustedJSON, err := json.Marshal(data)
	if err != nil {
		return "", fmt.Errorf("failed to marshal adjusted post JSON: %w", err)
	}

	return string(adjustedJSON), nil
}

// adjustAllTimestamps applies offset to all timestamp fields in post data
func adjustAllTimestamps(data map[string]any) {
	post, ok := data["post"].(map[string]any)
	if !ok {
		return
	}

	// Main post timestamp
	adjustTimestampField(post, "create_at")

	// Reply timestamps
	if replies, ok := post["replies"].([]any); ok {
		for _, replyIntf := range replies {
			if reply, ok := replyIntf.(map[string]any); ok {
				adjustTimestampField(reply, "create_at")
			}
		}
	}

	// Call post timestamps in props
	if props, ok := post["props"].(map[string]any); ok {
		adjustTimestampField(props, "start_at")
		adjustTimestampField(props, "end_at")
	}
}

// adjustTimestampField adds offset to a single timestamp field
func adjustTimestampField(obj map[string]any, field string) {
	if timestamp, ok := obj[field].(float64); ok {
		obj[field] = int64(timestamp) + timestampOffset
	}
}

// calculateTimestampOffset calculates how much to shift timestamps to make posts recent
func calculateTimestampOffset() error {
	maxTimestamp, err := findLatestTimestamp()
	if err != nil {
		return err
	}

	// Make newest post ~5 minutes ago
	now := time.Now().Unix() * 1000
	fiveMinutesAgo := now - (5 * 60 * 1000)
	timestampOffset = fiveMinutesAgo - maxTimestamp

	return nil
}

// findLatestTimestamp scans all posts to find the most recent timestamp
func findLatestTimestamp() (int64, error) {
	// Use the global import path if set, otherwise find the default
	bulkImportPath := globalCurrentImportPath
	if bulkImportPath == "" {
		path, err := findBulkImportPath()
		if err != nil {
			return 0, err
		}
		bulkImportPath = path
	}

	file, err := os.Open(bulkImportPath)
	if err != nil {
		return 0, err
	}
	defer closeWithLog(file, "bulk import file")

	var maxTimestamp int64
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.Contains(line, `"type": "post"`) {
			continue
		}

		var data map[string]any
		if json.Unmarshal([]byte(line), &data) != nil {
			continue
		}

		// Extract all timestamps from this post
		timestamps := extractAllTimestampsFromPost(data)
		for _, ts := range timestamps {
			if ts > maxTimestamp {
				maxTimestamp = ts
			}
		}
	}

	if maxTimestamp == 0 {
		return 0, fmt.Errorf("no post timestamps found")
	}

	return maxTimestamp, nil
}

// extractAllTimestampsFromPost gets all timestamps from a post (main + replies + props)
func extractAllTimestampsFromPost(data map[string]any) []int64 {
	var timestamps []int64

	post, ok := data["post"].(map[string]any)
	if !ok {
		return timestamps
	}

	// Main post timestamp
	if ts := getTimestamp(post, "create_at"); ts > 0 {
		timestamps = append(timestamps, ts)
	}

	// Reply timestamps
	if replies, ok := post["replies"].([]any); ok {
		for _, replyIntf := range replies {
			if reply, ok := replyIntf.(map[string]any); ok {
				if ts := getTimestamp(reply, "create_at"); ts > 0 {
					timestamps = append(timestamps, ts)
				}
			}
		}
	}

	// Props timestamps (call posts)
	if props, ok := post["props"].(map[string]any); ok {
		if ts := getTimestamp(props, "start_at"); ts > 0 {
			timestamps = append(timestamps, ts)
		}
		if ts := getTimestamp(props, "end_at"); ts > 0 {
			timestamps = append(timestamps, ts)
		}
	}

	return timestamps
}

// getTimestamp safely extracts a timestamp from an object
func getTimestamp(obj map[string]any, field string) int64 {
	if timestamp, ok := obj[field].(float64); ok {
		return int64(timestamp)
	}
	return 0
}

// CreateZipFile creates a zip file containing the JSONL import file
func CreateZipFile(jsonlPath, zipPath string) error {
	zipFile, err := os.Create(zipPath)
	if err != nil {
		return fmt.Errorf("failed to create zip file: %w", err)
	}
	// Note: zipFile will be closed by zipWriter.Close(), no need for separate close

	zipWriter := zip.NewWriter(zipFile)
	defer closeWithLog(zipWriter, "zip writer")

	jsonlFile, err := os.Open(jsonlPath)
	if err != nil {
		return fmt.Errorf("failed to open JSONL file: %w", err)
	}
	defer closeWithLog(jsonlFile, "jsonl file")

	fileInfo, err := jsonlFile.Stat()
	if err != nil {
		return fmt.Errorf("failed to get file info: %w", err)
	}

	header, err := zip.FileInfoHeader(fileInfo)
	if err != nil {
		return fmt.Errorf("failed to create file header: %w", err)
	}
	// Use consistent filename in zip regardless of source filename
	header.Name = "import.jsonl"
	header.Method = zip.Deflate

	writer, err := zipWriter.CreateHeader(header)
	if err != nil {
		return fmt.Errorf("failed to create zip entry: %w", err)
	}

	_, err = io.Copy(writer, jsonlFile)
	return err
}

// ImportBulkData handles the complete bulk import process
func (c *Client) ImportBulkData(filePath string) error {
	zipPath := filePath + ".zip"

	// Clean up temp files when done
	defer func() {
		removeWithLog(zipPath)
		// Clean up temp file if it's a temp file (contains "/tmp/" or similar pattern)
		if strings.Contains(filePath, "import_") && strings.HasSuffix(filePath, ".jsonl") {
			removeWithLog(filePath)
		}
	}()

	// Create ZIP file
	if err := CreateZipFile(filePath, zipPath); err != nil {
		return fmt.Errorf("failed to create ZIP file: %w", err)
	}

	// Upload file using upload session
	importFileName, err := c.uploadImportFile(zipPath)
	if err != nil {
		return fmt.Errorf("failed to upload import file: %w", err)
	}

	// Start import job
	Log.WithFields(logrus.Fields{"import_file": importFileName}).Info("🚀 Creating import job for file")
	job, resp, err := c.API.CreateJob(context.Background(), &model.Job{
		Type: model.JobTypeImportProcess,
		Data: map[string]string{
			"import_file": importFileName,
		},
	})
	if err != nil {
		return handleAPIError("failed to create import job", err, resp)
	}
	Log.WithFields(logrus.Fields{"job_id": job.Id}).Info("✅ Import job created")

	// Wait for completion
	return c.waitForJobCompletion(job)
}

// uploadImportFile uploads a file using the upload session mechanism
func (c *Client) uploadImportFile(zipPath string) (string, error) {
	file, err := os.Open(zipPath)
	if err != nil {
		return "", fmt.Errorf("failed to open zip file: %w", err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			// Silently ignore "file already closed" errors
			if !strings.Contains(err.Error(), "file already closed") {
				Log.WithFields(logrus.Fields{"zip_path": zipPath, "error": err.Error()}).Warn("⚠️ Failed to close upload zip file")
			}
		}
	}()

	fileInfo, err := file.Stat()
	if err != nil {
		return "", fmt.Errorf("failed to get file info: %w", err)
	}

	// Get current user
	user, _, err := c.API.GetMe(context.Background(), "")
	if err != nil {
		return "", fmt.Errorf("failed to get current user: %w", err)
	}

	// Create upload session
	uploadSession, resp, err := c.API.CreateUpload(context.Background(), &model.UploadSession{
		Filename: fileInfo.Name(),
		FileSize: fileInfo.Size(),
		Type:     model.UploadTypeImport,
		UserId:   user.Id,
	})
	if err != nil {
		return "", handleAPIError("failed to create upload session: %w", err, resp)
	}

	// Upload the file data
	_, resp, err = c.API.UploadData(context.Background(), uploadSession.Id, file)
	if err != nil {
		return "", handleAPIError("failed to upload file data: %w", err, resp)
	}

	// The actual filename that Mattermost stores is sessionId_originalName
	actualFileName := uploadSession.Id + "_" + fileInfo.Name()
	Log.WithFields(logrus.Fields{
		"original_file": fileInfo.Name(),
		"actual_file":   actualFileName,
		"session_id":    uploadSession.Id,
	}).Info("📤 Uploaded import file")
	return actualFileName, nil
}

// waitForJobCompletion waits for a job to complete
func (c *Client) waitForJobCompletion(job *model.Job) error {
	for {
		currentJob, resp, err := c.API.GetJob(context.Background(), job.Id)
		if err != nil {
			return handleAPIError("failed to get job status", err, resp)
		}

		switch currentJob.Status {
		case model.JobStatusSuccess:
			return nil
		case model.JobStatusError:
			return fmt.Errorf("import job failed: %s", currentJob.Data["error"])
		case model.JobStatusCanceled:
			return fmt.Errorf("import job was canceled")
		case model.JobStatusPending, model.JobStatusInProgress:
			time.Sleep(2 * time.Second)
			continue
		default:
			return fmt.Errorf("unknown job status: %s", currentJob.Status)
		}
	}
}

// findBulkImportPath finds the bulk import file in common locations
func findBulkImportPath() (string, error) {
	paths := []string{"bulk_import.jsonl", "../bulk_import.jsonl"}
	for _, path := range paths {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("bulk_import.jsonl not found in current directory or parent directory")
}

// SetupWithSplitImport performs setup using two-phase bulk import
func (c *Client) SetupWithSplitImport() error {
	return c.SetupWithSplitImportAndForce(false, false)
}

// SetupWithSplitImportAndForce performs setup using two-phase bulk import with force options
func (c *Client) SetupWithSplitImportAndForce(forcePlugins, forceGitHubPlugins bool) error {
	// Use the client's BulkImportPath if set, otherwise find the default
	bulkImportPath := c.BulkImportPath
	if bulkImportPath == "" {
		path, err := findBulkImportPath()
		if err != nil {
			return err
		}
		bulkImportPath = path
	}

	Log.WithFields(logrus.Fields{
		"file": bulkImportPath,
	}).Info("🚀 Starting two-phase bulk import")

	if err := c.importInfrastructure(bulkImportPath); err != nil {
		return fmt.Errorf("failed to import infrastructure: %w", err)
	}

	if err := c.processPlugins(bulkImportPath, forcePlugins, forceGitHubPlugins); err != nil {
		return fmt.Errorf("failed to process plugins: %w", err)
	}

	if err := c.processChannelCategories(bulkImportPath); err != nil {
		return fmt.Errorf("failed to process channel categories: %w", err)
	}

	if err := c.processChannelBanners(bulkImportPath); err != nil {
		return fmt.Errorf("failed to process channel banners: %w", err)
	}

	if err := c.processCommands(bulkImportPath); err != nil {
		return fmt.Errorf("failed to process commands: %w", err)
	}

	if err := c.importUsers(bulkImportPath); err != nil {
		return fmt.Errorf("failed to import users: %w", err)
	}

	if err := c.processChannelMemberships(); err != nil {
		return fmt.Errorf("failed to process channel memberships: %w", err)
	}

	if err := c.processUserAttributes(bulkImportPath); err != nil {
		return fmt.Errorf("failed to process user attributes: %w", err)
	}

	if err := c.importPosts(bulkImportPath); err != nil {
		return fmt.Errorf("failed to import posts: %w", err)
	}

	return nil
}

// importInfrastructure imports teams and channels
func (c *Client) importInfrastructure(bulkImportPath string) error {
	Log.WithFields(logrus.Fields{"import_type": "infrastructure", "file_path": bulkImportPath}).Info("📋 Processing infrastructure import")
	return c.processLines(bulkImportPath, []string{"team", "channel"}, c.ImportBulkData)
}

// importUsers imports only users first
func (c *Client) importUsers(bulkImportPath string) error {
	Log.WithFields(logrus.Fields{"import_type": "users", "file_path": bulkImportPath}).Info("👥 Processing users import")
	return c.processLines(bulkImportPath, []string{"user"}, c.ImportBulkData)
}

// importPosts imports posts after users are created
func (c *Client) importPosts(bulkImportPath string) error {
	Log.WithFields(logrus.Fields{"import_type": "posts", "file_path": bulkImportPath}).Info("💬 Processing posts import")
	return c.processLines(bulkImportPath, []string{"post"}, c.ImportBulkData)
}

// processLines processes specific line types from bulk import file
func (c *Client) processLines(bulkImportPath string, lineTypes []string, processor func(string) error) error {
	// Store the current import path globally for timestamp processing
	globalCurrentImportPath = bulkImportPath
	
	file, err := os.Open(bulkImportPath)
	if err != nil {
		return fmt.Errorf("failed to open bulk import file: %w", err)
	}
	defer closeWithLog(file, "bulk import file")

	tempFile, err := os.CreateTemp("", "import_*.jsonl")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	// Note: temp file cleanup is handled by ImportBulkData after job completion

	// Write version line
	if _, err := tempFile.WriteString("{\"type\": \"version\", \"version\": 1}\n"); err != nil {
		return fmt.Errorf("failed to write version line: %w", err)
	}

	// Define custom types that should be skipped during bulk import
	customTypes := map[string]bool{
		"channel-category": true,
		"channel-banner":   true,
		"command":          true,
		"plugin":           true,
		"user-attribute":   true,
		"user-profile":     true,
	}

	scanner := bufio.NewScanner(file)
	count := 0
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		// First, extract just the type field to check if it's a custom type
		var typeCheck struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal([]byte(line), &typeCheck); err != nil {
			// If we can't even parse the type, skip with warning
			Log.WithFields(logrus.Fields{"line": line}).Warn("⚠️ Failed to parse type from line, skipping")
			continue
		}

		// Skip custom types silently (no warning)
		if customTypes[typeCheck.Type] {
			continue
		}

		// For standard types, try to unmarshal as BulkImportLine
		var importLine BulkImportLine
		if err := json.Unmarshal([]byte(line), &importLine); err != nil {
			// Only warn for non-custom types that fail to parse
			Log.WithFields(logrus.Fields{"line": line}).Warn("⚠️ Failed to parse standard import line, skipping")
			continue
		}

		if slices.Contains(lineTypes, importLine.Type) {
			lineToWrite := line

			// Special handling for team entries - store team names
			if importLine.Type == "team" {
				var teamData map[string]any
				if err := json.Unmarshal([]byte(line), &teamData); err == nil {
					if teamName := getNestedString(teamData, "team", "name"); teamName != "" {
						if !slices.Contains(globalImportedTeams, teamName) {
							globalImportedTeams = append(globalImportedTeams, teamName)
							Log.WithFields(logrus.Fields{
								"team_name": teamName,
							}).Debug("📋 Stored team name for channel membership processing")
						}
					}
				}
			}

			// Special handling for user entries - extract channel memberships
			if importLine.Type == "user" {
				cleanedLine, err := extractChannelMemberships(line)
				if err != nil {
					Log.WithFields(logrus.Fields{
						"username":      "unknown",
						"error":         err.Error(),
						"original_line": line,
					}).Warn("⚠️ Failed to extract channel memberships from user, using original line")
				} else {
					lineToWrite = cleanedLine
					Log.WithFields(logrus.Fields{
						"total_users_stored": len(globalChannelMemberships),
						"original_line":      line,
						"cleaned_line":       cleanedLine,
					}).Debug("📋 Extracted channel memberships from user")
				}
			}

			// Special handling for posts - adjust timestamps to be recent
			if importLine.Type == "post" {
				adjustedLine, err := adjustPostTimestamps(line)
				if err != nil {
					Log.WithFields(logrus.Fields{
						"error": err.Error(),
					}).Warn("⚠️ Failed to adjust post timestamps, using original")
				} else {
					lineToWrite = adjustedLine
				}
			}

			if _, err := tempFile.WriteString(lineToWrite + "\n"); err != nil {
				return fmt.Errorf("failed to write line: %w", err)
			}
			count++
		}
	}

	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("failed to close temp file: %w", err)
	}

	if count == 0 {
		Log.WithFields(logrus.Fields{"line_types": lineTypes}).Info("ℹ️ No items found for import")
		return nil
	}

	Log.WithFields(logrus.Fields{"line_types": lineTypes, "count": count}).Info("📤 Processing items for bulk import")
	return processor(tempFile.Name())
}

// processChannelMemberships joins users to channels via API to trigger hooks
func (c *Client) processChannelMemberships() error {
	if len(globalChannelMemberships) == 0 {
		Log.Info("ℹ️ No channel memberships to process")
		return nil
	}

	if len(globalImportedTeams) == 0 {
		Log.Warn("⚠️ No teams found during import, skipping channel memberships")
		return nil
	}

	Log.WithFields(logrus.Fields{
		"total_users": len(globalChannelMemberships),
		"teams":       globalImportedTeams,
	}).Info("👥 Processing channel memberships via API")

	joinedCount := 0
	errorCount := 0

	// Get all imported teams
	teams := make(map[string]*model.Team)
	for _, teamName := range globalImportedTeams {
		team, _, err := c.API.GetTeamByName(context.Background(), teamName, "")
		if err != nil {
			Log.WithFields(logrus.Fields{
				"team_name": teamName,
				"error":     err.Error(),
			}).Warn("⚠️ Failed to find team for channel membership")
			continue
		}
		teams[teamName] = team
	}

	if len(teams) == 0 {
		return fmt.Errorf("no teams found for channel membership processing")
	}

	// Process each user's channels
	for username, channels := range globalChannelMemberships {
		// Get user by username
		user, _, err := c.API.GetUserByUsername(context.Background(), username, "")
		if err != nil {
			Log.WithFields(logrus.Fields{
				"username": username,
				"error":    err.Error(),
			}).Warn("⚠️ Failed to find user for channel membership")
			errorCount++
			continue
		}

		// Join user to ALL channels (no filtering - let API handle duplicates)
		for _, channelName := range channels {
			channelFound := false
			
			// Try to find the channel in any of the teams
			for teamName, team := range teams {
				channel, _, err := c.API.GetChannelByName(context.Background(), channelName, team.Id, "")
				if err != nil {
					continue // Try next team
				}

				channelFound = true
				
				// Add user to channel via API (triggers hooks)
				_, _, err = c.API.AddChannelMember(context.Background(), channel.Id, user.Id)
				if err != nil {
					// Check if user is already a member (not an error)
					if strings.Contains(err.Error(), "already") || strings.Contains(err.Error(), "member") {
						Log.WithFields(logrus.Fields{
							"username":     username,
							"channel_name": channelName,
							"team":         teamName,
						}).Debug("👤 User already member of channel")
					} else {
						Log.WithFields(logrus.Fields{
							"username":     username,
							"channel_name": channelName,
							"team":         teamName,
							"error":        err.Error(),
						}).Warn("⚠️ Failed to add user to channel")
						errorCount++
						continue
					}
				} else {
					joinedCount++
					Log.WithFields(logrus.Fields{
						"username":     username,
						"channel_name": channelName,
						"team":         teamName,
					}).Debug("✅ Added user to channel via API")
				}
				
				break // Channel found and processed, no need to check other teams
			}
			
			if !channelFound {
				Log.WithFields(logrus.Fields{
					"channel_name": channelName,
					"username":     username,
					"teams":        globalImportedTeams,
				}).Warn("⚠️ Failed to find channel in any team")
				errorCount++
			}
		}
	}

	Log.WithFields(logrus.Fields{
		"joined_count": joinedCount,
		"error_count":  errorCount,
	}).Info("✅ Channel membership processing complete")

	// Clear global data after processing
	globalChannelMemberships = make(map[string][]string)
	globalImportedTeams = make([]string, 0)

	return nil
}