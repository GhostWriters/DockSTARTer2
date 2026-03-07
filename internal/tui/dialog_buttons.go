package tui

import (
	"strings"

	"DockSTARTer2/internal/strutil"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// RenderButton renders a button with the given label and focus state
func RenderButton(label string, focused bool) string {
	styles := GetStyles()

	style := styles.ButtonInactive
	if focused {
		style = styles.ButtonActive
	}

	renderedLabel := RenderHotkeyLabel(label, focused)
	return style.Render("<" + renderedLabel + ">")
}

// RenderHotkeyLabel styles the first letter of a label with the theme's hotkey color
func RenderHotkeyLabel(label string, focused bool) string {
	return RenderHotkeyLabelCtx(label, focused, GetActiveContext())
}

// RenderButtonRow renders a row of buttons centered
func RenderButtonRow(buttons ...string) string {
	return lipgloss.JoinHorizontal(lipgloss.Center, buttons...)
}

// ButtonSpec defines a button to render
type ButtonSpec struct {
	Text   string
	Active bool
	ZoneID string // Optional zone ID for mouse support (empty = no zone marking)
}

// GetButtonHitRegions returns hit regions for a row of buttons.
// Use this when you need clickable buttons - it ensures hit regions
// match the visual layout from RenderCenteredButtons.
// Parameters:
//   - dialogID: prefix for zone IDs to disambiguate multiple dialogs (can be empty)
//   - offsetX, offsetY: position of the button row in the dialog
//   - contentWidth: same width passed to RenderCenteredButtons
//   - zOrder: z-order for the hit regions (typically ZDialog + 20)
//   - buttons: same button specs passed to RenderCenteredButtons
func GetButtonHitRegions(dialogID string, offsetX, offsetY, contentWidth, zOrder int, buttons ...ButtonSpec) []HitRegion {
	if len(buttons) == 0 {
		return nil
	}

	// Measure the exact rendered button width by using the same render path
	// as RenderCenteredButtonsCtx, so hit regions match visually precisely.
	ctx := GetActiveContext()
	maxButtonWidth := 0
	for _, btn := range buttons {
		width := lipgloss.Width(btn.Text)
		if width > maxButtonWidth {
			maxButtonWidth = width
		}
	}
	buttonContentWidth := maxButtonWidth + 4
	sampleStyle := ctx.ButtonInactive.Width(buttonContentWidth).Align(lipgloss.Center)
	sampleStyle = ApplyInnerBorderCtx(sampleStyle, false, ctx)
	buttonWidth := lipgloss.Width(sampleStyle.Render(strings.Repeat("x", maxButtonWidth)))

	var regions []HitRegion
	numButtons := len(buttons)
	sectionWidth := contentWidth / numButtons

	for i, btn := range buttons {
		if btn.ZoneID == "" {
			continue
		}
		// Prepend dialog ID if provided for disambiguation
		id := btn.ZoneID
		if dialogID != "" {
			id = dialogID + "." + btn.ZoneID
		}

		// Calculate precise starting X for this section's button
		// Centering logic: (section_start) + (half of unused space in section)
		buttonX := offsetX + (i * sectionWidth) + (sectionWidth-buttonWidth)/2

		regions = append(regions, HitRegion{
			ID:     id,
			X:      buttonX,
			Y:      offsetY,
			Width:  buttonWidth,
			Height: 3, // Button border + text + border
			ZOrder: zOrder,
		})
	}

	return regions
}

// RenderCenteredButtons renders buttons centered in sections
func RenderCenteredButtons(contentWidth int, buttons ...ButtonSpec) string {
	return RenderCenteredButtonsCtx(contentWidth, GetActiveContext(), buttons...)
}

// RenderCenteredButtonsCtx renders buttons centered using a specific context
func RenderCenteredButtonsCtx(contentWidth int, ctx StyleContext, buttons ...ButtonSpec) string {
	if len(buttons) == 0 {
		return ""
	}

	// Find the maximum button text width
	maxButtonWidth := 0
	for _, btn := range buttons {
		width := lipgloss.Width(btn.Text)
		if width > maxButtonWidth {
			maxButtonWidth = width
		}
	}

	// Render each button with fixed width and rounded border
	buttonContentWidth := maxButtonWidth + 4
	var renderedButtons []string
	for _, btn := range buttons {
		var buttonStyle lipgloss.Style
		if btn.Active {
			buttonStyle = ctx.ButtonActive
		} else {
			buttonStyle = ctx.ButtonInactive
		}

		buttonStyle = buttonStyle.Width(buttonContentWidth).Align(lipgloss.Center)
		buttonStyle = ApplyInnerBorderCtx(buttonStyle, btn.Active, ctx)

		// RenderHotkeyLabel needs to handle focus too, but it uses GetStyles() inside.
		// Let's pass the context if we refactor it, or just use the existing one for now.
		// Actually, RenderHotkeyLabel should also be context-aware.
		renderedLabel := RenderHotkeyLabelCtx(btn.Text, btn.Active, ctx)
		renderedButtons = append(renderedButtons, buttonStyle.Render(renderedLabel))
	}

	numButtons := len(buttons)
	sectionWidth := contentWidth / numButtons

	var sections []string
	for _, btn := range renderedButtons {
		centeredBtn := lipgloss.NewStyle().
			Width(sectionWidth).
			Align(lipgloss.Center).
			Background(ctx.Dialog.GetBackground()).
			Render(btn)

		sections = append(sections, centeredBtn)
	}

	return lipgloss.JoinHorizontal(lipgloss.Top, sections...)
}

// RenderHotkeyLabelCtx styles the first letter of a label with the theme's hotkey color using a context
func RenderHotkeyLabelCtx(label string, focused bool, ctx StyleContext) string {
	trimmed := strings.TrimSpace(label)
	if len(trimmed) == 0 {
		return label
	}

	var charStyle, restStyle lipgloss.Style
	if focused {
		charStyle = ctx.TagKeySelected
		restStyle = ctx.ButtonActive
	} else {
		charStyle = ctx.TagKey
		restStyle = ctx.ButtonInactive
	}

	prefix := ""
	if strings.HasPrefix(label, " ") {
		prefix = strutil.Repeat(" ", len(label)-len(strings.TrimLeft(label, " ")))
	}

	firstChar := string(trimmed[0])
	rest := trimmed[1:]

	return prefix + charStyle.Render(firstChar) + restStyle.Render(rest)
}

// CheckButtonHotkeys checks if a key matches the first letter of any button.
// Returns button index and true if a match is found.
// NOTE: In Bubble Tea v2, KeyMsg is now a union type - use tea.KeyPressMsg for key press events.
func CheckButtonHotkeys(msg tea.KeyPressMsg, buttons []ButtonSpec) (int, bool) {
	// In v2, msg.Text contains the printable character(s)
	if msg.Text == "" {
		return -1, false
	}
	keyRune := strings.ToLower(msg.Text)

	for i, btn := range buttons {
		// Normalize button text (remove brackets/spaces)
		text := strings.TrimSpace(btn.Text)
		text = strings.Trim(text, "<>")
		if len(text) > 0 {
			firstChar := strings.ToLower(string(text[0]))
			if firstChar == keyRune {
				return i, true
			}
		}
	}
	return -1, false
}
