package mattermost

import (
	"archive/zip"
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
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
		"actual_file": actualFileName,
		"session_id": uploadSession.Id,
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
	bulkImportPath, err := findBulkImportPath()
	if err != nil {
		return err
	}

	Log.Info("🚀 Starting two-phase bulk import")

	if err := c.importInfrastructure(bulkImportPath); err != nil {
		return fmt.Errorf("failed to import infrastructure: %w", err)
	}

	if err := c.processChannelCategories(bulkImportPath); err != nil {
		return fmt.Errorf("failed to process channel categories: %w", err)
	}

	if err := c.importUsers(bulkImportPath); err != nil {
		return fmt.Errorf("failed to import users: %w", err)
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
						"team_name": categoryImport.Team,
						"category": categoryImport.Category,
						"error": err.Error(),
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
			"error_count": errorCount,
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
					"team_name": commandImport.Command.Team,
					"channel_name": commandImport.Command.Channel,
					"command_text": commandImport.Command.Text,
					"error": err.Error(),
				}).Warn("⚠️ Failed to execute command")
			}
		}
	}

	return scanner.Err()
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
			return c.categorizeChannelAPI(channel.Id, categoryName)
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
