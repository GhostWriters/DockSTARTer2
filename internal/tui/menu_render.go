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
		offset = m.list.Index()
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

	// Append the scrollbar/gutter column for all full-dialog menus (non-subMenu).
	// The slot is always reserved by calculateLayout, so this adds exactly one char —
	// space when off or not needed, track/thumb when on and scrollable.
	// Store the geometry in m.sbInfo so GetHitRegions can emit accurate hit regions.
	ctx := GetActiveContext()
	if !m.subMenuMode {
		enabled := currentConfig.UI.Scrollbar
		if m.variableHeight {
			listView, m.sbInfo = ApplyScrollbarColumnTracked(listView, m.lastScrollTotal, m.layout.ViewportHeight, m.viewStartY, enabled, ctx.LineCharacters, ctx)
		} else {
			listView, m.sbInfo = ApplyScrollbarColumnTracked(listView, len(m.items), m.layout.ViewportHeight, m.list.Index(), enabled, ctx.LineCharacters, ctx)
		}
	}

	// Wrap list in its own border (no padding, items have their own margins).
	// Disable the bottom so we can append a plain or scroll-indicator bottom border.
	listStyle := styles.Dialog.
		Padding(0, 0)
	listStyle = ApplyInnerBorder(listStyle, m.focused, styles.LineCharacters)
	listStyle = listStyle.BorderBottom(false)
	borderedList := InjectBorderFlags(listStyle.Render(listView), styles.BorderFlags, styles.Border2Flags, false)
	totalWidth := m.list.Width() + ScrollbarGutterWidth + 2
	borderedList = strings.TrimSuffix(borderedList, "\n")

	// AE borders only show focus markers when a top-level app is selected.
	// When navigating instances, the border markers are "unmarked" (unfocused).
	showAEFocus := m.focused && !m.SelectedItem().IsSubItem && !m.SelectedItem().IsAddInstance && !m.SelectedItem().IsEditing

	if m.groupedMode {
		borderedList = borderedList + "\n" + BuildAEBottomBorder(totalWidth, 2, showAEFocus, m.activeColumn, ctx)
	} else if m.sbInfo.Needed {
		borderedList = borderedList + "\n" + BuildScrollPercentBottomBorder(totalWidth, m.listScrollPercent(), m.focused, ctx)
	} else {
		borderedList = borderedList + "\n" + BuildPlainBottomBorder(totalWidth, m.focused, ctx)
	}

	// prefixDashes=2: corner(1)+dash(2)+dash(3)+A(4,5,6) center=5 = g0(1)+g1(2)+" ▣ "(3,4,5) center=5. MATCH.
	// AE top border (with individual column focus)
	if m.groupedMode {
		if nl := strings.Index(borderedList, "\n"); nl >= 0 {
			borderedList = BuildAETopBorder(totalWidth, 2, showAEFocus, m.activeColumn, ctx) + borderedList[nl:]
		}
	}

	// Determine the target content width (the space inside the outer dialog borders)
	layout := GetLayout()
	contentWidth := m.GetInnerContentWidth()

	// Inner components (list and button row) should fit within contentWidth minus the 1-char margin on each side.
	innerBoxWidth := contentWidth - layout.ContentMarginWidth()

	// Render buttons to match the exact same width as the list's border box
	buttonRow := m.renderSimpleButtons(innerBoxWidth)
	borderedButtonBox := m.renderButtonBox(buttonRow, innerBoxWidth)

	// Spacing style for both the list and the button box
	marginStyle := lipgloss.NewStyle().
		Background(styles.Dialog.GetBackground()).
		Padding(0, layout.ContentSideMargin)

	paddedList := marginStyle.Render(borderedList)
	paddedButtons := marginStyle.Width(contentWidth).Render(borderedButtonBox)

	// Build inner content parts
	var innerParts []string

	// Add subtitle if present (always left-aligned)
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
			Padding(0, layout.ContentSideMargin). // matches internal padding
			Align(lipgloss.Left).
			Border(lipgloss.Border{})

		subStr := RenderThemeText(s.subtitle, subtitleStyle)
		subtitleView = subtitleStyle.Render(subStr)
	}

	// 2. Render List
	ctx := GetActiveContext()
	var content string
	if s.flowMode {
		content = s.renderFlow()
	} else {
		content = MaintainBackground(s.list.View(), styles.Dialog)
		// Append scrollbar/gutter column (same slot reserved by calculateLayout).
		content, s.sbInfo = ApplyScrollbarColumnTracked(content, len(s.items), s.layout.ViewportHeight, s.list.Index(), currentConfig.UI.Scrollbar, ctx.LineCharacters, ctx)
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
	result := s.renderBorderWithTitle(combined, contentWidth, targetHeight, s.focusedSub, true, "Title")

	// Replace bottom border with scroll-percent variant when content overflows.
	if !s.flowMode && s.sbInfo.Needed {
		if lastNL := strings.LastIndex(result, "\n"); lastNL >= 0 {
			bottomLine := BuildScrollPercentBottomBorder(s.width, s.listScrollPercent(), s.focusedSub, ctx)
			result = result[:lastNL+1] + bottomLine
		}
	}
	return result
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
		v := strings.TrimRight(sec.ViewString(), "\n")
		if v != "" {
			parts = append(parts, marginStyle.Render(v))
		}
	}

	// Button row also inset by the same margin.
	buttonRow := marginStyle.Render(m.renderSimpleButtons(sectionWidth))
	parts = append(parts, buttonRow)

	content := lipgloss.JoinVertical(lipgloss.Left, parts...)

	return m.SaveCache(
		m.renderBorderWithTitle(content, contentWidth, m.height, m.focused, false, "Title"),
	)
}
