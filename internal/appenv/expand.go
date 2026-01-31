package appenv

import (
	"strings"
)

// ExpandVars recursively expands variables in a string until no more changes occur or a limit is reached.
// It mimics the Bash expand_vars function.
func ExpandVars(s string, vars map[string]string) string {
	changed := true
	loopCount := 0
	maxLoops := 10

	for changed && loopCount < maxLoops {
		changed = false
		for key, val := range vars {
			// Check for ${KEY?} pattern
			pattern1 := "${" + key + "?}"
			if strings.Contains(s, pattern1) {
				newS := strings.ReplaceAll(s, pattern1, val)
				if newS != s {
					s = newS
					changed = true
				}
			}
			// Check for ${KEY} pattern
			pattern2 := "${" + key + "}"
			if strings.Contains(s, pattern2) {
				newS := strings.ReplaceAll(s, pattern2, val)
				if newS != s {
					s = newS
					changed = true
				}
			}
		}
		loopCount++
	}
	return s
}

// ReplaceWithVars replaces occurrences of specified variables in the string.
// It mimics the Bash replace_with_vars function.
// It specifically handles patterns like ${KEY?}.
func ReplaceWithVars(s string, vars map[string]string) string {
	// bash version iterates over keys in order passed, but map is unordered.
	// bash version escapes patterns for sed/regex. Go strings.ReplaceAll handles literal string matching.

	for key, val := range vars {
		if val == "" {
			continue
		}
		// Bash replace_with_vars constructs replacement as ${KEY?}
		// Wait, usage in env_sanitize is:
		// replace_with_vars "${UpdatedValue}" DOCKER_CONFIG_FOLDER "${DOCKER_CONFIG_FOLDER}" ...
		// It REPLACES the *value* (path) WITH the variable reference *pattern* (${KEY?}).
		// It is the INVERSE of expansion. It's used to sanitize absolute paths BACK to variables.

		// In Bash: Pattern="${Value...}", Replacement="\${${Key}?}"
		// String="${String//${Pattern}/${Replacement}}"

		// So if Value is "/home/user/.config", Key is "DOCKER_CONFIG_FOLDER"
		// It replaces "/home/user/.config" with "${DOCKER_CONFIG_FOLDER?}"

		replacement := "${" + key + "?}"
		s = strings.ReplaceAll(s, val, replacement)
	}
	return s
}
