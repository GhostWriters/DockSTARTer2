package appenv

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Set sets the variable in the file.
// If it exists, it replaces the first occurrence and removes others.
// If it doesn't exist, it appends to the end.
func Set(key, value, file string) error {
	// Use single quotes and escape internal single quotes
	escapedVal := strings.ReplaceAll(value, "'", `'"'"'`)
	newLine := fmt.Sprintf("%s='%s'", key, escapedVal)

	return setInFile(key, newLine, file)
}

// SetLiteral sets the variable with the raw value provided.
func SetLiteral(key, value, file string) error {
	newLine := fmt.Sprintf("%s=%s", key, value)
	return setInFile(key, newLine, file)
}

func setInFile(key, newLine, file string) error {
	var lines []string
	found := false
	re := regexp.MustCompile(fmt.Sprintf(`^\s*%s\s*=`, regexp.QuoteMeta(key)))

	if _, err := os.Stat(file); err == nil {
		f, err := os.Open(file)
		if err != nil {
			return err
		}
		defer f.Close()

		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := scanner.Text()
			if re.MatchString(line) {
				if !found {
					lines = append(lines, newLine)
					found = true
				}
				// Skip subsequent occurrences to avoid duplicates
			} else {
				lines = append(lines, line)
			}
		}
		if err := scanner.Err(); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}

	if !found {
		lines = append(lines, newLine)
	}

	return writeLines(lines, file)
}

func writeLines(lines []string, file string) error {
	// Ensure dir exists
	if err := os.MkdirAll(filepath.Dir(file), 0755); err != nil {
		return err
	}

	f, err := os.Create(file)
	if err != nil {
		return err
	}
	defer f.Close()

	writer := bufio.NewWriter(f)

	for _, line := range lines {
		writer.WriteString(line + "\n")
	}
	return writer.Flush()
}
