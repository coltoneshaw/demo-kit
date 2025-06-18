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


// UserAttributeImport represents a single user attribute definition import entry
type UserAttributeImport struct {
	Type      string `json:"type"`
	Attribute UserAttributeField `json:"attribute"`
}

// UserAttributeField represents a custom profile field definition
type UserAttributeField struct {
	Name          string   `json:"name"`
	DisplayName   string   `json:"display_name"`
	Type          string   `json:"type"`
	HideWhenEmpty bool     `json:"hide_when_empty,omitempty"`
	Required      bool     `json:"required,omitempty"`
	// Extended configuration fields
	LDAPAttribute string   `json:"ldap,omitempty"`        // LDAP attribute mapping
	SAMLAttribute string   `json:"saml,omitempty"`        // SAML attribute mapping  
	Options       []string `json:"options,omitempty"`     // Options for select fields
	SortOrder     int      `json:"sort_order,omitempty"`  // Display order
	ValueType     string   `json:"value_type,omitempty"`  // Value type constraint
	Visibility    string   `json:"visibility,omitempty"`  // Visibility setting
}

// UserProfileImport represents a user profile assignment entry
type UserProfileImport struct {
	Type       string            `json:"type"`
	User       string            `json:"user"`       // Username
	Attributes map[string]string `json:"attributes"` // Map of attribute name to value
}

// GroupConfig represents group configuration from import data
type GroupConfig struct {
	Name           string   `json:"name"`             // Group name
	ID             string   `json:"id"`               // Unique group identifier
	Members        []string `json:"members"`          // Array of usernames
	AllowReference bool     `json:"allow_reference"`  // Whether the group can be referenced (@mentions)
}

// UserGroupImport represents a user group import entry
type UserGroupImport struct {
	Type  string      `json:"type"`
	Group GroupConfig `json:"group"`
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

	if err := c.processPlugins(bulkImportPath, forcePlugins, forceGitHubPlugins); err != nil {
		return fmt.Errorf("failed to process plugins: %w", err)
	}

	if err := c.importInfrastructure(bulkImportPath); err != nil {
		return fmt.Errorf("failed to import infrastructure: %w", err)
	}

	if err := c.processChannelCategories(bulkImportPath); err != nil {
		return fmt.Errorf("failed to process channel categories: %w", err)
	}

	if err := c.importUsers(bulkImportPath); err != nil {
		return fmt.Errorf("failed to import users: %w", err)
	}

	if err := c.processUserAttributes(bulkImportPath); err != nil {
		return fmt.Errorf("failed to process user attributes: %w", err)
	}


	if err := c.processCommands(bulkImportPath); err != nil {
		return fmt.Errorf("failed to process commands: %w", err)
	}

	return nil
}

// importInfrastructure imports teams and channels
func (c *Client) importInfrastructure(bulkImportPath string) error {
	Log.WithFields(logrus.Fields{"import_type": "infrastructure", "file_path": bulkImportPath}).Info("📋 Processing infrastructure import")
	return c.processLines(bulkImportPath, []string{"team", "channel"}, c.ImportBulkData)
}

func (c *Client) importUsers(bulkImportPath string) error {
	Log.WithFields(logrus.Fields{"import_type": "users", "file_path": bulkImportPath}).Info("📋 Processing users import")
	return c.processLines(bulkImportPath, []string{"user"}, c.ImportBulkData)
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

		for _, lineType := range lineTypes {
			if importLine.Type == lineType {
				if _, err := tempFile.WriteString(line + "\n"); err != nil {
					return fmt.Errorf("failed to write line: %w", err)
				}
				count++
				break
			}
		}
	}

	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("failed to close temp file: %w", err)
	}

	if count == 0 {
		return nil
	}

	return processor(tempFile.Name())
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


