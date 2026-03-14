package appenv

import (
	"DockSTARTer2/internal/logger"
	"DockSTARTer2/internal/system"
	"context"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
)

// SyncVariables performs a surgical single-pass update of multiple variables in a file.
// It uses initialVars (state when editor opened) and newVars (current state) to
// calculate a scoped diff, ensuring only variables in the editor tab are modified.
func SyncVariables(ctx context.Context, file string, initialVars, newVars map[string]string) error {
	// 1. Calculate the diff based on the editor's scope
	var added, updated, removed []string
	
	for k, v := range newVars {
		if initialVal, exists := initialVars[k]; !exists {
			added = append(added, k)
		} else if initialVal != v {
			updated = append(updated, k)
		}
	}
	for k := range initialVars {
		if _, exists := newVars[k]; !exists {
			removed = append(removed, k)
		}
	}

	// Exit early if nothing to do
	if len(added) == 0 && len(updated) == 0 && len(removed) == 0 {
		return nil
	}

	// 2. Log surgical notices
	sort.Strings(added)
	sort.Strings(updated)
	sort.Strings(removed)

	if len(added) > 0 {
		logger.Notice(ctx, "Adding variables to {{|File|}}%s{{[-]}}:", file)
		for _, k := range added {
			logger.Notice(ctx, "\t{{|Var|}}%s{{[-]}}={{|Theme_Highlight|}}%s{{[-]}}", k, newVars[k])
		}
	}
	if len(updated) > 0 {
		logger.Notice(ctx, "Updating variables in {{|File|}}%s{{[-]}}:", file)
		for _, k := range updated {
			logger.Notice(ctx, "\t{{|Var|}}%s{{[-]}}={{|Theme_Highlight|}}%s{{[-]}}", k, newVars[k])
		}
	}
	if len(removed) > 0 {
		logger.Notice(ctx, "Removing variables from {{|File|}}%s{{[-]}}:", file)
		for _, k := range removed {
			logger.Notice(ctx, "\t{{|Var|}}%s{{[-]}}", k)
		}
	}

	// 3. Process the file surgical-style
	content, err := os.ReadFile(file)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	lines := strings.Split(string(content), "\n")
	var resultLines []string
	removedMap := make(map[string]bool)
	for _, k := range removed {
		removedMap[k] = true
	}

	re := regexp.MustCompile(`^\s*([a-zA-Z_][a-zA-Z0-9_]*)=`)
	processedKeys := make(map[string]bool)

	for _, line := range lines {
		// Clean up trailing carriage returns if any (from Windows env)
		line = strings.TrimRight(line, "\r")
		
		matches := re.FindStringSubmatch(line)
		if matches != nil {
			key := matches[1]
			if removedMap[key] {
				// Skip this line (removed)
				continue
			}
			if newVal, exists := newVars[key]; exists {
				// Replace line with new literal value (includes quotes/comments from editor)
				resultLines = append(resultLines, fmt.Sprintf("%s=%s", key, newVal))
				processedKeys[key] = true
				continue
			}
		}
		resultLines = append(resultLines, line)
	}

	// Append added variables (that weren't already in the file)
	for _, k := range added {
		if !processedKeys[k] {
			val := newVars[k]
			resultLines = append(resultLines, fmt.Sprintf("%s=%s", k, val))
		}
	}

	// 4. Write back
	// Ensure we don't end up with multiple trailing newlines
	finalContent := strings.Join(resultLines, "\n")
	finalContent = strings.TrimRight(finalContent, "\n") + "\n"
	
	if err := os.WriteFile(file, []byte(finalContent), 0644); err != nil {
		return err
	}
	system.SetPermissions(ctx, file)

	return nil
}
