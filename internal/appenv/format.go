package appenv

import (
	"DockSTARTer2/internal/constants"
	"DockSTARTer2/internal/envutil"
	"DockSTARTer2/internal/paths"
	"context"
	"fmt"
	"github.com/GhostWriters/semstyle"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"DockSTARTer2/internal/strutil"
)

// Heading and tag strings used in formatted env output.
const (
	globalVarsHeading        = "Global Variables"
	appDeprecatedTag         = " [*DEPRECATED*]"
	appDisabledTag           = " (Disabled)"
	appUserDefinedTag        = " (User Defined)"
	appUserTemplateTag       = " (User Template)"
	appUserDefinedVarsTag    = " (User Defined Variables)"
	userDefinedGlobalVarsTag = " (User Defined)"
)

// FormatLinesCore processes environment variable lines to match DockSTARTer formatting.
// currentLines contains the staged variable values (already in memory).
// defaultLines contains the template lines (nil = no template).
// envLines, when non-nil, is scanned for APPNAME__ENABLED to determine the heading.
// When envLines is nil, composeEnvFile is read from disk instead (non-editor callers).
// For the global .env tab, pass currentLines as envLines.
// For the .env.app.appname tab, pass the global tab's staged lines as envLines.
// fileLabel, when non-empty, replaces the heading block's opening blank
// "###" separator with "### <fileLabel>" -- distinguishes a multi-service
// app's several .env.app.* files (e.g. ".env.app.immich-database") from
// each other, since they'd otherwise share an identical heading.
func FormatLinesCore(ctx context.Context, currentLines, defaultLines, envLines []string, appName, composeEnvFile string, fileLabel string) []string {
	appUpper := strings.ToUpper(appName)

	var formattedEnvLines []string

	// Resolve app status once — used by both the heading block and template inclusion.
	var appIsUserDefined, appEnabled bool
	var appDescription string
	if appUpper != "" {
		if envLines != nil {
			appIsUserDefined = IsAppUserDefinedFromLines(ctx, appUpper, envLines)
			appEnabled = IsAppEnabledFromLines(appUpper, envLines)
			appDescription = GetDescriptionFromLines(ctx, appUpper, envLines)
		} else {
			appIsUserDefined = IsAppUserDefined(ctx, appUpper, composeEnvFile)
			appEnabled = IsAppEnabled(appUpper, composeEnvFile)
			appDescription = GetDescription(ctx, appUpper, composeEnvFile)
		}
	}

	// fileLabel, when set, is a standalone "File: <path>" line placed above
	// whatever heading block follows (unconditionally -- the global .env
	// case has no app heading of its own, but still gets this line). The
	// zero time.Time means "not written yet" -- fileHeaderLines returns just
	// this one line; Update()'s actual disk-write paths call it again with
	// time.Now() (see stampLastWritten in update.go) to get a second,
	// aligned "Last written:" line to insert right after this one.
	if fileLabel != "" {
		formattedEnvLines = append(formattedEnvLines, fileHeaderLines(fileLabel, time.Time{})...)
		formattedEnvLines = append(formattedEnvLines, "")
	}

	// 2. Add App Heading if APPNAME is specified (Parity with env_format_lines.sh lines 31-56)
	if appUpper != "" {
		appNameNice := GetNiceName(ctx, appUpper)

		headingTitle := appNameNice
		if appIsUserDefined {
			headingTitle += appUserDefinedTag
		} else {
			if IsUserTemplate(appUpper) {
				headingTitle += appUserTemplateTag
			}
			if IsAppDeprecated(ctx, appUpper) {
				headingTitle += appDeprecatedTag
			}
			if !appEnabled {
				headingTitle += appDisabledTag
			}
		}

		// Parity lines 46-55: Adds ### wrapping including before/after descriptions.
		// Description is only shown for built-in apps (not user-defined).
		formattedEnvLines = append(formattedEnvLines, "###")
		formattedEnvLines = append(formattedEnvLines, "### "+headingTitle)
		formattedEnvLines = append(formattedEnvLines, "###")
		if appDescription != "" {
			descLines := strutil.WordWrapToSlice(semstyle.StripTags(appDescription), 75)
			for _, line := range descLines {
				trimmed := strings.TrimRight(line, " \r\t")
				if trimmed == "" {
					formattedEnvLines = append(formattedEnvLines, "###")
				} else {
					formattedEnvLines = append(formattedEnvLines, "### "+trimmed)
				}
			}
			formattedEnvLines = append(formattedEnvLines, "###")
		}

		// Template last-updated lines: only meaningful for a built-in app's
		// real template (not user-defined, which has no template folder).
		if !appIsUserDefined {
			if timestampLines := templateTimestampLines(appUpper); len(timestampLines) > 0 {
				formattedEnvLines = append(formattedEnvLines, timestampLines...)
				formattedEnvLines = append(formattedEnvLines, "###")
			}
		}
	}

	// 3. Add Template Contents Verbatim (Parity with env_format_lines.sh lines 57-64)
	// Skip the template when the app is user-defined — it has no built-in template structure.
	if defaultLines != nil && !appIsUserDefined {
		formattedEnvLines = append(formattedEnvLines, defaultLines...)
		if len(formattedEnvLines) > 0 {
			formattedEnvLines = append(formattedEnvLines, "")
		}
	}

	// 4. Index existing variables in formattedEnvLines (Parity lines 66-78)
	varRe := regexp.MustCompile(`^([A-Za-z0-9_]+)\s*=`)
	formattedEnvVarIndex := make(map[string]int)
	for i, line := range formattedEnvLines {
		matches := varRe.FindStringSubmatch(line)
		if len(matches) > 1 {
			formattedEnvVarIndex[matches[1]] = i
		}
	}

	// 5. Update values from currentLines (Parity lines 80-91)
	if len(currentLines) > 0 {
		consumed := make([]bool, len(currentLines))
		for i, line := range currentLines {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) > 1 {
				varName := strings.TrimSpace(parts[0])
				if idx, exists := formattedEnvVarIndex[varName]; exists {
					formattedEnvLines[idx] = line
					consumed[i] = true
				}
			}
		}

		// 6. Handle remaining currentLines (User Defined) (Parity lines 93-124)
		var remaining []string
		for i, line := range currentLines {
			if !consumed[i] {
				remaining = append(remaining, line)
			}
		}

		if len(remaining) > 0 {
			// Add User Defined heading (Parity lines 93-109)
			var appIsUserDefined2 bool
			if envLines != nil {
				appIsUserDefined2 = IsAppUserDefinedFromLines(ctx, appUpper, envLines)
			} else {
				appIsUserDefined2 = IsAppUserDefined(ctx, appUpper, composeEnvFile)
			}
			if appUpper == "" || !appIsUserDefined2 {
				headingTitle := ""
				if appUpper != "" {
					headingTitle = GetNiceName(ctx, appUpper) + appUserDefinedVarsTag
				} else {
					headingTitle = globalVarsHeading + userDefinedGlobalVarsTag
				}

				// Parity lines 102-108
				formattedEnvLines = append(formattedEnvLines, "###")
				formattedEnvLines = append(formattedEnvLines, "### "+headingTitle)
				formattedEnvLines = append(formattedEnvLines, "###")
			}

			// Add the remaining variables (Parity lines 111-122)
			for _, line := range remaining {
				parts := strings.SplitN(line, "=", 2)
				if len(parts) > 1 {
					varName := strings.TrimSpace(parts[0])
					// Parity line 116 check: update if exists (handle duplicates in currentLines)
					if idx, exists := formattedEnvVarIndex[varName]; exists {
						formattedEnvLines[idx] = line
					} else {
						// Variable is new, add it (Parity line 119)
						formattedEnvLines = append(formattedEnvLines, line)
						formattedEnvVarIndex[varName] = len(formattedEnvLines) - 1
					}
				}
			}
			// Parity line 123
			formattedEnvLines = append(formattedEnvLines, "")
		}
	} else {
		// Parity line 126 fallback
		formattedEnvLines = append(formattedEnvLines, "")
	}

	// Remove all trailing empty strings to avoid extra newlines (Parity with env_format_lines.sh)
	for len(formattedEnvLines) > 0 && formattedEnvLines[len(formattedEnvLines)-1] == "" {
		formattedEnvLines = formattedEnvLines[:len(formattedEnvLines)-1]
	}
	return formattedEnvLines
}

// templateTimestampLines builds the "### Template updated: ..." /
// "### Vars updated: ..." heading lines for appUpper (their labels padded
// to the same width so the timestamps line up in a column), or nil if
// there's nothing to report (no template folder, or -- for a repo-tracked
// app -- no git history for it).
func templateTimestampLines(appUpper string) []string {
	var templateUpdated, varsUpdated time.Time
	if IsUserTemplate(appUpper) {
		// A user override has no git history at all -- mtime on the files
		// themselves is the only available signal there.
		templateUpdated, varsUpdated = userTemplateTimestamps(TemplateFolder(appUpper))
	} else {
		templateUpdated, varsUpdated = paths.GetAppTemplateTimestamps(appUpper)
	}
	if templateUpdated.IsZero() {
		return nil
	}
	const timeFormat = "2006-01-02 15:04:05"
	pairs := [][2]string{{"Template updated:", templateUpdated.Format(timeFormat)}}
	if !varsUpdated.IsZero() {
		pairs = append(pairs, [2]string{"Vars updated:", varsUpdated.Format(timeFormat)})
	}
	return formatAlignedHeadingLines(pairs)
}

// fileHeaderLines builds the top-of-file "File: <fileLabel>" line and,
// when lastWritten is non-zero, an aligned "Last written: <timestamp>"
// line right after it. lastWritten's zero value means "not written yet" --
// used by FormatLinesCore (render/display, nothing saved yet) to get just
// the "File:" line; Update()'s actual disk-write paths pass time.Now() (see
// stampLastWritten) to get both, aligned against each other even though
// they're produced by different code at different times.
func fileHeaderLines(fileLabel string, lastWritten time.Time) []string {
	pairs := [][2]string{{"File:", fileLabel}}
	if lastWritten.IsZero() {
		pairs = append(pairs, [2]string{"Last written:", ""})
		return formatAlignedHeadingLines(pairs)[:1]
	}
	pairs = append(pairs, [2]string{"Last written:", lastWritten.Format("2006-01-02 15:04:05")})
	return formatAlignedHeadingLines(pairs)
}

// formatAlignedHeadingLines renders label/value pairs as "### <label> <value>"
// heading lines, padding every label to the width of the longest one so the
// values line up in a column regardless of label length.
func formatAlignedHeadingLines(pairs [][2]string) []string {
	maxLabel := 0
	for _, p := range pairs {
		if len(p[0]) > maxLabel {
			maxLabel = len(p[0])
		}
	}
	lines := make([]string, len(pairs))
	for i, p := range pairs {
		lines[i] = fmt.Sprintf("### %-*s%s", maxLabel+1, p[0], p[1])
	}
	return lines
}

// userTemplateTimestamps returns the most recent mtime among all files
// under dir (templateUpdated), and separately among just its var-bearing
// files (.env / .env.app.*, varsUpdated). Either is the zero Time if dir
// doesn't exist or has no matching files.
func userTemplateTimestamps(dir string) (templateUpdated, varsUpdated time.Time) {
	_ = filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		if info.ModTime().After(templateUpdated) {
			templateUpdated = info.ModTime()
		}
		name := d.Name()
		if name == constants.EnvFileName || strings.HasPrefix(name, constants.AppEnvFileNamePrefix) {
			if info.ModTime().After(varsUpdated) {
				varsUpdated = info.ModTime()
			}
		}
		return nil
	})
	return
}

// ReadDefaultLines loads the default/template lines for the given file path.
// Returns nil if the file is not found or cannot be read.
func ReadDefaultLines(defaultEnvFile string) []string {
	if defaultEnvFile == "" {
		return nil
	}
	if info, err := os.Stat(defaultEnvFile); err != nil || info.IsDir() {
		return nil
	}
	data, err := os.ReadFile(defaultEnvFile)
	if err != nil {
		return nil
	}
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// FormatLines processes environment variable lines to match DockSTARTer formatting.
// Matches env_format_lines.sh exactly. Reads files from disk and delegates to FormatLinesCore.
func FormatLines(ctx context.Context, currentEnvFile, defaultEnvFile, appName, composeEnvFile string, fileLabel string) ([]string, error) {
	var currentLines []string
	if currentEnvFile != "" {
		var err error
		currentLines, err = envutil.ReadLines(currentEnvFile)
		if err != nil {
			return nil, err
		}
	}
	defaultLines := ReadDefaultLines(defaultEnvFile)
	return FormatLinesCore(ctx, currentLines, defaultLines, nil, appName, composeEnvFile, fileLabel), nil
}

// GetReferencedApps returns a list of apps referenced in the compose env file.
func GetReferencedApps(composeEnvFile string) ([]string, error) {
	lines, err := envutil.ReadLines(composeEnvFile)
	if err != nil {
		return nil, err
	}

	appMap := make(map[string]bool)
	for _, line := range lines {
		varName := line
		if idx := strings.Index(line, "="); idx > 0 {
			varName = strings.TrimSpace(line[:idx])
		}
		appName := VarNameToAppName(varName)
		if appName != "" && IsAppNameValid(appName) {
			appMap[appName] = true
		}
	}

	var result []string
	for app := range appMap {
		result = append(result, app)
	}
	slices.Sort(result)
	return result, nil
}
