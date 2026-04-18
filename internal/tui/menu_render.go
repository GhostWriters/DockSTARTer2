package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// listScrollPercent returns the current scroll position in [0.0, 1.0] for the list.
func (m *MenuModel) listScrollPercent() float64 {
	var offset, total int
	if m.variableHeight {
		offset = m.viewStartY
		total = m.lastScrollTotal
	} else {
		offset = m.viewStartY
		total = len(m.items)
	}
	maxOff := total - m.layout.ViewportHeight
	if maxOff <= 0 {
		return 1.0
	}
	pct := float64(offset) / float64(maxOff)
	if pct > 1.0 {
		pct = 1.0
	}
	return pct
}

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
	ctx := GetActiveContext()

	// 1. Render core list content with scrollbar
	listView := m.renderVerticalListBlock(ctx)

	// 2. Wrap list in its own inner border (only for non-subMenu mode)
	// Full dialogs use a nested "border-in-border" look.
	listStyle := styles.Dialog.Padding(0, 0)
	listStyle = ApplyInnerBorder(listStyle, m.focused, ctx.LineCharacters)
	listStyle = listStyle.BorderBottom(false)
	borderedList := InjectBorderFlags(listStyle.Render(listView), styles.BorderFlags, styles.Border2Flags, false)
	totalWidth := m.list.Width() + ScrollbarGutterWidth + 2
	borderedList = strings.TrimSuffix(borderedList, "\n")

	// 3. Add bottom border (AE or Scroll Percent)
	showAEFocus := m.focused && !m.SelectedItem().IsSubItem && !m.SelectedItem().IsAddInstance && !m.SelectedItem().IsEditing
	if m.groupedMode {
		pct := -1.0
		if m.Scroll.Info.Needed {
			pct = m.listScrollPercent()
		}
		borderedList = borderedList + "\n" + BuildAEBottomBorder(totalWidth, 2, showAEFocus, m.activeColumn, pct, ctx)
	} else if m.Scroll.Info.Needed {
		borderedList = borderedList + "\n" + BuildScrollPercentBottomBorder(totalWidth, m.listScrollPercent(), m.focused, ctx)
	} else {
		borderedList = borderedList + "\n" + BuildPlainBottomBorder(totalWidth, m.focused, ctx)
	}

	// 4. Add AE top border
	if m.groupedMode {
		if nl := strings.Index(borderedList, "\n"); nl >= 0 {
			borderedList = BuildAETopBorder(totalWidth, 2, showAEFocus, m.activeColumn, ctx) + borderedList[nl:]
		}
	}

	// 5. Build Content Area
	layout := GetLayout()
	contentWidth := m.GetInnerContentWidth()
	innerBoxWidth := contentWidth - layout.ContentMarginWidth()

	buttonRow := m.renderSimpleButtons(innerBoxWidth)
	borderedButtonBox := m.renderButtonBox(buttonRow, innerBoxWidth)

	marginStyle := lipgloss.NewStyle().
		Background(styles.Dialog.GetBackground()).
		Padding(0, layout.ContentSideMargin)

	paddedList := marginStyle.Render(borderedList)
	paddedButtons := marginStyle.Width(contentWidth).Render(borderedButtonBox)

	var innerParts []string
	if m.subtitle != "" {
		subtitleStyle := styles.Dialog.
			Width(contentWidth).
			Padding(0, layout.ContentSideMargin).
			Align(lipgloss.Left).
			Border(lipgloss.Border{})

		subStr := RenderThemeText(m.subtitle, subtitleStyle)
		innerParts = append(innerParts, subtitleStyle.Render(subStr))
	}
	innerParts = append(innerParts, paddedList)
	innerParts = append(innerParts, paddedButtons)

	// Combine all parts
	content := lipgloss.JoinVertical(lipgloss.Left, innerParts...)

	// Force total content height to match the calculated budget
	// only if maximized. Otherwise it should have its intrinsic height.
	if m.maximized {
		heightBudget := m.layout.Height - layout.BorderHeight()
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
		dialog = m.renderBorderWithTitle(content, contentWidth, targetHeight, m.focused, false, "Title")
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
		dialog = InjectBorderFlags(dialogStyle.Render(content), styles.BorderFlags, styles.Border2Flags, true)
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
			titleTag = "TitleSubMenuFocused"
		} else {
			titleTag = "TitleSubMenu"
		}
	}

	ctx := GetActiveContext()
	ctx.Type = m.dialogType
	return RenderBorderedBoxCtx(m.title, content, contentWidth, targetHeight, focused, true, rounded, align, titleTag, ctx)
}

func (m *MenuModel) viewSubMenu() string {
	styles := GetStyles()
	layout := GetLayout()
	ctx := GetActiveContext()

	// The target outer dimensions
	contentWidth := m.width - layout.BorderWidth()

	// 1. Build inner content components
	var innerParts []string
	if m.subtitle != "" {
		subtitleStyle := styles.Dialog.
			Width(contentWidth).
			Padding(0, layout.ContentSideMargin).
			Align(lipgloss.Left).
			Border(lipgloss.Border{})

		subStr := RenderThemeText(m.subtitle, subtitleStyle)
		innerParts = append(innerParts, subtitleStyle.Render(subStr))
	}

	// Render core list with scrollbar (or flow layout if flowMode is set)
	if m.flowMode {
		innerParts = append(innerParts, m.renderFlowContent(contentWidth))
	} else {
		content := m.renderVerticalListBlock(ctx)
		innerParts = append(innerParts, content)
	}

	// Render buttons (if any)
	buttons := m.getButtonSpecs()
	if len(buttons) > 0 {
		useBorders := m.layout.ButtonHeight == DialogButtonHeight
		buttonView := renderCenteredButtonsImpl(contentWidth, useBorders, ctx, buttons...)
		innerParts = append(innerParts, buttonView)
	}

	combined := lipgloss.JoinVertical(lipgloss.Left, innerParts...)

	// 2. Wrap in bordered dialog
	targetHeight := lipgloss.Height(combined) + 2
	if m.maximized {
		targetHeight = m.height
	} else if targetHeight > m.height {
		targetHeight = m.height
	}
	result := m.renderBorderWithTitle(combined, contentWidth, targetHeight, m.focusedSub, true, "Title")

	// 3. Replace bottom border with scroll-percent indicator if needed
	if !m.flowMode && m.Scroll.Info.Needed {
		if lastNL := strings.LastIndex(result, "\n"); lastNL >= 0 {
			bottomLine := BuildScrollPercentBottomBorder(m.width, m.listScrollPercent(), m.focusedSub, ctx)
			result = result[:lastNL+1] + bottomLine
		}
	}
	return result
}

// renderVerticalListBlock renders the core list content and applies the scrollbar.
// This is the single source of truth for list viewport rendering.
func (m *MenuModel) renderVerticalListBlock(ctx StyleContext) string {
	content := m.renderVariableHeightList()

	// Ensure content is exactly ViewportHeight lines before applying the scrollbar,
	// so the gutter column spans the full border box.
	content = strings.TrimSuffix(content, "\n")
	h := strings.Count(content, "\n") + 1
	if h < m.layout.ViewportHeight {
		content += strings.Repeat("\n", m.layout.ViewportHeight-h)
	}

	total := len(m.items)
	if m.variableHeight {
		total = m.lastScrollTotal
	}
	return ApplyScrollbar(&m.Scroll, content, total, m.layout.ViewportHeight, m.viewStartY, ctx.LineCharacters, ctx)
}

// viewWithSections renders an outer dialog that stacks content sections (sub-menus)
// vertically inside its border, followed by a standard button row.
// This path is taken when m.contentSections is non-empty and m.subMenuMode is false.
func (m *MenuModel) viewWithSections() string {
	layout := GetLayout()
	styles := GetStyles()
	// Content width is the space inside the outer border.
	contentWidth := m.width - layout.BorderWidth()
	if contentWidth < 1 {
		contentWidth = 1
	}

	// Sections are rendered at the inset width; the margin brings them back to contentWidth.
	marginStyle := lipgloss.NewStyle().
		Background(styles.Dialog.GetBackground()).
		Padding(0, layout.ContentSideMargin)
	sectionWidth := contentWidth - layout.ContentMarginWidth()

	var parts []string

	// Stack sections with margin — each section already renders its own bordered panel.
	for _, sec := range m.contentSections {
		// Use the rendered string without trimming trailing newlines that are part of the height budget.
		v := sec.ViewString()
		if v != "" {
			parts = append(parts, marginStyle.Render(v))
		}
	}

	// Button row also inset by the same margin.
	buttonRowRaw := m.renderSimpleButtons(sectionWidth)
	if m.layout.ButtonHeight > 1 {
		buttonRowRaw = lipgloss.NewStyle().
			Height(m.layout.ButtonHeight).
			Align(lipgloss.Center, lipgloss.Center).
			Render(buttonRowRaw)
	}
	buttonRow := marginStyle.Render(buttonRowRaw)
	parts = append(parts, buttonRow)

	content := lipgloss.JoinVertical(lipgloss.Left, parts...)

	return m.SaveCache(
		m.renderBorderWithTitle(content, contentWidth, m.height, m.focused, false, "Title"),
	)
}
