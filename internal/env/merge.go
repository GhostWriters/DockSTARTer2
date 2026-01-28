package env

import (
	"DockSTARTer2/internal/logger"
	"bufio"
	"context"
	"os"
	"regexp"
)

// MergeNewOnly merges variables from source file to target file, adding only new ones.
//
// This function mirrors env_merge_newonly.sh and copies variables from the source
// file to the target file, but only if they don't already exist in the target.
//
// Behavior:
//   - If source file doesn't exist: logs a warning and returns nil
//   - If target file doesn't exist: creates it as an empty file first
//   - Skips variables that already exist in the target (no overwriting)
//   - Logs each variable being added with its full definition line
//   - Prevents duplicates even if the source file contains them
//
// Returns a slice of the new variable names that were added, or nil if none.
func MergeNewOnly(ctx context.Context, targetFile, sourceFile string) ([]string, error) {
	var addedvars []string

	if _, err := os.Stat(sourceFile); os.IsNotExist(err) {
		logger.Warn(ctx, "File '{{_File_}}%s{{|-|}}' does not exist.", sourceFile)
		return nil, nil // Source doesn't exist, nothing to merge
	}

	// Ensure target exists
	if _, err := os.Stat(targetFile); os.IsNotExist(err) {
		// Bash behavior: touch file, then merge.
		if err := os.WriteFile(targetFile, []byte{}, 0644); err != nil {
			return nil, err
		}
	}

	targetVars, err := ListVars(targetFile)
	if err != nil {
		return nil, err
	}

	fSource, err := os.Open(sourceFile)
	if err != nil {
		return nil, err
	}
	defer fSource.Close()

	var newLines []string
	re := regexp.MustCompile(`^\s*([a-zA-Z_][a-zA-Z0-9_]*)=`)

	scanner := bufio.NewScanner(fSource)
	for scanner.Scan() {
		line := scanner.Text()
		matches := re.FindStringSubmatch(line)
		if matches != nil {
			key := matches[1]
			if !targetVars[key] {
				newLines = append(newLines, line)
				addedvars = append(addedvars, key)
				// Add to local map to avoid duplicates if source has duplicates
				targetVars[key] = true
			}
		}
	}

	if len(newLines) > 0 {
		logger.Notice(ctx, "Adding variables to {{_File_}}%s{{|-|}}:", targetFile)
		for _, line := range newLines {
			logger.Notice(ctx, "   {{_Var_}}%s{{|-|}}", line)
		}

		// Append to target
		fTarget, err := os.OpenFile(targetFile, os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			return nil, err
		}
		defer fTarget.Close()

		writer := bufio.NewWriter(fTarget)
		// Check if target ends with newline, if not add one
		targetContent, _ := os.ReadFile(targetFile)
		if len(targetContent) > 0 && targetContent[len(targetContent)-1] != '\n' {
			writer.WriteString("\n")
		} else if len(targetContent) == 0 {
			// for new file
		} else {
			// already ends with newline, but maybe we want an extra gap?
			// Bash version does printf '\n' then lines
			writer.WriteString("\n")
		}

		for _, line := range newLines {
			writer.WriteString(line + "\n")
		}
		if err := writer.Flush(); err != nil {
			return nil, err
		}
	}

	return newLines, nil
}
