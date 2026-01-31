package appenv

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
)

// Get returns the value of the variable from the file.
// Logic mirrors env_get.sh:
// 1. Reads the file.
// 2. Finds the line definition.
// 3. Parses the value respecting quotes and comments.
func Get(key, file string) (string, error) {
	literal, err := GetLiteral(key, file)
	if err != nil {
		return "", err
	}
	if literal == "" {
		return "", nil
	}

	// literal is everything after the first '='
	// Bash regex: ^\s*(?:(?:(?<Q>['"]).*\k<Q>)|(?:[^\s]+(?:\s+(?!#)[^\s]+)*))
	val := strings.TrimLeft(literal, " \t")

	// 1. Quoted string (must start and end with same quote)
	if len(val) >= 2 {
		quote := val[0]
		if quote == '"' || quote == '\'' {
			// Find matching end quote
			// In Bash/Grep regex context, it's greedy: .*\k<Q>
			// But usually it matches the first matching quote that isn't escaped?
			// The Bash regex says .*\k<Q> which is greedy until the LAST matching quote.
			lastIdx := strings.LastIndexByte(val, quote)
			if lastIdx > 0 {
				return val[1:lastIdx], nil
			}
		}
	}

	// 2. Unquoted value: matches until we hit " #" (space followed by hash)
	// or until end of string.
	// We also need to handle the case where it starts with # (it shouldn't be matched as a comment unless it's " #")
	// The Bash regex (?:[^\s]+(?:\s+(?!#)[^\s]+)*)) actually implies it can't START with a space,
	// but can start with # if it's not a space.
	// Actually, if val is "#Value", it should be returned as is.
	// If val is "Value #Comment", it should be "Value".
	// If val is "Value#NotAComment", it should be "Value#NotAComment".

	if idx := strings.Index(val, " #"); idx != -1 {
		return strings.TrimRight(val[:idx], " \t"), nil
	}

	// No comment found, return trimmed space from right
	return strings.TrimRight(val, " \t"), nil
}

// GetLine returns the full line containing the variable definition.
// Mirrors env_get_line.sh.
func GetLine(key, file string) (string, error) {
	f, err := os.Open(file)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	defer f.Close()

	// Regex to match variable definition: ^\s*KEY\s*=
	re := regexp.MustCompile(fmt.Sprintf(`^\s*%s\s*=`, regexp.QuoteMeta(key)))

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if re.MatchString(line) {
			return line, nil
		}
	}
	return "", scanner.Err()
}

// GetLineRegex returns all lines matching the variable key regex.
// Mirrors env_get_line_regex.sh.
func GetLineRegex(keyRegex, file string) ([]string, error) {
	var lines []string
	f, err := os.Open(file)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	re := regexp.MustCompile(fmt.Sprintf(`^\s*(%s)\s*=`, keyRegex))

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if re.MatchString(line) {
			lines = append(lines, line)
		}
	}
	sort.Strings(lines)
	return lines, scanner.Err()
}

// GetLiteral returns the raw value part (RHS) of the variable definition.
// Mirrors env_get_literal.sh: returns line content after first '='.
func GetLiteral(key, file string) (string, error) {
	line, err := GetLine(key, file)
	if err != nil || line == "" {
		return "", err
	}
	parts := strings.SplitN(line, "=", 2)
	if len(parts) == 2 {
		return parts[1], nil
	}
	return "", nil
}

// GetLineNumber returns the line number of the variable definition.
// GetLineNumber returns the line number of the variable definition.
func GetLineNumber(key, file string) (int, error) {
	f, err := os.Open(file)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	defer f.Close()

	lineNum := 0
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		lineNum++
	}
	return 0, nil
}
