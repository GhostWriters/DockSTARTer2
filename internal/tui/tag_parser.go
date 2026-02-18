package tui

import (
	"DockSTARTer2/internal/console"

	"charm.land/lipgloss/v2"
)

// ParseTitleTags extracts semantic tags from the start of the string
// and applies them to the base style. Returns the cleaned text and modified style.
func ParseTitleTags(text string, baseStyle lipgloss.Style) (string, lipgloss.Style) {
	// Use the console package's unified regex for both semantic and direct tags
	tagRegex := console.GetDelimitedRegex()
	directRegex := console.GetDirectRegex()

	// Find all tags
	matches := tagRegex.FindAllStringSubmatchIndex(text, -1)

	// Create a working style
	style := baseStyle

	lastIndex := 0

	// Iterate through all found tags
	for i, match := range matches {
		start, end := match[0], match[1]

		// On the first valid tag, reset the style to avoid inheriting properties
		// (like Bold/Underline) from the default baseStyle.
		if i == 0 {
			bg := baseStyle.GetBackground()
			style = lipgloss.NewStyle().Background(bg)
		}

		// Get named group indices
		semanticIdx := tagRegex.SubexpIndex("semantic")
		directIdx := tagRegex.SubexpIndex("direct")

		// Check for Semantic Tag
		if semanticIdx > 0 && match[semanticIdx*2] != -1 {
			tagName := text[match[semanticIdx*2]:match[semanticIdx*2+1]]
			def := console.GetColorDefinition(tagName)

			// The definition is now in {{[code]}} format (direct tags)
			subMatches := directRegex.FindAllStringSubmatch(def, -1)
			if len(subMatches) > 0 {
				// Apply each direct tag found in the definition sequentially
				contentIdx := directRegex.SubexpIndex("content")
				for j, sm := range subMatches {
					if contentIdx >= len(sm) {
						continue
					}
					code := sm[contentIdx]
					// Special Handling: If the LAST tag in the definition is a Reset {{[-]}},
					// and we have prior tags, we likely want to ignore this reset.
					if j == len(subMatches)-1 && len(subMatches) > 1 {
						if code == "-" {
							continue
						}
					}
					style = ApplyStyleCode(style, baseStyle, code)
				}
			} else {
				// Fallback for simple definitions without brackets
				style = ApplyStyleCode(style, baseStyle, def)
			}

		} else if directIdx > 0 && match[directIdx*2] != -1 {
			// Check for Direct Code
			code := text[match[directIdx*2]:match[directIdx*2+1]]
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
	// Strip remaining styling tags using the console package
	cleanText = console.Strip(cleanText)

	return cleanText, style
}
