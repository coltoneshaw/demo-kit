package logger

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/sirupsen/logrus"
)

// Log is the global logger instance - use this directly
var Log *logrus.Logger

// CleanTextFormatter is a custom formatter for cleaner verbose output
type CleanTextFormatter struct {
	*logrus.TextFormatter
	Verbose bool
}

// customFieldSort sorts fields alphabetically but puts "caller" at the end
func customFieldSort(keys []string) {
	sort.Slice(keys, func(i, j int) bool {
		// Put "caller" at the end
		if keys[i] == "caller" {
			return false
		}
		if keys[j] == "caller" {
			return true
		}
		// Sort all other fields alphabetically
		return keys[i] < keys[j]
	})
}

// Format renders a single log entry with timestamp first and cleaner caller paths
func (f *CleanTextFormatter) Format(entry *logrus.Entry) ([]byte, error) {
	var b *bytes.Buffer
	if entry.Buffer != nil {
		b = entry.Buffer
	} else {
		b = &bytes.Buffer{}
	}

	// Clean up caller info if verbose mode
	if f.Verbose && entry.Caller != nil {
		filename := filepath.Base(entry.Caller.File)
		funcName := entry.Caller.Function
		
		// Shorten the function name (remove package path)
		if idx := strings.LastIndex(funcName, "."); idx != -1 {
			funcName = funcName[idx+1:]
		}
		
		// Create a cleaner caller format
		entry.Data["caller"] = filename + ":" + funcName + "()"
	}

	// Format: [15:04:05] LEVEL message key=value caller="file.go:func()"
	
	// Always add timestamp first
	fmt.Fprintf(b, "[%s] ", entry.Time.Format("15:04:05"))
	
	// Add level with color if enabled
	level := strings.ToUpper(entry.Level.String())
	if !f.DisableColors {
		levelColor := getLevelColor(entry.Level)
		fmt.Fprintf(b, "\x1b[%dm%-5s\x1b[0m ", levelColor, level)
	} else {
		fmt.Fprintf(b, "%-5s ", level)
	}
	
	// Add message
	b.WriteString(entry.Message)
	
	// Add fields using custom sorting
	if len(entry.Data) > 0 {
		keys := make([]string, 0, len(entry.Data))
		for k := range entry.Data {
			keys = append(keys, k)
		}
		customFieldSort(keys)
		
		for _, k := range keys {
			v := entry.Data[k]
			
			// Add field key with color if enabled
			if !f.DisableColors {
				fmt.Fprintf(b, " \x1b[36m%s\x1b[0m=", k) // cyan for keys
			} else {
				fmt.Fprintf(b, " %s=", k)
			}
			
			// Quote values that contain spaces or special characters
			str := fmt.Sprintf("%v", v)
			if strings.ContainsAny(str, " \t\n\r") || strings.Contains(str, "=") {
				fmt.Fprintf(b, "\"%s\"", str)
			} else {
				b.WriteString(str)
			}
		}
	}
	
	b.WriteByte('\n')
	return b.Bytes(), nil
}

// getLevelColor returns the ANSI color code for log levels
func getLevelColor(level logrus.Level) int {
	switch level {
	case logrus.DebugLevel:
		return 37 // white
	case logrus.InfoLevel:
		return 36 // cyan
	case logrus.WarnLevel:
		return 33 // yellow
	case logrus.ErrorLevel, logrus.FatalLevel, logrus.PanicLevel:
		return 31 // red
	default:
		return 37 // white
	}
}

// Init initializes the global logger with the given configuration
func Init(config *Config) {
	Log = logrus.New()
	Log.SetLevel(config.Level)
	Log.SetOutput(os.Stdout)

	if config.Format == "json" {
		Log.SetFormatter(&logrus.JSONFormatter{})
	} else {
		Log.SetFormatter(&CleanTextFormatter{
			TextFormatter: &logrus.TextFormatter{
				DisableColors:          false,
				FullTimestamp:          config.Verbose,
				TimestampFormat:        "15:04:05",
				DisableTimestamp:       !config.Verbose,
				DisableLevelTruncation: true,
				ForceColors:            true,
				DisableSorting:         false,
				SortingFunc:            customFieldSort,
			},
			Verbose: config.Verbose,
		})
	}

	if config.Verbose {
		Log.SetReportCaller(true)
	}
}

// init ensures the logger is always available with default settings
func init() {
	Init(&Config{
		Level:   logrus.InfoLevel,
		Format:  "text",
		Verbose: false,
	})
}