package mattermost

import (
	"os"

	"github.com/sirupsen/logrus"
)

// Log is the package logger - use this directly
var Log *logrus.Logger

// LogConfig holds configuration for logger setup
type LogConfig struct {
	Level   logrus.Level
	Format  string // "text" or "json"
	Verbose bool
}

// InitLogger initializes the package logger
func InitLogger(config *LogConfig) {
	Log = logrus.New()
	Log.SetLevel(config.Level)
	Log.SetOutput(os.Stdout)

	if config.Format == "json" {
		Log.SetFormatter(&logrus.JSONFormatter{})
	} else {
		Log.SetFormatter(&logrus.TextFormatter{
			DisableColors:          false,
			FullTimestamp:          config.Verbose,
			TimestampFormat:        "15:04:05",
			DisableTimestamp:       !config.Verbose,
			DisableLevelTruncation: true,
		})
	}

	if config.Verbose {
		Log.SetReportCaller(true)
	}
}

// init ensures the logger is always available
func init() {
	InitLogger(&LogConfig{
		Level:   logrus.InfoLevel,
		Format:  "text",
		Verbose: false,
	})
}