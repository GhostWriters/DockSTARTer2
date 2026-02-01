package appenv

import (
	"sort"
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
// It mimics the Bash replace_with_vars function but ensures deterministic replacement order.
// Keys are sorted by the length of their values (descending) to ensure specific paths
// are replaced before their parent paths (e.g., replace DOCKER_CONFIG_FOLDER before HOME).
func ReplaceWithVars(s string, vars map[string]string) string {
	type kv struct {
		Key string
		Val string
	}

	var sorted []kv
	for k, v := range vars {
		if v != "" {
			sorted = append(sorted, kv{Key: k, Val: v})
		}
	}

	// Sort by value length descending. If lengths equal, sort by key ascending for stability.
	sort.Slice(sorted, func(i, j int) bool {
		if len(sorted[i].Val) != len(sorted[j].Val) {
			return len(sorted[i].Val) > len(sorted[j].Val)
		}
		return sorted[i].Key < sorted[j].Key
	})

	for _, item := range sorted {
		replacement := "${" + item.Key + "?}"
		s = strings.ReplaceAll(s, item.Val, replacement)
	}
	return s
}
