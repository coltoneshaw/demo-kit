package command

import "strings"

// parseArgs parses command arguments from Mattermost slash command format
func parseArgs(command string) map[string]string {
	args := map[string]string{}

	// Skip command name
	parts := strings.Split(command, " ")
	if len(parts) <= 1 {
		return args
	}

	// Skip the first part, which is the command itself
	parts = parts[1:]

	var currentKey string
	var value []string

	for i := 0; i < len(parts); i++ {
		part := parts[i]

		if strings.HasPrefix(part, "--") {
			// Save previous key-value pair if exists
			if currentKey != "" && len(value) > 0 {
				args[currentKey] = strings.Join(value, " ")
			}

			// Start new key
			currentKey = strings.TrimPrefix(part, "--")
			value = []string{}
		} else if currentKey != "" {
			// Accumulate values for current key
			value = append(value, part)
		} else if i == 0 && !strings.HasPrefix(part, "--") {
			// First argument is direct action without flag (for simpler commands)
			args["action"] = part
		}
	}

	// Save final key-value pair if exists
	if currentKey != "" && len(value) > 0 {
		args[currentKey] = strings.Join(value, " ")
	}

	return args
}
