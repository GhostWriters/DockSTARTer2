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

	// Sections-based layout: stack sub-menus inside the outer border.
	if len(m.contentSections) > 0 {
		return m.viewWithSections()
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

	// Determine the target content width (the space inside the outer dialog borders)
	layout := GetLayout()
	contentWidth := m.GetInnerContentWidth()

	// Inner components (list and button row) should fit within contentWidth - padding (2)
	// Padding = 1 on each side (fixed margin in marginStyle below)
	innerBoxWidth := contentWidth - 2

	// Render buttons to match the exact same width as the list's border box
	buttonRow := m.renderSimpleButtons(innerBoxWidth)
	borderedButtonBox := m.renderButtonBox(buttonRow, innerBoxWidth)

	// Spacing style for both the list and the button box
	marginStyle := lipgloss.NewStyle().
		Background(styles.Dialog.GetBackground()).
		Padding(0, 1)

	paddedList := marginStyle.Render(borderedList)
	paddedButtons := marginStyle.Width(contentWidth).Render(borderedButtonBox)

	// Build inner content parts
	var innerParts []string

	// Add subtitle if present (always left-aligned)
	if m.subtitle != "" {
		subtitleStyle := styles.Dialog.
			Width(contentWidth).
			Padding(0, 1).
			Align(lipgloss.Left).
			Border(lipgloss.Border{})

		subStr := RenderThemeText(m.subtitle, subtitleStyle)
		innerParts = append(innerParts, subtitleStyle.Render(subStr))
	}

	innerParts = append(innerParts, paddedList)
	innerParts = append(innerParts, paddedButtons)

	// Combine all parts and standardize TrimRight to prevent implicit gaps
	for i, part := range innerParts {
		innerParts[i] = strings.TrimRight(part, "\n")
	}
	content := lipgloss.JoinVertical(lipgloss.Left, innerParts...)

	// Force total content height to match the calculated budget
	// only if maximized. Otherwise it should have its intrinsic height.
	if m.maximized {
		heightBudget := m.layout.Height - layout.BorderHeight() - m.layout.ShadowHeight
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
	var dialog string
	if m.title != "" {
		dialog = m.renderBorderWithTitle(content, contentWidth, targetHeight, m.focused, false, "Theme_Title")
	} else {
		// No title: use focus-aware inner rounded border
		// We must ensure the style width accounts for the layout's actual visual borders
		dialogStyle := lipgloss.NewStyle().
			Background(styles.Dialog.GetBackground()).
			Width(contentWidth + layout.BorderWidth())

		dialogStyle = ApplyInnerBorder(dialogStyle, m.focused, styles.LineCharacters)
		if targetHeight > 0 {
			dialogStyle = dialogStyle.Height(targetHeight)
		}
		dialog = dialogStyle.Render(content)
	}

	// Save to cache before returning
	return m.SaveCache(dialog)
}

// Layers returns a single layer with the menu content for visual compositing
func (m *MenuModel) Layers() []*lipgloss.Layer {
	return []*lipgloss.Layer{
		lipgloss.NewLayer(m.ViewString()).Z(ZScreen).ID(m.id),
	}
}

// SetIsDialog marks the menu as a modal dialog, raising its hit-region Z priority above screen regions
func (m *MenuModel) SetIsDialog(isDialog bool) {
	m.isDialog = isDialog
}

func (m *MenuModel) renderBorderWithTitle(content string, contentWidth int, targetHeight int, focused bool, rounded bool, titleTag string) string {
	align := GetActiveContext().DialogTitleAlign
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

func (s *MenuModel) viewSubMenu() string {
	styles := GetStyles()
	layout := GetLayout()

	// The target outer dimensions
	targetHeight := s.height
	contentWidth := s.width - layout.BorderWidth()

	// 1. Render Subtitle
	var subtitleView string
	if s.subtitle != "" {
		subtitleStyle := styles.Dialog.
			Width(contentWidth).
			Padding(0, 1). // matches internal padding
			Align(lipgloss.Left).
			Border(lipgloss.Border{})

		subStr := RenderThemeText(s.subtitle, subtitleStyle)
		subtitleView = subtitleStyle.Render(subStr)
	}

	// 2. Render List
	var content string
	if s.flowMode {
		content = s.renderFlow()
	} else {
		content = MaintainBackground(s.list.View(), styles.Dialog)
	}

	// 3. Render Buttons (if any)
	var buttonView string
	buttons := s.getButtonSpecs()
	if len(buttons) > 0 {
		useBorders := s.layout.ButtonHeight == DialogButtonHeight
		buttonView = renderCenteredButtonsImpl(contentWidth, useBorders, GetActiveContext(), buttons...)
	}

	// Combine all internal content vertically
	parts := []string{subtitleView, strings.TrimRight(content, "\n"), buttonView}
	var filteredParts []string
	for _, p := range parts {
		if p != "" {
			filteredParts = append(filteredParts, p)
		}
	}
	combined := lipgloss.JoinVertical(lipgloss.Left, filteredParts...)

	// 4. Render the bordered box with embedded title.
	// We pass 'true' for rounded so submenus use the rounded corner style.
	return s.renderBorderWithTitle(combined, contentWidth, targetHeight, s.focusedSub, true, "Theme_Title")
}

// viewWithSections renders an outer dialog that stacks content sections (sub-menus)
// vertically inside its border, followed by a standard button row.
// This path is taken when m.contentSections is non-empty and m.subMenuMode is false.
func (m *MenuModel) viewWithSections() string {
	layout := GetLayout()
	// Content width is the space inside the outer border.
	contentWidth := m.width - layout.BorderWidth()
	if contentWidth < 1 {
		contentWidth = 1
	}

	var parts []string

	// Stack sections directly — each section already renders its own bordered panel.
	for _, sec := range m.contentSections {
		v := strings.TrimRight(sec.ViewString(), "\n")
		if v != "" {
			parts = append(parts, v)
		}
	}

	// Button row at contentWidth (matches what renderSettingsDialog used to do).
	buttonRow := m.renderSimpleButtons(contentWidth)
	parts = append(parts, buttonRow)

	content := lipgloss.JoinVertical(lipgloss.Left, parts...)

	return m.SaveCache(
		m.renderBorderWithTitle(content, contentWidth, m.height, m.focused, false, "Theme_Title"),
	)
}
