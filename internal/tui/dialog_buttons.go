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
	Help   string // Help text for right-click context
}

// GetButtonHitRegions returns hit regions for a row of buttons.
// Use this when you need clickable buttons - it ensures hit regions
// GetButtonHitRegions returns a slice of HitRegions for a horizontal row of buttons.
// The provided hCtx is used as a template for the HelpContext of each button hit region,
// allowing for both screen-level and button-specific help to be displayed.
// Parameters:
//   - hCtx: template for HelpContext, providing ScreenName, PageTitle, PageText
//   - dialogID: prefix for zone IDs to disambiguate multiple dialogs (can be empty)
//   - offsetX, offsetY: position of the button row in the dialog
//   - contentWidth: same width passed to RenderCenteredButtons
//   - baseZ: z-order for the hit regions (typically ZDialog + 20)
//   - buttons: same button specs passed to RenderCenteredButtons
func GetButtonHitRegions(hCtx HelpContext, dialogID string, offsetX, offsetY, contentWidth, baseZ int, buttons ...ButtonSpec) []HitRegion {
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
	useBorders := buttonsFitWithBorders(contentWidth, ctx, buttons)
	var buttonWidth int
	if useBorders {
		buttonContentWidth := maxButtonWidth + 4
		sampleStyle := ctx.ButtonInactive.Width(buttonContentWidth).Align(lipgloss.Center)
		sampleStyle = ApplyInnerBorderCtx(sampleStyle, false, ctx)
		buttonWidth = lipgloss.Width(sampleStyle.Render(strings.Repeat("x", maxButtonWidth)))
	} else {
		buttonWidth = maxButtonWidth + 2 // "< label >" with interior padding
	}
	buttonHeight := 1
	if useBorders {
		buttonHeight = 3
	}

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
			Height: buttonHeight,
			ZOrder: baseZ,
			Label:  btn.Text,
		})
		// If button has help text, add rich HelpContext using hCtx as a template
		if btn.Help != "" {
			regions[len(regions)-1].Help = &HelpContext{
				ScreenName: hCtx.ScreenName,
				PageTitle:  hCtx.PageTitle,
				PageText:   hCtx.PageText,
				ItemTitle:  btn.Text + " Button",
				ItemText:   btn.Help,
			}
		}
	}

	return regions
}

// buttonsFitWithBorders returns true if bordered buttons fit within contentWidth
// and the button_borders setting is enabled.
// It renders a sample button to get the exact bordered width, matching the real render path.
func buttonsFitWithBorders(contentWidth int, ctx StyleContext, buttons []ButtonSpec) bool {
	if !ctx.ButtonBorders {
		return false
	}
	if len(buttons) == 0 {
		return true
	}
	maxButtonWidth := 0
	for _, btn := range buttons {
		if w := lipgloss.Width(btn.Text); w > maxButtonWidth {
			maxButtonWidth = w
		}
	}
	buttonContentWidth := maxButtonWidth + 4
	sampleStyle := ctx.ButtonInactive.Width(buttonContentWidth).Align(lipgloss.Center)
	sampleStyle = ApplyInnerBorderCtx(sampleStyle, false, ctx)
	buttonWidth := lipgloss.Width(sampleStyle.Render(strings.Repeat(" ", maxButtonWidth)))
	sectionWidth := contentWidth / len(buttons)
	return buttonWidth <= sectionWidth
}

// ButtonRowHeight returns the rendered height of a button row given constraints.
//
//   - contentWidth: available horizontal space; bordered buttons are dropped when too narrow.
//   - availableHeight: vertical rows the button row is allowed to occupy (0 = unconstrained).
//     If availableHeight is positive but less than DialogButtonHeight (3), flat buttons are
//     used because there is simply not enough room for the bordered box.
//
// Returns 3 (DialogButtonHeight) when both constraints allow it, 1 otherwise.
func ButtonRowHeight(contentWidth, availableHeight int, buttons ...ButtonSpec) int {
	if availableHeight > 0 && availableHeight < DialogButtonHeight {
		return 1
	}
	if buttonsFitWithBorders(contentWidth, GetActiveContext(), buttons) {
		return 3
	}
	return 1
}

// RenderCenteredButtons renders buttons centered in sections
func RenderCenteredButtons(contentWidth int, buttons ...ButtonSpec) string {
	return RenderCenteredButtonsCtx(contentWidth, GetActiveContext(), buttons...)
}

// RenderCenteredButtonsCtx renders buttons centered using a specific context.
// Borders are automatically dropped if the buttons don't fit within contentWidth.
func RenderCenteredButtonsCtx(contentWidth int, ctx StyleContext, buttons ...ButtonSpec) string {
	useBorders := buttonsFitWithBorders(contentWidth, ctx, buttons)
	return renderCenteredButtonsImpl(contentWidth, useBorders, ctx, buttons...)
}

// RenderCenteredButtonsExplicit renders buttons with an explicit border decision,
// bypassing the automatic width check. Use when the caller pre-computes border
// suitability from both width and height constraints (e.g. DisplayOptionsScreen).
func RenderCenteredButtonsExplicit(contentWidth int, useBorders bool, ctx StyleContext, buttons ...ButtonSpec) string {
	return renderCenteredButtonsImpl(contentWidth, useBorders, ctx, buttons...)
}

// renderCenteredButtonsImpl renders buttons with an explicit border decision.
// Use this when the caller has already determined whether borders should be shown
// (e.g. from a pre-computed layout.ButtonHeight), bypassing the width re-check.
func renderCenteredButtonsImpl(contentWidth int, useBorders bool, ctx StyleContext, buttons ...ButtonSpec) string {
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

	// buttonContentWidth: bordered buttons use maxWidth + 4 (2 spaces each side inside the box);
	// flat buttons use maxWidth (brackets are rendered separately in dialog color).
	buttonContentWidth := maxButtonWidth + 4
	if !useBorders {
		buttonContentWidth = maxButtonWidth
	}

	var renderedButtons []string
	for _, btn := range buttons {
		var buttonStyle lipgloss.Style
		if btn.Active {
			buttonStyle = ctx.ButtonActive
		} else {
			buttonStyle = ctx.ButtonInactive
		}

		buttonStyle = buttonStyle.Width(buttonContentWidth).Align(lipgloss.Center)
		if useBorders {
			buttonStyle = ApplyInnerBorderCtx(buttonStyle, btn.Active, ctx)
		}

		renderedLabel := RenderHotkeyLabelCtx(btn.Text, btn.Active, ctx)
		var rendered string
		if useBorders {
			// Border is the visual delimiter; center the hotkey-highlighted text inside the box.
			rendered = InjectBorderFlags(buttonStyle.Render(renderedLabel), ctx.BorderFlags, ctx.Border2Flags, true)
		} else {
			// Flat: pad label to maxButtonWidth so all buttons are the same inner width —
			// shorter labels get interior spaces (< Back >, < Exit >) while the longest
			// fills flush (<Select>). Brackets rendered in dialog color (colors only, no width),
			// text in button style with MaintainBackground to prevent ANSI bleed from hotkey rendering.
			bracketStyle := lipgloss.NewStyle().
				Foreground(ctx.Dialog.GetForeground()).
				Background(ctx.Dialog.GetBackground())
			pad := maxButtonWidth - lipgloss.Width(btn.Text)
			leftPad := pad / 2
			rightPad := pad - leftPad
			inner := strings.Repeat(" ", leftPad) + renderedLabel + strings.Repeat(" ", rightPad)
			// Pass a background-only style to MaintainBackground — the full buttonStyle has
			// Width+Align set, which causes getANSI() to capture leading padding spaces and
			// inject them after every reset in the text, producing visible extra spaces.
			bgStyle := lipgloss.NewStyle().Background(buttonStyle.GetBackground())
			buttonPart := MaintainBackground(buttonStyle.Render(inner), bgStyle)
			rendered = bracketStyle.Render("<") + buttonPart + bracketStyle.Render(">")
		}
		renderedButtons = append(renderedButtons, rendered)
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

// ButtonIDMatches reports whether a LayerHitMsg ID refers to a button with the given name.
// Handles both prefixed IDs (e.g. "confirm_dialog.Yes") and bare IDs (e.g. "Button.Yes").
func ButtonIDMatches(id, name string) bool {
	id = strings.ToLower(id)
	name = strings.ToLower(name)
	return strings.HasSuffix(id, "."+name) || id == "button."+name
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
