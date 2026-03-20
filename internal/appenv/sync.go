package appenv

import (
	"DockSTARTer2/internal/logger"
	"context"
	"fmt"
	"sort"
)

// SyncVariables performs a surgical per-variable update of a file.
// It uses initialVars (state when editor opened) and newVars (current state) to
// calculate a scoped diff, ensuring only variables in the editor tab are modified.
// Mirrors the bash approach of calling env_set / env_delete one variable at a time.
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
			logger.Notice(ctx, "\t{{|Var|}}%s{{[-]}}={{|Highlight|}}%s{{[-]}}", k, newVars[k])
		}
	}
	if len(updated) > 0 {
		logger.Notice(ctx, "Updating variables in {{|File|}}%s{{[-]}}:", file)
		for _, k := range updated {
			logger.Notice(ctx, "\t{{|Var|}}%s{{[-]}}={{|Highlight|}}%s{{[-]}}", k, newVars[k])
		}
	}
	if len(removed) > 0 {
		logger.Notice(ctx, "Removing variables from {{|File|}}%s{{[-]}}:", file)
		for _, k := range removed {
			logger.Notice(ctx, "\t{{|Var|}}%s{{[-]}}", k)
		}
	}

	// 3. Apply changes one variable at a time (mirrors bash env_delete / env_set_literal)
	for _, k := range removed {
		if err := unsetVarInFile(ctx, k, file); err != nil {
			return fmt.Errorf("failed to remove %s: %w", k, err)
		}
	}
	for _, k := range updated {
		if err := SetLiteral(ctx, k, newVars[k], file); err != nil {
			return fmt.Errorf("failed to update %s: %w", k, err)
		}
	}
	for _, k := range added {
		if err := SetLiteral(ctx, k, newVars[k], file); err != nil {
			return fmt.Errorf("failed to add %s: %w", k, err)
		}
	}

	return nil
}
