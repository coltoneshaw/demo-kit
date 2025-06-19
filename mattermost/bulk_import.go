package mattermost

import (
	"archive/zip"
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/sirupsen/logrus"
)

// BulkImportLine represents a single line in the bulk import JSONL file
type BulkImportLine struct {
	Type    string          `json:"type"`
	Version int             `json:"version,omitempty"`
	Raw     json.RawMessage `json:"-"` // Store original JSON for writing
}

// ChannelCategoryImport represents a channel category import entry
type ChannelCategoryImport struct {
	Type     string   `json:"type"`
	Category string   `json:"category"`
	Team     string   `json:"team"`
	Channels []string `json:"channels"`
}

// CommandImport represents a command import entry
type CommandImport struct {
	Type    string `json:"type"`
	Command struct {
		Team    string `json:"team"`
		Channel string `json:"channel"`
		Text    string `json:"text"`
	} `json:"command"`
}

// ChannelBannerImport represents a channel banner import entry
type ChannelBannerImport struct {
	Type   string `json:"type"`
	Banner struct {
		Team            string `json:"team"`
		Channel         string `json:"channel"`
		Text            string `json:"text"`
		BackgroundColor string `json:"background_color"`
		Enabled         bool   `json:"enabled"`
	} `json:"banner"`
}

// PluginImport represents a plugin import entry
type PluginImport struct {
	Type   string `json:"type"`
	Plugin struct {
		Source       string `json:"source"`        // "github" or "local"
		GithubRepo   string `json:"github_repo"`   // For GitHub plugins: "owner/repo"
		Path         string `json:"path"`          // For local plugins: "../apps/plugin-name"
		PluginID     string `json:"plugin_id"`     // Plugin ID
		Name         string `json:"name"`          // Human readable name
		ForceInstall bool   `json:"force_install"` // Whether to force reinstall
	} `json:"plugin"`
}

// UserRank represents military rank information
type UserRank struct {
	Username string
	Rank     string
	Level    int // For comparison (higher = senior)
	Unit     string
}

// ChannelContext represents conversation context for a channel
type ChannelContext struct {
	Name        string
	Topics      []string
	Formality   string // "formal", "operational", "informal"
	MessageType string // "briefing", "status", "coordination", "intel"
}

// UserAttributeImport represents a single user attribute definition import entry
type UserAttributeImport struct {
	Type      string             `json:"type"`
	Attribute UserAttributeField `json:"attribute"`
}

// UserAttributeField represents a custom profile field definition
type UserAttributeField struct {
	Name          string `json:"name"`
	DisplayName   string `json:"display_name"`
	Type          string `json:"type"`
	HideWhenEmpty bool   `json:"hide_when_empty,omitempty"`
	Required      bool   `json:"required,omitempty"`
	// Extended configuration fields
	LDAPAttribute string   `json:"ldap,omitempty"`       // LDAP attribute mapping
	SAMLAttribute string   `json:"saml,omitempty"`       // SAML attribute mapping
	Options       []string `json:"options,omitempty"`    // Options for select fields
	SortOrder     int      `json:"sort_order,omitempty"` // Display order
	ValueType     string   `json:"value_type,omitempty"` // Value type constraint
	Visibility    string   `json:"visibility,omitempty"` // Visibility setting
}

// UserProfileImport represents a user profile assignment entry
type UserProfileImport struct {
	Type       string            `json:"type"`
	User       string            `json:"user"`       // Username
	Attributes map[string]string `json:"attributes"` // Map of attribute name to value
}

// GroupConfig represents group configuration from import data
type GroupConfig struct {
	Name           string   `json:"name"`            // Group name
	ID             string   `json:"id"`              // Unique group identifier
	Members        []string `json:"members"`         // Array of usernames
	AllowReference bool     `json:"allow_reference"` // Whether the group can be referenced (@mentions)
}

// UserGroupImport represents a user group import entry
type UserGroupImport struct {
	Type  string      `json:"type"`
	Group GroupConfig `json:"group"`
}

// Global storage for channel memberships during import processing  
// Simple map: username -> list of channel names
var globalChannelMemberships = make(map[string][]string)

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

// Global variables for timestamp adjustment
var (
	timestampOffset int64 = 0
	offsetCalculated bool = false
)

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
	bulkImportPath, err := findBulkImportPath()
	if err != nil {
		return 0, err
	}
	
	file, err := os.Open(bulkImportPath)
	if err != nil {
		return 0, err
	}
	defer file.Close()
	
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

func closeWithLog(c io.Closer, label string) {
	if err := c.Close(); err != nil {
		Log.WithFields(logrus.Fields{"label": label, "error": err.Error()}).Warn("⚠️ Failed to close resource")
	}
}

func removeWithLog(path string) {
	if err := os.Remove(path); err != nil {
		Log.WithFields(logrus.Fields{"file_path": path, "error": err.Error()}).Warn("⚠️ Failed to remove file")
	}
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
	bulkImportPath, err := findBulkImportPath()
	if err != nil {
		return err
	}

	Log.Info("🚀 Starting two-phase bulk import")

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
			
			// Special handling for user entries - extract channel memberships
			if importLine.Type == "user" {
				cleanedLine, err := extractChannelMemberships(line)
				if err != nil {
					Log.WithFields(logrus.Fields{
						"username": "unknown",
						"error":    err.Error(),
						"original_line": line,
					}).Warn("⚠️ Failed to extract channel memberships from user, using original line")
				} else {
					lineToWrite = cleanedLine
					Log.WithFields(logrus.Fields{
						"total_users_stored": len(globalChannelMemberships),
						"original_line": line,
						"cleaned_line": cleanedLine,
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
	
	Log.WithFields(logrus.Fields{
		"total_users": len(globalChannelMemberships),
	}).Info("👥 Processing channel memberships via API")
	
	joinedCount := 0
	errorCount := 0
	
	// Get the team (assuming single team for simplicity)
	team, _, err := c.API.GetTeamByName(context.Background(), "usaf-team", "")
	if err != nil {
		return fmt.Errorf("failed to find team: %w", err)
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
			channel, _, err := c.API.GetChannelByName(context.Background(), channelName, team.Id, "")
			if err != nil {
				Log.WithFields(logrus.Fields{
					"channel_name": channelName,
					"username":     username,
					"error":        err.Error(),
				}).Warn("⚠️ Failed to find channel for membership")
				errorCount++
				continue
			}
			
			// Add user to channel via API (triggers hooks)
			_, _, err = c.API.AddChannelMember(context.Background(), channel.Id, user.Id)
			if err != nil {
				// Check if user is already a member (not an error)
				if strings.Contains(err.Error(), "already") || strings.Contains(err.Error(), "member") {
					Log.WithFields(logrus.Fields{
						"username":     username,
						"channel_name": channelName,
					}).Debug("👤 User already member of channel")
				} else {
					Log.WithFields(logrus.Fields{
						"username":     username,
						"channel_name": channelName,
						"error":        err.Error(),
					}).Warn("⚠️ Failed to add user to channel")
					errorCount++
					continue
				}
			}
			
			joinedCount++
			Log.WithFields(logrus.Fields{
				"username":     username,
				"channel_name": channelName,
			}).Debug("✅ Added user to channel via API")
		}
	}
	
	Log.WithFields(logrus.Fields{
		"joined_count": joinedCount,
		"error_count":  errorCount,
	}).Info("✅ Channel membership processing complete")
	
	// Clear global memberships after processing
	globalChannelMemberships = make(map[string][]string)
	
	return nil
}

// processChannelCategories processes channel categories
func (c *Client) processChannelCategories(bulkImportPath string) error {
	Log.WithFields(logrus.Fields{"file_path": bulkImportPath}).Info("📋 Processing channel categories")

	file, err := os.Open(bulkImportPath)
	if err != nil {
		return err
	}
	defer closeWithLog(file, "bulk import file")

	categorizedCount := 0
	errorCount := 0

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var categoryImport ChannelCategoryImport
		if err := json.Unmarshal([]byte(line), &categoryImport); err != nil {
			continue
		}

		if categoryImport.Type == "channel-category" {
			for _, channelName := range categoryImport.Channels {
				if err := c.categorizeChannel(categoryImport.Team, channelName, categoryImport.Category); err != nil {
					if err.Error() == "ALREADY_CATEGORIZED" {
						// Don't count as error or update - just silently skip
						continue
					}
					Log.WithFields(logrus.Fields{
						"channel_name": channelName,
						"team_name":    categoryImport.Team,
						"category":     categoryImport.Category,
						"error":        err.Error(),
					}).Warn("⚠️ Failed to categorize channel")
					errorCount++
				} else {
					categorizedCount++
				}
			}
		}
	}

	if categorizedCount > 0 || errorCount > 0 {
		Log.WithFields(logrus.Fields{
			"categorized_count": categorizedCount,
			"error_count":       errorCount,
		}).Info("✅ Channel categorization complete")
	} else {
		Log.Info("✅ All channels already properly categorized")
	}

	return scanner.Err()
}

// processChannelBanners processes channel banners
func (c *Client) processChannelBanners(bulkImportPath string) error {
	Log.WithFields(logrus.Fields{"file_path": bulkImportPath}).Info("🎯 Processing channel banners")

	file, err := os.Open(bulkImportPath)
	if err != nil {
		return err
	}
	defer closeWithLog(file, "bulk import file")

	bannersCount := 0
	errorCount := 0

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var bannerImport ChannelBannerImport
		if err := json.Unmarshal([]byte(line), &bannerImport); err != nil {
			continue
		}

		if bannerImport.Type == "channel-banner" {
			if err := c.setChannelBanner(bannerImport.Banner.Team, bannerImport.Banner.Channel, bannerImport.Banner.Text, bannerImport.Banner.BackgroundColor, bannerImport.Banner.Enabled); err != nil {
				Log.WithFields(logrus.Fields{
					"channel_name":     bannerImport.Banner.Channel,
					"team_name":        bannerImport.Banner.Team,
					"banner_text":      bannerImport.Banner.Text,
					"background_color": bannerImport.Banner.BackgroundColor,
					"error":            err.Error(),
				}).Warn("⚠️ Failed to set channel banner")
				errorCount++
			} else {
				bannersCount++
			}
		}
	}

	if bannersCount > 0 || errorCount > 0 {
		Log.WithFields(logrus.Fields{
			"banners_count": bannersCount,
			"error_count":   errorCount,
		}).Info("✅ Channel banner setup complete")
	} else {
		Log.Info("✅ No channel banners to process")
	}

	return scanner.Err()
}

// processCommands processes commands
func (c *Client) processCommands(bulkImportPath string) error {
	Log.WithFields(logrus.Fields{"file_path": bulkImportPath}).Info("📋 Processing commands")
	file, err := os.Open(bulkImportPath)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			Log.WithFields(logrus.Fields{"file_path": bulkImportPath, "error": closeErr.Error()}).Warn("⚠️ Failed to close file")
		}
	}()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var commandImport CommandImport
		if err := json.Unmarshal([]byte(line), &commandImport); err != nil {
			continue
		}

		if commandImport.Type == "command" {
			if err := c.executeCommand(commandImport.Command.Team, commandImport.Command.Channel, commandImport.Command.Text); err != nil {
				Log.WithFields(logrus.Fields{
					"team_name":    commandImport.Command.Team,
					"channel_name": commandImport.Command.Channel,
					"command_text": commandImport.Command.Text,
					"error":        err.Error(),
				}).Warn("⚠️ Failed to execute command")
			}
		}
	}

	return scanner.Err()
}

// processGitHubPlugin downloads and installs a GitHub plugin
func (c *Client) processGitHubPlugin(pluginImport PluginImport) error {
	Log.WithFields(logrus.Fields{
		"plugin_name":   pluginImport.Plugin.Name,
		"github_repo":   pluginImport.Plugin.GithubRepo,
		"plugin_id":     pluginImport.Plugin.PluginID,
		"force_install": pluginImport.Plugin.ForceInstall,
	}).Info("📦 Processing plugin " + pluginImport.Plugin.Name)

	// Check if already installed unless forced
	pm := NewPluginManager(c)
	if !pluginImport.Plugin.ForceInstall && pm.isInstalledByID(pluginImport.Plugin.PluginID) {
		Log.WithFields(logrus.Fields{
			"plugin_name": pluginImport.Plugin.Name,
			"plugin_id":   pluginImport.Plugin.PluginID,
		}).Info("⏭️ Skipping " + pluginImport.Plugin.Name + ": already installed")
		return nil
	}

	// Create PluginConfig for compatibility with existing plugin manager
	pluginConfig := PluginConfig{
		Name:     pluginImport.Plugin.Name,
		Repo:     pluginImport.Plugin.GithubRepo,
		PluginID: pluginImport.Plugin.PluginID,
	}

	// Download the plugin
	Log.WithFields(logrus.Fields{
		"plugin_name": pluginImport.Plugin.Name,
		"github_repo": pluginImport.Plugin.GithubRepo,
		"plugin_id":   pluginImport.Plugin.PluginID,
	}).Info("📥 Downloading plugin from GitHub...")

	if err := pm.downloadPlugin(pluginConfig); err != nil {
		return fmt.Errorf("failed to download GitHub plugin: %w", err)
	}

	// Install from plugins directory
	pluginsDir := "../files/mattermost/plugins"
	if _, err := os.Stat("files/mattermost/plugins"); err == nil {
		pluginsDir = "files/mattermost/plugins"
	}

	// Find the downloaded .tar.gz file
	files, err := os.ReadDir(pluginsDir)
	if err != nil {
		return fmt.Errorf("failed to read plugins directory: %w", err)
	}

	for _, file := range files {
		if strings.HasSuffix(file.Name(), ".tar.gz") && strings.Contains(file.Name(), pluginImport.Plugin.PluginID) {
			pluginPath := filepath.Join(pluginsDir, file.Name())
			if err := pm.uploadPlugin(pluginPath); err != nil {
				return fmt.Errorf("failed to install GitHub plugin: %w", err)
			}
			Log.WithFields(logrus.Fields{
				"plugin_name": pluginImport.Plugin.Name,
				"plugin_id":   pluginImport.Plugin.PluginID,
			}).Info("✅ Successfully installed " + pluginImport.Plugin.Name)
			return nil
		}
	}

	return fmt.Errorf("downloaded plugin file not found for %s", pluginImport.Plugin.Name)
}

// processLocalPlugin builds and installs a local plugin
func (c *Client) processLocalPlugin(pluginImport PluginImport) error {
	Log.WithFields(logrus.Fields{
		"plugin_name":   pluginImport.Plugin.Name,
		"plugin_path":   pluginImport.Plugin.Path,
		"plugin_id":     pluginImport.Plugin.PluginID,
		"force_install": pluginImport.Plugin.ForceInstall,
	}).Info("📦 Processing plugin " + pluginImport.Plugin.Name)

	// Check if already installed unless forced
	pm := NewPluginManager(c)
	if !pluginImport.Plugin.ForceInstall && pm.isInstalledByID(pluginImport.Plugin.PluginID) {
		Log.WithFields(logrus.Fields{
			"plugin_name": pluginImport.Plugin.Name,
			"plugin_id":   pluginImport.Plugin.PluginID,
		}).Info("⏭️ Skipping " + pluginImport.Plugin.Name + ": already installed")
		return nil
	}

	// Check if plugin directory exists
	if _, err := os.Stat(pluginImport.Plugin.Path); os.IsNotExist(err) {
		return fmt.Errorf("plugin directory not found: %s", pluginImport.Plugin.Path)
	}

	// Clean if forced install
	if pluginImport.Plugin.ForceInstall {
		Log.WithFields(logrus.Fields{
			"plugin_path": pluginImport.Plugin.Path,
		}).Debug("🧹 Cleaning plugin before rebuild")
		if err := pm.cleanPlugin(pluginImport.Plugin.Path); err != nil {
			Log.WithFields(logrus.Fields{
				"plugin_path": pluginImport.Plugin.Path,
				"error":       err.Error(),
			}).Warn("⚠️ Warning: Failed to clean plugin")
		}
	}

	// Build the plugin
	Log.WithFields(logrus.Fields{
		"plugin_path": pluginImport.Plugin.Path,
	}).Info("🔨 Building plugin...")

	if err := pm.buildPlugin(pluginImport.Plugin.Path); err != nil {
		return fmt.Errorf("failed to build local plugin: %w", err)
	}

	Log.Info("✅ Plugin build completed, looking for built file...")

	// Find the built .tar.gz file in the plugin's dist directory
	distDir := filepath.Join(pluginImport.Plugin.Path, "dist")

	Log.WithFields(logrus.Fields{
		"dist_dir": distDir,
	}).Debug("🔍 Checking plugin dist directory...")

	files, err := os.ReadDir(distDir)
	if err != nil {
		return fmt.Errorf("failed to read plugin dist directory '%s': %w", distDir, err)
	}

	Log.WithFields(logrus.Fields{
		"dist_dir":   distDir,
		"file_count": len(files),
	}).Debug("🔍 Searching for built plugin file in dist directory...")

	// List all .tar.gz files for debugging
	var tarFiles []string
	for _, file := range files {
		if strings.HasSuffix(file.Name(), ".tar.gz") {
			tarFiles = append(tarFiles, file.Name())
		}
	}

	Log.WithFields(logrus.Fields{
		"tar_gz_files": tarFiles,
	}).Debug("📁 Found .tar.gz files in dist directory")

	for _, file := range files {
		if strings.HasSuffix(file.Name(), ".tar.gz") {
			pluginPath := filepath.Join(distDir, file.Name())
			Log.WithFields(logrus.Fields{
				"found_file": file.Name(),
				"full_path":  pluginPath,
			}).Info("📦 Found plugin file in dist directory, installing...")

			if err := pm.uploadPlugin(pluginPath); err != nil {
				return fmt.Errorf("failed to install local plugin: %w", err)
			}
			Log.WithFields(logrus.Fields{
				"plugin_name": pluginImport.Plugin.Name,
				"plugin_id":   pluginImport.Plugin.PluginID,
			}).Info("✅ Successfully installed " + pluginImport.Plugin.Name)
			return nil
		}
	}

	return fmt.Errorf("built plugin file not found for %s (no .tar.gz files found in dist directory '%s')", pluginImport.Plugin.Name, distDir)
}

// categorizeChannel categorizes a channel by name
func (c *Client) categorizeChannel(teamName, channelName, categoryName string) error {
	team, resp, err := c.API.GetTeamByName(context.Background(), teamName, "")
	if err != nil {
		return handleAPIError(fmt.Sprintf("failed to get team '%s'", teamName), err, resp)
	}

	channels, _, _ := c.API.GetPublicChannelsForTeam(context.Background(), team.Id, 0, 1000, "")
	privateChannels, _, _ := c.API.GetPrivateChannelsForTeam(context.Background(), team.Id, 0, 1000, "")
	channels = append(channels, privateChannels...)

	for _, channel := range channels {
		if channel.Name == channelName {
			return c.categorizeChannelAPI(channel.Id, channel.Name, categoryName)
		}
	}

	return fmt.Errorf("channel '%s' not found in team '%s'", channelName, teamName)
}

// setChannelBanner sets a banner for a channel by name
func (c *Client) setChannelBanner(teamName, channelName, text, backgroundColor string, enabled bool) error {
	team, resp, err := c.API.GetTeamByName(context.Background(), teamName, "")
	if err != nil {
		return handleAPIError(fmt.Sprintf("failed to get team '%s'", teamName), err, resp)
	}

	channels, _, _ := c.API.GetPublicChannelsForTeam(context.Background(), team.Id, 0, 1000, "")
	privateChannels, _, _ := c.API.GetPrivateChannelsForTeam(context.Background(), team.Id, 0, 1000, "")
	channels = append(channels, privateChannels...)

	for _, channel := range channels {
		if channel.Name == channelName {
			return c.setChannelBannerAPI(channel.Id, channel.Name, text, backgroundColor, enabled)
		}
	}

	return fmt.Errorf("channel '%s' not found in team '%s'", channelName, teamName)
}

// executeCommand executes a command in a channel
func (c *Client) executeCommand(teamName, channelName, commandText string) error {
	team, resp, err := c.API.GetTeamByName(context.Background(), teamName, "")
	if err != nil {
		return handleAPIError(fmt.Sprintf("failed to get team '%s'", teamName), err, resp)
	}

	channels, _, _ := c.API.GetPublicChannelsForTeam(context.Background(), team.Id, 0, 1000, "")
	privateChannels, _, _ := c.API.GetPrivateChannelsForTeam(context.Background(), team.Id, 0, 1000, "")
	channels = append(channels, privateChannels...)

	for _, channel := range channels {
		if channel.Name == channelName {
			_, resp, err := c.API.ExecuteCommand(context.Background(), channel.Id, commandText)
			return handleAPIError(fmt.Sprintf("failed to execute command '%s'", commandText), err, resp)
		}
	}

	return fmt.Errorf("channel '%s' not found in team '%s'", channelName, teamName)
}

// processPlugins processes plugin entries from bulk import file
func (c *Client) processPlugins(bulkImportPath string, forcePlugins, forceGitHubPlugins bool) error {
	Log.Info("📦 Processing plugins from JSONL")

	file, err := os.Open(bulkImportPath)
	if err != nil {
		return err
	}
	defer closeWithLog(file, "bulk import file")

	var plugins []PluginImport

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var pluginImport PluginImport
		if err := json.Unmarshal([]byte(line), &pluginImport); err != nil {
			continue
		}

		if pluginImport.Type == "plugin" {
			plugins = append(plugins, pluginImport)
		}
	}

	if len(plugins) == 0 {
		Log.Info("📦 No plugins found in JSONL")
		return nil
	}

	Log.WithFields(logrus.Fields{
		"plugin_count": len(plugins),
	}).Info("📦 Found plugins in JSONL")

	// Process plugins in order: GitHub first, then local
	for _, plugin := range plugins {
		if plugin.Plugin.Source == "github" {
			// Apply force flags: forceGitHubPlugins forces all plugins
			pluginCopy := plugin
			if forceGitHubPlugins {
				pluginCopy.Plugin.ForceInstall = true
			}

			if err := c.processGitHubPlugin(pluginCopy); err != nil {
				Log.WithFields(logrus.Fields{
					"plugin_name": plugin.Plugin.Name,
					"error":       err.Error(),
				}).Error("❌ Failed to process GitHub plugin")
				return fmt.Errorf("failed to process GitHub plugin '%s': %w", plugin.Plugin.Name, err)
			}
		}
	}

	for _, plugin := range plugins {
		if plugin.Plugin.Source == "local" {
			// Apply force flags: forceGitHubPlugins forces all plugins, forcePlugins forces local plugins
			pluginCopy := plugin
			if forceGitHubPlugins || forcePlugins {
				pluginCopy.Plugin.ForceInstall = true
			}

			if err := c.processLocalPlugin(pluginCopy); err != nil {
				Log.WithFields(logrus.Fields{
					"plugin_name": plugin.Plugin.Name,
					"error":       err.Error(),
				}).Error("❌ Failed to process local plugin")
				return fmt.Errorf("failed to process local plugin '%s': %w", plugin.Plugin.Name, err)
			}
		}
	}

	return scanner.Err()
}

// processUserAttributes processes user attribute definitions from JSONL
func (c *Client) processUserAttributes(bulkImportPath string) error {
	Log.WithFields(logrus.Fields{"file_path": bulkImportPath}).Info("📋 Processing user attributes")

	file, err := os.Open(bulkImportPath)
	if err != nil {
		return err
	}
	defer closeWithLog(file, "bulk import file")

	var attributeFields []UserAttributeField
	createdCount := 0
	errorCount := 0

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var attributeImport UserAttributeImport
		if err := json.Unmarshal([]byte(line), &attributeImport); err != nil {
			continue
		}

		if attributeImport.Type == "user-attribute" {
			attributeFields = append(attributeFields, attributeImport.Attribute)
		}
	}

	if len(attributeFields) == 0 {
		Log.Info("📋 No user attributes found, skipping")
		return nil
	}

	Log.WithFields(logrus.Fields{
		"field_count": len(attributeFields),
	}).Info("📋 Found user attributes")

	// Ensure all custom fields exist
	for _, field := range attributeFields {
		if err := c.ensureCustomFieldExists(field); err != nil {
			Log.WithFields(logrus.Fields{
				"field_name": field.Name,
				"error":      err.Error(),
			}).Warn("⚠️ Failed to ensure custom field exists")
			errorCount++
		} else {
			createdCount++
		}
	}

	Log.WithFields(logrus.Fields{
		"created_count": createdCount,
		"error_count":   errorCount,
	}).Info("✅ User attributes processing complete")

	return scanner.Err()
}

// ensureCustomFieldExists creates a custom field if it doesn't exist
func (c *Client) ensureCustomFieldExists(field UserAttributeField) error {
	// Get existing fields
	existingFields, err := c.ListCustomProfileFields()
	if err != nil {
		return fmt.Errorf("failed to list existing custom fields: %w", err)
	}

	// Check if field already exists
	for _, existingField := range existingFields {
		if existingField.Name == field.Name {
			Log.WithFields(logrus.Fields{
				"field_name": field.Name,
			}).Debug("🔍 Custom field already exists")
			return nil
		}
	}

	// Create the field with extended configuration
	Log.WithFields(logrus.Fields{
		"field_name":     field.Name,
		"display_name":   field.DisplayName,
		"field_type":     field.Type,
		"ldap_attribute": field.LDAPAttribute,
		"saml_attribute": field.SAMLAttribute,
		"options_count":  len(field.Options),
		"sort_order":     field.SortOrder,
		"value_type":     field.ValueType,
		"visibility":     field.Visibility,
	}).Info("📝 Creating custom profile field with extended configuration")

	_, err = c.CreateCustomProfileFieldExtended(field)
	if err != nil {
		return fmt.Errorf("failed to create custom field '%s': %w", field.Name, err)
	}

	Log.WithFields(logrus.Fields{
		"field_name": field.Name,
	}).Info("✅ Successfully created custom profile field")

	return nil
}
