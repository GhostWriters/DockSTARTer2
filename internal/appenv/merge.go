package appenv

import (
	"DockSTARTer2/internal/logger"
	"bufio"
	"context"
	"os"
	"regexp"
)

// MergeNewOnly merges variables from source file to target file, adding only new ones.
func MergeNewOnly(ctx context.Context, targetFile, sourceFile string) ([]string, error) {
	var addedVars []string

	if _, err := os.Stat(sourceFile); os.IsNotExist(err) {
		logger.Warn(ctx, "File '{{_File_}}%s{{|-|}}' does not exist.", sourceFile)
		return nil, nil
	}

	if _, err := os.Stat(targetFile); os.IsNotExist(err) {
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
	var varsToLog []string
	var commentBuffer []string

	re := regexp.MustCompile(`^\s*([a-zA-Z_][a-zA-Z0-9_]*)=`)

	scanner := bufio.NewScanner(fSource)
	for scanner.Scan() {
		line := scanner.Text()
		matches := re.FindStringSubmatch(line)

		if matches != nil {
			key := matches[1]
			// Check if key exists in target
			if _, exists := targetVars[key]; !exists {
				newLines = append(newLines, commentBuffer...)
				newLines = append(newLines, line)
				varsToLog = append(varsToLog, line)
				addedVars = append(addedVars, key)
				targetVars[key] = "exists" // mark as exists for subsequent lines in same source
			}
			commentBuffer = nil
		} else {
			commentBuffer = append(commentBuffer, line)
		}
	}

	if len(newLines) > 0 {
		if len(varsToLog) > 0 {
			logger.Notice(ctx, "Adding variables to {{_File_}}%s{{|-|}}:", targetFile)
			for _, line := range varsToLog {
				logger.Notice(ctx, "\t{{_Var_}}%s{{|-|}}", line)
			}
		}

		fTarget, err := os.OpenFile(targetFile, os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			return nil, err
		}
		defer fTarget.Close()

		writer := bufio.NewWriter(fTarget)
		targetContent, _ := os.ReadFile(targetFile)
		if len(targetContent) > 0 && targetContent[len(targetContent)-1] != '\n' {
			writer.WriteString("\n")
		} else if len(targetContent) > 0 {
			writer.WriteString("\n")
		}

		for _, line := range newLines {
			writer.WriteString(line + "\n")
		}
		if err := writer.Flush(); err != nil {
			return nil, err
		}
	}

	return addedVars, nil
}
