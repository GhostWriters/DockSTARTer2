package screens

import (
	"fmt"
	"strings"

	"DockSTARTer2/internal/strutil"
)

// MenuHeadingParams holds data for FormatMenuHeading.
// Mirrors the bash menu_heading.sh argument semantics, but with typed fields
// instead of the bash :AppName / AppName: string conventions.
type MenuHeadingParams struct {
	// Application block (omitted when AppName is empty)
	AppName         string
	AppDescription  string // Word-wrapped to contentWidth - labelW
	AppIsDeprecated bool
	AppIsDisabled   bool
	AppIsUserDefined bool

	// File line (omitted when FilePath is empty)
	FilePath string

	// Variable line (omitted when VarName is empty)
	VarName         string
	VarIsUserDefined bool

	// Value lines (omitted when empty)
	OriginalValue string
	CurrentValue  string
}

// menuLabelW is the fixed label column width — the maximum across ALL possible
// labels ("Original Value: " = 16). Using a constant keeps values aligned at the
// same column regardless of which fields are shown, so headings remain visually
// stable when navigating between screens (matches bash menu_heading.sh behaviour).
const menuLabelW = 16

// FormatMenuHeading formats a context heading block for the F1 help panel and
// value-editing dialogs. contentWidth is the available display width, used to
// word-wrap AppDescription.
//
// Returns a string with {{|...|}} theme tags (not ANSI) — callers resolve colours.
//
// Colour cascade (matches bash menu_heading.sh behaviour):
// The highest-priority present field (CurrentValue > OriginalValue > VarName >
// FilePath > AppName) gets HeadingValue; all others get Heading.
func FormatMenuHeading(p MenuHeadingParams, contentWidth int) string {
	labelW := menuLabelW

	label := func(s string) string { return fmt.Sprintf("%*s", labelW, s) }
	indent := strings.Repeat(" ", labelW)

	// Determine which field is "primary" (HeadingValue).
	// Priority order matches bash: CurrentValue first, AppName last.
	primaryField := ""
	switch {
	case p.CurrentValue != "":
		primaryField = "CurrentValue"
	case p.OriginalValue != "":
		primaryField = "OriginalValue"
	case p.VarName != "":
		primaryField = "VarName"
	case p.FilePath != "":
		primaryField = "FilePath"
	default:
		primaryField = "AppName"
	}

	colorFor := func(field string) string {
		if field == primaryField {
			return "{{|HeadingValue|}}"
		}
		return "{{|Heading|}}"
	}

	var sb strings.Builder

	// Application block
	if p.AppName != "" {
		sb.WriteString(label("Application: "))
		sb.WriteString(colorFor("AppName"))
		sb.WriteString(p.AppName)
		sb.WriteString("{{[-]}}")
		if p.AppIsDeprecated {
			sb.WriteString(" {{|HeadingTag|}}[*DEPRECATED*]{{[-]}}")
		}
		if p.AppIsDisabled {
			sb.WriteString(" {{|HeadingTag|}}(Disabled){{[-]}}")
		}
		if p.AppIsUserDefined {
			sb.WriteString(" {{|HeadingTag|}}(User Defined){{[-]}}")
		}
		sb.WriteString("\n")

		if p.AppDescription != "" {
			valueW := contentWidth - labelW
			if valueW < 10 {
				valueW = 10
			}
			for _, dl := range strutil.WordWrapToSlice(p.AppDescription, valueW) {
				sb.WriteString(indent)
				sb.WriteString("{{|HeadingAppDescription|}}")
				sb.WriteString(dl)
				sb.WriteString("{{[-]}}\n")
			}
		}
		sb.WriteString("\n")
	}

	// File line
	if p.FilePath != "" {
		sb.WriteString(label("File: "))
		sb.WriteString(colorFor("FilePath"))
		sb.WriteString(p.FilePath)
		sb.WriteString("{{[-]}}\n")
	}

	// Variable line
	if p.VarName != "" {
		sb.WriteString(label("Variable: "))
		sb.WriteString(colorFor("VarName"))
		sb.WriteString(p.VarName)
		sb.WriteString("{{[-]}}")
		if p.VarIsUserDefined {
			sb.WriteString(" {{|HeadingTag|}}(User Defined){{[-]}}")
		}
		sb.WriteString("\n")
	}

	// Original Value
	if p.OriginalValue != "" {
		sb.WriteString("\n")
		sb.WriteString(label("Original Value: "))
		sb.WriteString(colorFor("OriginalValue"))
		sb.WriteString(p.OriginalValue)
		sb.WriteString("{{[-]}}\n")
	}

	// Current Value
	if p.CurrentValue != "" {
		sb.WriteString(label("Current Value: "))
		sb.WriteString(colorFor("CurrentValue"))
		sb.WriteString(p.CurrentValue)
		sb.WriteString("{{[-]}}\n")
	}

	return strings.TrimRight(sb.String(), "\n")
}
