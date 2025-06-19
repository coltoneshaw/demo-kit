package mattermost

import (
	"github.com/coltoneshaw/demokit/mattermost/logger"
)

// Log is an alias to the global logger for backward compatibility
var Log = logger.Log

// LogConfig is an alias for backward compatibility
type LogConfig = logger.Config

// InitLogger initializes the logger with backward compatibility
func InitLogger(config *LogConfig) {
	logger.Init(config)
	// Update the local Log reference in case it was replaced
	Log = logger.Log
}
