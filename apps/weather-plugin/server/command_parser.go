package main

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

type CommandParser struct{}

type SubscribeArgs struct {
	Location        string
	FrequencyStr    string
	UpdateFrequency int64
}

func NewCommandParser() *CommandParser {
	return &CommandParser{}
}

func (cp *CommandParser) ParseSubscribeCommand(commandFields []string) (*SubscribeArgs, error) {
	if len(commandFields) < 4 {
		return nil, fmt.Errorf("insufficient arguments")
	}

	args := &SubscribeArgs{}

	// Check if using flag syntax or simple syntax
	if len(commandFields) >= 4 && !strings.HasPrefix(commandFields[2], "--") {
		// Simple syntax: /weather subscribe <location> <frequency>
		args.Location = commandFields[2]
		args.FrequencyStr = commandFields[3]
	} else {
		// Flag syntax: /weather subscribe --location <location> --frequency <frequency>
		err := cp.parseFlagSyntax(commandFields[2:], args)
		if err != nil {
			return nil, err
		}
	}

	if args.Location == "" || args.FrequencyStr == "" {
		return nil, fmt.Errorf("missing required parameters: location and frequency")
	}

	// Parse frequency
	err := cp.parseFrequency(args)
	if err != nil {
		return nil, err
	}

	// Validate minimum frequency (30 seconds)
	if args.UpdateFrequency < 30000 {
		return nil, fmt.Errorf("update frequency must be at least 30000 milliseconds (30 seconds)")
	}

	return args, nil
}

func (cp *CommandParser) parseFlagSyntax(fields []string, args *SubscribeArgs) error {
	for i := 0; i < len(fields); i++ {
		switch fields[i] {
		case "--location":
			if i+1 < len(fields) {
				// Collect all words until next flag or end
				locationParts := []string{}
				j := i + 1
				for ; j < len(fields) && !strings.HasPrefix(fields[j], "--"); j++ {
					locationParts = append(locationParts, fields[j])
				}
				args.Location = strings.Join(locationParts, " ")
				i = j - 1 // Skip processed words
			}
		case "--frequency":
			if i+1 < len(fields) {
				args.FrequencyStr = fields[i+1]
				i++ // Skip the frequency value
			}
		}
	}
	return nil
}

func (cp *CommandParser) parseFrequency(args *SubscribeArgs) error {
	// Try to parse as milliseconds first
	updateFrequency, err := strconv.ParseInt(args.FrequencyStr, 10, 64)
	if err != nil {
		// Try to parse as duration
		duration, err := time.ParseDuration(args.FrequencyStr)
		if err != nil {
			return fmt.Errorf("invalid frequency: %s. Please use milliseconds (e.g., 60000 for 1 minute) or a valid duration like 30s, 5m, 1h", args.FrequencyStr)
		}
		updateFrequency = duration.Milliseconds()
	}

	args.UpdateFrequency = updateFrequency
	return nil
}