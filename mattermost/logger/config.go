package logger

import "github.com/sirupsen/logrus"

// Config holds configuration for logger setup
type Config struct {
	Level   logrus.Level
	Format  string // "text" or "json"
	Verbose bool
}