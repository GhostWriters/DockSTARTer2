package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// ViewString renders the menu content as a string (for compositing)
func (m *MenuModel) ViewString() string {
	if m.width == 0 {
		return ""
	}

	// Return cached view if the state hasn't changed since the last render
	if cachedView, valid := m.CheckCache(); valid {
		return cachedView
	}

	// In Sub-menu mode, we render a simpler view without the global backdrop logic
	if m.subMenuMode {
		return m.viewSubMenu()
	}

	if m.flowMode {
		return m.renderFlow()
	}

	styles := GetStyles()

	var listView string
	if m.variableHeight {
		listView = m.renderVariableHeightList()
	} else {
		listView = m.list.View()
		// Wrap with dialog background to eliminate black space
		listViewStyle := lipgloss.NewStyle().
			Background(styles.Dialog.GetBackground())
		if m.maximized {
			// Only force height when maximized — ensures list fills the full dialog.
			listViewStyle = listViewStyle.Height(m.list.Height())
		}
		listView = listViewStyle.Render(listView)
	}

	// Wrap list in its own border (no padding, items have their own margins).
	// Use rounded border when focused for higher visual fidelity.
	listStyle := styles.Dialog.
		Padding(0, 0)
	listStyle = ApplyInnerBorder(listStyle, m.focused, styles.LineCharacters)
	borderedList := listStyle.Render(listView)

	// Calculate widths from known dimensions, not measured content
	// For maximized: use m.width - outer border (2) for the content width
	// For non-maximized: use list width + inner padding (2) for consistency
	listWidth := m.list.Width()

	var outerContentWidth int
	if m.maximized {
		// Outer dialog fills available space: content = total - outer border
		outerContentWidth = m.width - 2
	} else {
		// Non-maximized: outer content = bordered list (listWidth + 2) + padding (2)
		outerContentWidth = listWidth + 2 + 2
	}

	// Inner components should fit within outerContentWidth - padding (2)
	innerWidth := outerContentWidth - 2
	// bordered list width = innerWidth, so list content = innerWidth - 2
	targetWidth := innerWidth

	// Render buttons to match the same bordered width
	// Account for the padding (2) that renderButtonBox will add
	buttonInnerWidth := targetWidth - 2
	buttonRow := m.renderSimpleButtons(buttonInnerWidth)
	borderedButtonBox := m.renderButtonBox(buttonRow, buttonInnerWidth)

	// Add equal margins around both boxes for spacing
	marginStyle := lipgloss.NewStyle().
		Background(styles.Dialog.GetBackground()).
		Padding(0, 1)

	paddedList := marginStyle.Render(borderedList)

	// Ensure button box has same width as list for proper vertical alignment
	paddedButtons := lipgloss.NewStyle().
		Background(styles.Dialog.GetBackground()).
		Width(outerContentWidth).
		Padding(0, 1).
		Render(borderedButtonBox)

	// Build inner content parts
	var innerParts []string

	// Add subtitle if present (always left-aligned)
	if m.subtitle != "" {
		subAlign := lipgloss.Left
		subtitleStyle := styles.Dialog.
			Width(outerContentWidth).
			Padding(0, 1).
			Align(subAlign).
			Border(lipgloss.Border{}) // Ensure no border on subtitle itself

		subStr := RenderThemeText(m.subtitle, subtitleStyle)
		subtitle := subtitleStyle.Render(subStr)
		innerParts = append(innerParts, subtitle)
	}

	// Add list box and button box with NO gaps to maximize list space
	// JoinVertical will stack them tightly
	innerParts = append(innerParts, paddedList)
	innerParts = append(innerParts, paddedButtons)

	// Combine all parts and standardize TrimRight to prevent implicit gaps
	for i, part := range innerParts {
		innerParts[i] = strings.TrimRight(part, "\n")
	}
	content := lipgloss.JoinVertical(lipgloss.Left, innerParts...)

	// Force total content height to match the calculated budget (Total - Outer Borders - Shadow)
	// only if maximized. Otherwise it should have its intrinsic height.
	if m.maximized {
		heightBudget := m.layout.Height - DialogBorderHeight - m.layout.ShadowHeight
		if heightBudget > 0 {
			content = lipgloss.NewStyle().
				Height(heightBudget).
				Background(styles.Dialog.GetBackground()).
				Render(content)
		}
	}
	// Determine target height for the outer dialog
	targetHeight := 0
	if m.maximized {
		targetHeight = m.height
	}

	// Wrap in bordered dialog with title embedded in border

	// Wrap in bordered dialog with title embedded in border
	// Use outerContentWidth - the known content width for the outer dialog
	var dialog string
	if m.title != "" {
		dialog = m.renderBorderWithTitle(content, outerContentWidth, targetHeight, m.focused, false)
	} else {
		// No title: use focus-aware inner rounded border
		dialogStyle := lipgloss.NewStyle().
			Background(styles.Dialog.GetBackground()).
			Padding(0, 1)
		dialogStyle = ApplyInnerBorder(dialogStyle, m.focused, styles.LineCharacters)
		if targetHeight > 0 {
			dialogStyle = dialogStyle.Height(targetHeight - DialogBorderHeight)
		}
		dialog = dialogStyle.Render(content)
	}

	// Save to cache before returning
	return m.SaveCache(dialog)
}

// Layers returns a single layer with the menu content for visual compositing
func (m *MenuModel) Layers() []*lipgloss.Layer {
	baseZ := ZScreen
	if m.isDialog {
		baseZ = ZDialog
	}
	return []*lipgloss.Layer{
		lipgloss.NewLayer(m.ViewString()).Z(baseZ).ID(m.id),
	}
}

// SetIsDialog sets whether the menu acts as a modal dialog vs an underlying screen
func (m *MenuModel) SetIsDialog(isDialog bool) {
	m.isDialog = isDialog
}

func (m *MenuModel) renderBorderWithTitle(content string, contentWidth int, targetHeight int, focused bool, rounded bool) string {
	align := GetActiveContext().DialogTitleAlign
	titleTag := "Theme_Title"
	if m.subMenuMode {
		align = GetActiveContext().SubmenuTitleAlign
		if focused {
			titleTag = "Theme_TitleSubMenuFocused"
		} else {
			titleTag = "Theme_TitleSubMenu"
		}
	}
	ctx := GetActiveContext()
	ctx.Type = m.dialogType
	return RenderBorderedBoxCtx(m.title, content, contentWidth, targetHeight, focused, rounded, align, titleTag, ctx)
}

func (m *MenuModel) viewSubMenu() string {
	styles := GetStyles()
	var content string
	if m.flowMode {
		content = m.renderFlow()
	} else {
		content = m.list.View()
	}

	// Inner content with maintained background
	inner := MaintainBackground(content, styles.Dialog)

	// Use Layout helpers for consistent border calculations
	layout := GetLayout()
	innerWidth, _ := layout.InnerContentSize(m.width, m.height)

	// When in flow mode, the height is strictly determined by the content + borders
	targetHeight := m.height
	if m.flowMode {
		_, targetHeight = layout.OuterTotalSize(0, m.layout.ViewportHeight)
	}
	return m.renderBorderWithTitle(inner, innerWidth, targetHeight, m.focusedSub, true)
}
