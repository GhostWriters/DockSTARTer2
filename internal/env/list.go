package env

import (
	"bufio"
	"os"
	"regexp"
)

// ListVars returns a map of all variable keys found in the file.
func ListVars(file string) (map[string]bool, error) {
	keys := make(map[string]bool)
	f, err := os.Open(file)
	if err != nil {
		if os.IsNotExist(err) {
			return keys, nil
		}
		return nil, err
	}
	defer f.Close()

	// Regex for valid shell variable assignment: ^\s*([a-zA-Z_][a-zA-Z0-9_]*)=
	re := regexp.MustCompile(`^\s*([a-zA-Z_][a-zA-Z0-9_]*)=`)

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		matches := re.FindStringSubmatch(line)
		if matches != nil {
			keys[matches[1]] = true
		}
	}
	return keys, scanner.Err()
}
