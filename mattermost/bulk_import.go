package mattermost

import (
	"archive/zip"
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/mattermost/mattermost/server/public/model"
)



// CreateZipFile creates a zip file containing the JSONL import file
func CreateZipFile(jsonlPath, zipPath string) error {
	// Create the zip file
	zipFile, err := os.Create(zipPath)
	if err != nil {
		return fmt.Errorf("failed to create zip file: %w", err)
	}
	defer func() {
		if closeErr := zipFile.Close(); closeErr != nil {
			fmt.Printf("Warning: failed to close zip file: %v\n", closeErr)
		}
	}()

	// Create a new zip writer
	zipWriter := zip.NewWriter(zipFile)
	defer func() {
		if closeErr := zipWriter.Close(); closeErr != nil {
			fmt.Printf("Warning: failed to close zip writer: %v\n", closeErr)
		}
	}()

	// Open the JSONL file
	jsonlFile, err := os.Open(jsonlPath)
	if err != nil {
		return fmt.Errorf("failed to open JSONL file: %w", err)
	}
	defer func() {
		if closeErr := jsonlFile.Close(); closeErr != nil {
			fmt.Printf("Warning: failed to close JSONL file: %v\n", closeErr)
		}
	}()

	// Create a file in the zip archive
	jsonlInfo, err := jsonlFile.Stat()
	if err != nil {
		return fmt.Errorf("failed to get file info: %w", err)
	}

	writer, err := zipWriter.Create(jsonlInfo.Name())
	if err != nil {
		return fmt.Errorf("failed to create file in zip: %w", err)
	}

	// Copy the JSONL content to the zip
	_, err = io.Copy(writer, jsonlFile)
	if err != nil {
		return fmt.Errorf("failed to copy file to zip: %w", err)
	}

	return nil
}

// UploadImportFile uploads a zip file for import using the Mattermost API
func (c *Client) UploadImportFile(zipPath string) (string, error) {
	fmt.Printf("Uploading import file: %s\n", zipPath)

	// Open the zip file
	file, err := os.Open(zipPath)
	if err != nil {
		return "", fmt.Errorf("failed to open zip file: %w", err)
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			fmt.Printf("Warning: failed to close file: %v\n", closeErr)
		}
	}()

	// Get file info
	fileInfo, err := file.Stat()
	if err != nil {
		return "", fmt.Errorf("failed to get file info: %w", err)
	}

	// Get current user ID
	user, resp, err := c.API.GetMe(context.Background(), "")
	if err != nil {
		return "", fmt.Errorf("failed to get current user: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to get current user, status: %d", resp.StatusCode)
	}

	// Create upload session
	uploadSession, _, err := c.API.CreateUpload(context.Background(), &model.UploadSession{
		Filename: fileInfo.Name(),
		FileSize: fileInfo.Size(),
		Type:     model.UploadTypeImport,
		UserId:   user.Id,
	})
	if err != nil {
		return "", fmt.Errorf("failed to create upload session: %w", err)
	}

	// Upload the file data
	fileInfo2, _, err := c.API.UploadData(context.Background(), uploadSession.Id, file)
	if err != nil {
		return "", fmt.Errorf("failed to upload data: %w", err)
	}

	// The import process expects the format: {upload_session_id}_{original_filename}
	importFileName := fmt.Sprintf("%s_%s", uploadSession.Id, fileInfo2.Name)
	fmt.Printf("✅ Import file uploaded: %s (%d bytes)\n", importFileName, fileInfo2.Size)
	
	return importFileName, nil
}

// ProcessImport starts the import process for an uploaded file using CreateJob
func (c *Client) ProcessImport(importFileName string) (*model.Job, error) {
	fmt.Printf("Starting import process for: %s\n", importFileName)

	// Create the import process job
	job, _, err := c.API.CreateJob(context.Background(), &model.Job{
		Type: model.JobTypeImportProcess,
		Data: map[string]string{
			"import_file": importFileName,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create import process job: %w", err)
	}
	
	fmt.Printf("✅ Import job created: %s\n", job.Id)
	return job, nil
}

// VerifyImportFile checks if the uploaded file is available for import using ListImports API
func (c *Client) VerifyImportFile(expectedFileName string) error {
	// Get list of available import files
	importFiles, _, err := c.API.ListImports(context.Background())
	if err != nil {
		return fmt.Errorf("failed to list import files: %w", err)
	}
	
	// Check if our file is in the list
	for _, fileName := range importFiles {
		if fileName == expectedFileName {
			fmt.Printf("✅ Verified import file: %s\n", expectedFileName)
			return nil
		}
	}
	
	return fmt.Errorf("uploaded file '%s' not found in available import files", expectedFileName)
}

// WaitForJobCompletion waits for a job to complete
func (c *Client) WaitForJobCompletion(job *model.Job) error {
	fmt.Printf("Waiting for job completion: %s\n", job.Id)

	maxAttempts := 60 // Wait up to 10 minutes (60 * 10s)
	
	for attempt := range maxAttempts {
		// Get job status
		currentJob, _, err := c.API.GetJob(context.Background(), job.Id)
		if err != nil {
			return fmt.Errorf("failed to get job status: %w", err)
		}
		
		switch currentJob.Status {
		case model.JobStatusSuccess:
			fmt.Println("✅ Import completed successfully")
			return nil
		case model.JobStatusError:
			errorMsg := currentJob.Data["error"]
			if errorMsg == "" {
				errorMsg = "Unknown error"
			}
			return fmt.Errorf("import failed: %s", errorMsg)
		case model.JobStatusInProgress, model.JobStatusPending:
			if attempt%6 == 0 { // Print status every minute (6 * 10s)
				fmt.Printf("Job status: %s (check %d/%d)\n", currentJob.Status, attempt+1, maxAttempts)
			}
			time.Sleep(10 * time.Second)
		case model.JobStatusCanceled:
			return fmt.Errorf("import job was canceled")
		default:
			return fmt.Errorf("unknown job status: %s", currentJob.Status)
		}
	}

	return fmt.Errorf("import job did not complete within timeout period")
}

// ImportBulkData imports data using the Mattermost API
func (c *Client) ImportBulkData(filePath string) error {
	fmt.Printf("Starting bulk import from: %s\n", filePath)

	// Check if file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return fmt.Errorf("bulk import file not found: %s", filePath)
	}

	// Create a zip file from the JSONL file
	zipPath := filePath + ".zip"
	if err := CreateZipFile(filePath, zipPath); err != nil {
		return fmt.Errorf("failed to create zip file: %w", err)
	}
	defer func() {
		if removeErr := os.Remove(zipPath); removeErr != nil {
			fmt.Printf("Warning: failed to clean up zip file: %v\n", removeErr)
		}
	}()

	// Upload the zip file
	importFileName, err := c.UploadImportFile(zipPath)
	if err != nil {
		return fmt.Errorf("failed to upload import file: %w", err)
	}

	// Verify the file is available for import
	if err := c.VerifyImportFile(importFileName); err != nil {
		return fmt.Errorf("failed to verify import file: %w", err)
	}

	// Start the import process
	job, err := c.ProcessImport(importFileName)
	if err != nil {
		return fmt.Errorf("failed to start import process: %w", err)
	}

	// Wait for completion
	if err := c.WaitForJobCompletion(job); err != nil {
		return fmt.Errorf("import process failed: %w", err)
	}

	return nil
}

// ChannelCategoryImport represents a channel category import entry
type ChannelCategoryImport struct {
	Team     string   `json:"team"`
	Category string   `json:"category"`
	Channels []string `json:"channels"`
}

// CommandImport represents a command import entry
type CommandImport struct {
	Team    string `json:"team"`
	Channel string `json:"channel"`
	Text    string `json:"text"`
}


// categorizeChannelByName finds a channel by team and name, then applies a category
func (c *Client) categorizeChannelByName(teamName, channelName, categoryName string) error {
	// Get all teams to find the team ID
	teams, resp, err := c.API.GetAllTeams(context.Background(), "", 0, 100)
	if err != nil {
		return handleAPIError("failed to get teams", err, resp)
	}

	var teamID string
	for _, team := range teams {
		if team.Name == teamName {
			teamID = team.Id
			break
		}
	}

	if teamID == "" {
		return fmt.Errorf("team '%s' not found", teamName)
	}

	// Get all channels for this team
	channels, _, err := c.API.GetPublicChannelsForTeam(context.Background(), teamID, 0, 1000, "")
	if err != nil {
		return fmt.Errorf("failed to get public channels: %w", err)
	}

	// Also get private channels
	privateChannels, _, err := c.API.GetPrivateChannelsForTeam(context.Background(), teamID, 0, 1000, "")
	if err == nil {
		channels = append(channels, privateChannels...)
	}

	// Find the specific channel
	var channelID string
	for _, channel := range channels {
		if channel.Name == channelName {
			channelID = channel.Id
			break
		}
	}

	if channelID == "" {
		return fmt.Errorf("channel '%s' not found in team '%s'", channelName, teamName)
	}

	// Apply the category using existing categorization function
	return c.categorizeChannelAPI(channelID, categoryName)
}

// ImportInfrastructureFromBulk imports only infrastructure items from bulk import file
func (c *Client) ImportInfrastructureFromBulk(bulkImportPath string) error {
	// Create temporary infrastructure file
	infraFile, err := os.CreateTemp("", "infrastructure_*.jsonl")
	if err != nil {
		return fmt.Errorf("failed to create temp infrastructure file: %w", err)
	}
	infraPath := infraFile.Name()
	defer func() {
		if closeErr := infraFile.Close(); closeErr != nil {
			fmt.Printf("Warning: failed to close temp file: %v\n", closeErr)
		}
		if removeErr := os.Remove(infraPath); removeErr != nil {
			fmt.Printf("Warning: failed to clean up temp file: %v\n", removeErr)
		}
	}()

	// Read bulk import and write only infrastructure entries
	bulkFile, err := os.Open(bulkImportPath)
	if err != nil {
		return fmt.Errorf("failed to open bulk import file: %w", err)
	}
	defer func() {
		if closeErr := bulkFile.Close(); closeErr != nil {
			fmt.Printf("Warning: failed to close bulk file: %v\n", closeErr)
		}
	}()

	scanner := bufio.NewScanner(bulkFile)
	infraCount := 0

	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}

		// Parse the line to determine type
		var importLine BulkImportLine
		if err := json.Unmarshal([]byte(line), &importLine); err != nil {
			continue // Skip malformed lines
		}

		// Only include actual Mattermost infrastructure import types
		if importLine.Type == "version" || importLine.Type == "team" || importLine.Type == "channel" {
			if _, err := infraFile.WriteString(line + "\n"); err != nil {
				return fmt.Errorf("failed to write infrastructure line: %w", err)
			}
			infraCount++
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading bulk import file: %w", err)
	}

	// Close the file before importing
	if err := infraFile.Close(); err != nil {
		return fmt.Errorf("failed to close temp file: %w", err)
	}

	fmt.Printf("Prepared %d infrastructure items for import\n", infraCount)
	return c.ImportBulkData(infraPath)
}

// ImportUsersFromBulk imports only user items from bulk import file
func (c *Client) ImportUsersFromBulk(bulkImportPath string) error {
	// Create temporary users file
	usersFile, err := os.CreateTemp("", "users_*.jsonl")
	if err != nil {
		return fmt.Errorf("failed to create temp users file: %w", err)
	}
	usersPath := usersFile.Name()
	defer func() {
		if closeErr := usersFile.Close(); closeErr != nil {
			fmt.Printf("Warning: failed to close temp file: %v\n", closeErr)
		}
		if removeErr := os.Remove(usersPath); removeErr != nil {
			fmt.Printf("Warning: failed to clean up temp file: %v\n", removeErr)
		}
	}()

	// Add version line first
	if _, err := usersFile.WriteString("{\"type\": \"version\", \"version\": 1}\n"); err != nil {
		return fmt.Errorf("failed to write version to users file: %w", err)
	}

	// Read bulk import and write only user entries
	bulkFile, err := os.Open(bulkImportPath)
	if err != nil {
		return fmt.Errorf("failed to open bulk import file: %w", err)
	}
	defer func() {
		if closeErr := bulkFile.Close(); closeErr != nil {
			fmt.Printf("Warning: failed to close bulk file: %v\n", closeErr)
		}
	}()

	scanner := bufio.NewScanner(bulkFile)
	userCount := 0

	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}

		// Parse the line to determine type
		var importLine BulkImportLine
		if err := json.Unmarshal([]byte(line), &importLine); err != nil {
			continue // Skip malformed lines
		}

		// Only include user entries
		if importLine.Type == "user" {
			if _, err := usersFile.WriteString(line + "\n"); err != nil {
				return fmt.Errorf("failed to write user line: %w", err)
			}
			userCount++
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading bulk import file: %w", err)
	}

	// Close the file before importing
	if err := usersFile.Close(); err != nil {
		return fmt.Errorf("failed to close temp file: %w", err)
	}

	fmt.Printf("Prepared %d users for import\n", userCount)
	return c.ImportBulkData(usersPath)
}

// ProcessChannelCategoriesFromBulk processes channel categories directly from bulk import file
func (c *Client) ProcessChannelCategoriesFromBulk(bulkImportPath string) error {
	fmt.Println("Processing channel categories...")

	// Open bulk import file
	file, err := os.Open(bulkImportPath)
	if err != nil {
		return fmt.Errorf("failed to open bulk import file: %w", err)
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			fmt.Printf("Warning: failed to close bulk import file: %v\n", closeErr)
		}
	}()

	// Process channel-category entries
	scanner := bufio.NewScanner(file)
	categoryCount := 0

	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}

		// Parse the line to check if it's a channel-category
		var importLine BulkImportLine
		if err := json.Unmarshal([]byte(line), &importLine); err != nil {
			continue // Skip malformed lines
		}

		if importLine.Type == "channel-category" {
			// Parse the full channel-category entry
			var categoryEntry ChannelCategoryImport

			if err := json.Unmarshal([]byte(line), &categoryEntry); err != nil {
				fmt.Printf("Warning: failed to parse channel-category entry, skipping: %v\n", err)
				continue
			}

			// Process each channel in this category
			for _, channelName := range categoryEntry.Channels {
				if err := c.categorizeChannelByName(categoryEntry.Team, 
					channelName, categoryEntry.Category); err != nil {
					fmt.Printf("Warning: failed to categorize channel '%s': %v\n", 
						channelName, err)
					continue
				}

				fmt.Printf("✅ Categorized channel '%s' in category '%s'\n", 
					channelName, categoryEntry.Category)
				categoryCount++
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading bulk import file: %w", err)
	}

	fmt.Printf("✅ Processed %d channel categories\n", categoryCount)
	return nil
}

// ProcessCommandsFromBulk processes commands directly from bulk import file
func (c *Client) ProcessCommandsFromBulk(bulkImportPath string) error {
	fmt.Println("Processing channel commands...")

	// Open bulk import file
	file, err := os.Open(bulkImportPath)
	if err != nil {
		return fmt.Errorf("failed to open bulk import file: %w", err)
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			fmt.Printf("Warning: failed to close bulk import file: %v\n", closeErr)
		}
	}()

	// Process command entries
	scanner := bufio.NewScanner(file)
	commandCount := 0

	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}

		// Parse the line to check if it's a command
		var importLine BulkImportLine
		if err := json.Unmarshal([]byte(line), &importLine); err != nil {
			continue // Skip malformed lines
		}

		if importLine.Type == "command" {
			// Parse the full command entry
			var commandEntry struct {
				Type    string        `json:"type"`
				Command CommandImport `json:"command"`
			}

			if err := json.Unmarshal([]byte(line), &commandEntry); err != nil {
				fmt.Printf("Warning: failed to parse command entry, skipping: %v\n", err)
				continue
			}

			// Execute the command in the specified channel
			if err := c.executeCommandInChannel(commandEntry.Command.Team, 
				commandEntry.Command.Channel, commandEntry.Command.Text); err != nil {
				fmt.Printf("Warning: failed to execute command '%s' in channel '%s': %v\n", 
					commandEntry.Command.Text, commandEntry.Command.Channel, err)
				continue
			}

			fmt.Printf("✅ Executed command '%s' in channel '%s'\n", 
				commandEntry.Command.Text, commandEntry.Command.Channel)
			commandCount++
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading bulk import file: %w", err)
	}

	fmt.Printf("✅ Executed %d commands\n", commandCount)
	return nil
}

// executeCommandInChannel executes a slash command in a specific channel
func (c *Client) executeCommandInChannel(teamName, channelName, commandText string) error {
	// Get all teams to find the team ID
	teams, resp, err := c.API.GetAllTeams(context.Background(), "", 0, 100)
	if err != nil {
		return handleAPIError("failed to get teams", err, resp)
	}

	var teamID string
	for _, team := range teams {
		if team.Name == teamName {
			teamID = team.Id
			break
		}
	}

	if teamID == "" {
		return fmt.Errorf("team '%s' not found", teamName)
	}

	// Get all channels for this team
	channels, _, err := c.API.GetPublicChannelsForTeam(context.Background(), teamID, 0, 1000, "")
	if err != nil {
		return fmt.Errorf("failed to get public channels: %w", err)
	}

	// Also get private channels
	privateChannels, _, err := c.API.GetPrivateChannelsForTeam(context.Background(), teamID, 0, 1000, "")
	if err == nil {
		channels = append(channels, privateChannels...)
	}

	// Find the specific channel
	var channelID string
	for _, channel := range channels {
		if channel.Name == channelName {
			channelID = channel.Id
			break
		}
	}

	if channelID == "" {
		return fmt.Errorf("channel '%s' not found in team '%s'", channelName, teamName)
	}

	// Validate command format
	commandText = strings.TrimSpace(commandText)
	if !strings.HasPrefix(commandText, "/") {
		return fmt.Errorf("invalid command '%s' - must start with /", commandText)
	}

	// Execute the command using the commands/execute API
	_, resp, err = c.API.ExecuteCommand(context.Background(), channelID, commandText)
	if err != nil {
		return fmt.Errorf("failed to execute command '%s': %w", commandText, err)
	}

	if resp.StatusCode != 200 {
		return fmt.Errorf("command '%s' returned status code %d", commandText, resp.StatusCode)
	}

	return nil
}

// BulkImportLine represents a single line in the bulk import JSONL file
type BulkImportLine struct {
	Type    string          `json:"type"`
	Version int             `json:"version,omitempty"`
	Raw     json.RawMessage `json:"-"` // Store original JSON for writing
}





// SetupWithBulkImport performs setup using bulk import instead of individual API calls
// This does the full import in one go (legacy behavior)
func (c *Client) SetupWithBulkImport() error {
	// Determine the correct path to bulk_import.jsonl
	var bulkImportPath string
	if _, err := os.Stat("bulk_import.jsonl"); err == nil {
		bulkImportPath = "bulk_import.jsonl"
	} else if _, err := os.Stat("../bulk_import.jsonl"); err == nil {
		bulkImportPath = "../bulk_import.jsonl"
	} else {
		return fmt.Errorf("bulk_import.jsonl not found in current directory or parent directory")
	}

	fmt.Printf("Using bulk import file: %s\n", bulkImportPath)

	// Import the data
	if err := c.ImportBulkData(bulkImportPath); err != nil {
		return fmt.Errorf("failed to import bulk data: %w", err)
	}

	fmt.Println("✅ Bulk import completed successfully")
	return nil
}

// SetupWithSplitImport performs setup using two-phase bulk import
func (c *Client) SetupWithSplitImport() error {
	fmt.Println("Starting two-phase bulk import...")

	// Determine the correct path to bulk_import.jsonl
	var bulkImportPath string
	if _, err := os.Stat("bulk_import.jsonl"); err == nil {
		bulkImportPath = "bulk_import.jsonl"
	} else if _, err := os.Stat("../bulk_import.jsonl"); err == nil {
		bulkImportPath = "../bulk_import.jsonl"
	} else {
		return fmt.Errorf("bulk_import.jsonl not found in current directory or parent directory")
	}

	// Phase 1: Create infrastructure (teams and channels)
	fmt.Println("Creating infrastructure (teams and channels)...")
	if err := c.ImportInfrastructureFromBulk(bulkImportPath); err != nil {
		return fmt.Errorf("failed to import infrastructure: %w", err)
	}
	fmt.Println("✅ Infrastructure creation completed successfully")

	// Phase 1.5: Process channel categories
	if err := c.ProcessChannelCategoriesFromBulk(bulkImportPath); err != nil {
		return fmt.Errorf("failed to process channel categories: %w", err)
	}

	// Phase 2: Create users
	fmt.Println("Creating users...")
	if err := c.ImportUsersFromBulk(bulkImportPath); err != nil {
		return fmt.Errorf("failed to import users: %w", err)
	}
	fmt.Println("✅ User creation completed successfully")

	// Phase 3: Execute commands
	if err := c.ProcessCommandsFromBulk(bulkImportPath); err != nil {
		return fmt.Errorf("failed to process commands: %w", err)
	}

	fmt.Println("✅ Two-phase bulk import completed successfully")
	return nil
}