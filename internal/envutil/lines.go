package envutil

import (
	"bufio"
	"os"
	"strings"
)

// ReadLines reads variable lines from a file (equivalent to env_lines script).
// Returns only lines that contain valid environment variable assignments (VAR=value).
// Skips empty lines and comments.
func ReadLines(filename string) ([]string, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		// Skip empty lines and comments
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		// Only include lines with = (valid env vars)
		if strings.Contains(line, "=") {
			lines = append(lines, line)
		}
	}

	return lines, scanner.Err()
}
