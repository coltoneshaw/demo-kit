package mattermost

import (
	"fmt"
	"io"
	"os"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/sirupsen/logrus"
)

// LogOptions contains options for controlling log output
type LogOptions struct {
	Suppress bool   // Suppress all output
	Prefix   string // Prefix to add to all log messages
}

// handleAPIError creates a formatted error from API responses.
// This standardizes error handling across API calls.
func handleAPIError(operation string, err error, resp *model.Response) error {
	if err != nil {
		if resp != nil {
			return fmt.Errorf("%s: %w (status code: %v)", operation, err, resp.StatusCode)
		}
		return fmt.Errorf("%s: %w", operation, err)
	}
	return nil
}

// closeWithLog is a helper function for closing resources with error logging
func closeWithLog(c io.Closer, label string) {
	if err := c.Close(); err != nil {
		Log.WithFields(logrus.Fields{"label": label, "error": err.Error()}).Warn("⚠️ Failed to close resource")
	}
}

// removeWithLog is a helper function for removing files with error logging
func removeWithLog(path string) {
	if err := os.Remove(path); err != nil {
		Log.WithFields(logrus.Fields{"file_path": path, "error": err.Error()}).Warn("⚠️ Failed to remove file")
	}
}