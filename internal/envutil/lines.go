package envutil

import (
	"bufio"
	"os"
	"regexp"
)

// ReadLines reads variable lines from a file (equivalent to env_lines script).
// Returns only lines that contain valid environment variable assignments (VAR=value).
// Skips empty lines and comments.
// ReadLines reads variable lines from a file (equivalent to env_lines script).
// It matches lines starting with optional whitespace, followed by a KEY, optional whitespace, and an '='.
// It returns "KEY=original_content_after_assignment_operator".
func ReadLines(filename string) ([]string, error) {
	file, err := os.Open(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	// Regex matching: ^\s*([A-Za-z0-9_]*)\s*=
	re := regexp.MustCompile(`^\s*([A-Za-z0-9_]+)\s*=(.*)`)

	for scanner.Scan() {
		line := scanner.Text()
		matches := re.FindStringSubmatch(line)
		if matches != nil {
			key := matches[1]
			rest := matches[2]
			// Parity with env_lines.sh: sed -n "s/^\s*\([A-Za-z0-9_]*\)\s*=/\1=/p"
			// This keeps everything after the '=' verbatim.
			lines = append(lines, key+"="+rest)
		}
	}

	return lines, scanner.Err()
}
