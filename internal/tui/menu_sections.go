package tui

// AddContentSection appends a sub-menu as a stacked section rendered inside this menu's border.
// When sections are present the standard list is not rendered.
func (m *MenuModel) AddContentSection(section *MenuModel) {
	m.contentSections = append(m.contentSections, section)
}

// SetFlowMode toggles horizontal flow layout for this menu (used by sections that
// should size to their intrinsic height rather than filling available space).
func (m *MenuModel) SetFlowMode(flow bool) {
	m.flowMode = flow
}

// calculateSectionLayout distributes available height among content sections.
// Fixed sections (flowMode) get their intrinsic height; the remaining height goes
// to expandable sections.  Called by calculateLayout when contentSections is set.
func (m *MenuModel) calculateSectionLayout() {
	layout := GetLayout()
	contentWidth := m.width - layout.BorderWidth()
	if contentWidth < 1 {
		contentWidth = 1
	}
	// Sections are inset by 1-char margin on each side (matching standard menu list padding).
	sectionWidth := contentWidth - layout.ContentMarginWidth()
	if sectionWidth < 1 {
		sectionWidth = 1
	}

	// Button height — width-based decision using the inset section width.
	buttonHeight := DialogButtonHeight
	buttonBudget := 0
	if m.showButtons {
		buttonHeight = ButtonRowHeight(sectionWidth, 0, m.getButtonSpecs()...)
		buttonBudget = buttonHeight
	}

	// Available height inside the outer border (subtract only borders).
	// Shadow space is handled by the outer renderer; we use all inner rows for content.
	innerHeight := m.height - layout.BorderHeight()

	// Pass 1: measure fixed sections (flow mode = intrinsic height).
	sectionHeights := make([]int, len(m.contentSections))
	fixedTotal := 0
	expandableCount := 0
	for i, sec := range m.contentSections {
		if sec.flowMode {
			flowH := sec.GetFlowHeight(sectionWidth)
			sectionH := flowH + layout.BorderHeight()
			sectionHeights[i] = sectionH
			fixedTotal += sectionH
		} else {
			expandableCount++
		}
	}

	// Remaining height for expandable sections.
	// Allocate every single remaining row to avoid gaps.
	const minExpandable = 4

	// Large titlebar: drop before buttons if space is tight.
	useLargeTitleBar := m.title != "" && currentConfig.UI.LargeTitleBars
	if useLargeTitleBar {
		tentativeRemaining := innerHeight - fixedTotal - buttonBudget - LargeTitleBarOverhead
		if tentativeRemaining < minExpandable {
			useLargeTitleBar = false // not enough room; titlebar stays small
		} else {
			innerHeight -= LargeTitleBarOverhead
		}
	}

	remaining := innerHeight - fixedTotal - buttonBudget

	// Height-based button border fallback: drop to flat only when expandable
	// sections would have no room at all.
	if m.showButtons && buttonHeight == DialogButtonHeight && remaining < minExpandable {
		buttonHeight = 1
		buttonBudget = 1
		remaining = innerHeight - fixedTotal - buttonBudget
	}
	if remaining < minExpandable {
		remaining = minExpandable
	}

	expandableH := 0
	remainder := 0
	if expandableCount > 0 {
		expandableH = remaining / expandableCount
		remainder = remaining % expandableCount
	}

	// Pass 2: size each section at the inset width.
	for i, sec := range m.contentSections {
		h := sectionHeights[i]
		if h == 0 {
			h = expandableH
			if remainder > 0 {
				h++
				remainder--
			}
		}
		sec.SetSize(sectionWidth, h)
	}

	shadowHeight := 0
	if currentConfig.UI.Shadow {
		shadowHeight = DialogShadowHeight
	}

	m.layout = DialogLayout{
		Width:         m.width,
		Height:        m.height,
		ButtonHeight:  buttonHeight,
		ShadowHeight:  shadowHeight,
		LargeTitleBar: useLargeTitleBar,
	}
}
