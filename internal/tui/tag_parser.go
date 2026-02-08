package tui

import (
	"DockSTARTer2/internal/console"
	"regexp"

	"github.com/charmbracelet/lipgloss"
)

var (
	// Matches {{_Tag_}} OR {{|Code|}}
	// Group 1: Semantic Tag Name
	// Group 2: Direct Code
	tagRegex = regexp.MustCompile(`\{\{(?:_([A-Za-z0-9_]+)_|\|([A-Za-z0-9_:\-#]+)\|)\}\}`)
)

// ParseTitleTags extracts semantic tags from the start of the string
// and applies them to the base style. Returns the cleaned text and modified style.
func ParseTitleTags(text string, baseStyle lipgloss.Style) (string, lipgloss.Style) {
	// Find all tags
	matches := tagRegex.FindAllStringSubmatchIndex(text, -1)

	// Create a working style
	style := baseStyle

	lastIndex := 0

	// Direct tag regex for parsing resolved tags (e.g. {{|white:blue|}})
	directTagRegex := regexp.MustCompile(`\{\{\|([A-Za-z0-9_:\-#]+)\|\}\}`)

	// Iterate through all found tags
	for i, match := range matches {
		start, end := match[0], match[1]

		// On the first valid tag, reset the style to avoid inheriting properties
		// (like Bold/Underline) from the default baseStyle.
		if i == 0 {
			bg := baseStyle.GetBackground()
			style = lipgloss.NewStyle().Background(bg)
		}

		// Check for Semantic Tag (Group 1)
		if match[2] != -1 {
			tagName := text[match[2]:match[3]]
			def := console.GetColorDefinition(tagName)

			// The definition is now in {{|code|}} format
			subMatches := directTagRegex.FindAllStringSubmatch(def, -1)
			if len(subMatches) > 0 {
				// Apply each direct tag found in the definition sequentially
				for j, sm := range subMatches {
					// Special Handling: If the LAST tag in the definition is a REset {{|-|}},
					// and we have prior tags, we likely want to ignore this reset.
					if j == len(subMatches)-1 && len(subMatches) > 1 {
						if sm[1] == "-" {
							continue
						}
					}
					style = ApplyStyleCode(style, baseStyle, sm[1])
				}
			} else {
				// Fallback for simple definitions without brackets
				style = ApplyStyleCode(style, baseStyle, def)
			}

		} else if match[4] != -1 {
			// Check for Direct Code (Group 2)
			code := text[match[4]:match[5]]
			if code != "" {
				style = ApplyStyleCode(style, baseStyle, code)
			}
		}

		// Update lastIndex (logic remains same)
		if start == lastIndex {
			lastIndex = end
		}
	}

	// Remove processed tags and clean up
	cleanText := text[lastIndex:]
	// Strip remaining styling tags
	cleanText = tagRegex.ReplaceAllString(cleanText, "")
	cleanText = directTagRegex.ReplaceAllString(cleanText, "")

	return cleanText, style
}
