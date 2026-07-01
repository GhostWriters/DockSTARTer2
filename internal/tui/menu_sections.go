package tui

import (
	"DockSTARTer2/internal/tui/components/sinput"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
)

// AddContentSection appends a sub-menu (or ContentRow) as a stacked section
// rendered inside this menu's border, one section = one row. When sections
// are present the standard list is not rendered.
func (m *MenuModel) AddContentSection(section Content) {
	// Inherit isDialog so section hit regions use ZDialog base, staying above
	// the outer dialog's frame catch-all region (ZDialog-1).
	section.SetIsDialog(m.isDialog)
	m.contentSections = append(m.contentSections, section)
	// First section added gets focus; move focusedItem away from buttons.
	if len(m.contentSections) == 1 {
		m.focusedSection = 0
		m.focusedItem = FocusList
		m.updateSectionFocus()
	}
}

// AddContentRow groups the given items into a single horizontal row and adds
// that row as one section (i.e. multiple sections sharing one row of
// vertical space, side by side).
func (m *MenuModel) AddContentRow(items ...Content) {
	m.AddContentSection(NewContentRow(items...))
}

// updateSections routes a message to the focused content section and returns
// (model, cmd, handled). It handles Tab/Shift-Tab to cycle sections, and
// LayerHitMsg to focus the clicked section. Called from Update when
// contentSections is non-empty.
func (m *MenuModel) updateSections(msg tea.Msg) (tea.Model, tea.Cmd, bool) {
	n := len(m.contentSections)
	if n == 0 {
		return m, nil, false
	}

	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		if key.Matches(msg, Keys.CycleTab) {
			// A ContentRow contributes one Tab stop per child (Left/Right at
			// this level already means "jump to the button row," so a row
			// can't also claim it for intra-row navigation -- Tab must visit
			// each child individually instead).
			if m.focusedSection >= 0 && m.focusedSection < n && m.focusedItem == FocusList {
				if row, ok := m.contentSections[m.focusedSection].(*ContentRow); ok {
					if row.SubFocusIndex() < row.NumTabStops()-1 {
						row.SetSubFocusIndex(row.SubFocusIndex() + 1)
						cmd := m.updateSectionFocus()
						m.InvalidateCache()
						return m, cmd, true
					}
				}
			}
			next := m.focusedSection + 1
			if next >= n {
				m.focusedSection = -1
				m.focusedItem = FocusSelectBtn
			} else {
				m.focusedSection = next
				m.focusedItem = FocusList
				if row, ok := m.contentSections[next].(*ContentRow); ok {
					row.SetSubFocusIndex(0)
				}
			}
			cmd := m.updateSectionFocus()
			m.InvalidateCache()
			return m, cmd, true
		}
		if key.Matches(msg, Keys.CycleShiftTab) {
			if m.focusedSection >= 0 && m.focusedSection < n && m.focusedItem == FocusList {
				if row, ok := m.contentSections[m.focusedSection].(*ContentRow); ok {
					if row.SubFocusIndex() > 0 {
						row.SetSubFocusIndex(row.SubFocusIndex() - 1)
						cmd := m.updateSectionFocus()
						m.InvalidateCache()
						return m, cmd, true
					}
				}
			}
			if m.focusedSection == -1 {
				m.focusedSection = n - 1
				m.focusedItem = FocusList
			} else {
				m.focusedSection--
				if m.focusedSection < 0 {
					m.focusedSection = 0
				}
				m.focusedItem = FocusList
			}
			if row, ok := m.contentSections[m.focusedSection].(*ContentRow); ok {
				row.SetSubFocusIndex(row.NumTabStops() - 1)
			}
			cmd := m.updateSectionFocus()
			m.InvalidateCache()
			return m, cmd, true
		}
		// Route keyboard to focused section when a section has focus.
		// Left/Right navigate buttons while keeping section highlighted (dual focus).
		// Up/Down always go to the section even in dual-focus (button highlighted) mode.
		if m.focusedSection >= 0 && m.focusedSection < n {
			if m.focusedItem == FocusList {
				sec := m.contentSections[m.focusedSection]
				if (key.Matches(msg, Keys.Left) || key.Matches(msg, Keys.Right)) && !sec.WantsHorizontalKeys() {
					// Switch focus to buttons without losing section highlight.
					if key.Matches(msg, Keys.Right) {
						m.focusedItem = m.nextButtonFocus()
					} else {
						m.focusedItem = m.prevButtonFocus()
					}
					m.InvalidateCache()
					return m, nil, true
				}
				updated, cmd := m.contentSections[m.focusedSection].Update(msg)
				if sec, ok := updated.(Content); ok {
					m.contentSections[m.focusedSection] = sec
				}
				m.InvalidateCache()
				return m, cmd, true
			}
			// Dual-focus: button is highlighted but section still active.
			// Up/Down/Space route to section; Left/Right/Enter stay in button navigation (fall through).
			if m.focusedItem == FocusBtn && (key.Matches(msg, Keys.Up) || key.Matches(msg, Keys.Down) || key.Matches(msg, Keys.Space)) {
				updated, cmd := m.contentSections[m.focusedSection].Update(msg)
				if sec, ok := updated.(Content); ok {
					m.contentSections[m.focusedSection] = sec
				}
				m.InvalidateCache()
				return m, cmd, true
			}
		}
		return m, nil, false

	case LayerHitMsg:
		// Check if the hit belongs to one of our sections.
		// Item IDs are "item-{sectionID}-{index}", background is "{sectionID}",
		// list panel is "{sectionID}.list-panel" — all contain the section ID.
		for i, sec := range m.contentSections {
			if sec.MatchesID(msg.ID) {
				// For a ContentRow, resolve which child the click landed on
				// (updating its internal sub-focus) before calling
				// updateSectionFocus, so SetSubFocused propagates to the
				// correct child rather than whatever child previously had
				// sub-focus.
				if row, ok := sec.(*ContentRow); ok {
					for ci, item := range row.Items() {
						if item.MatchesID(msg.ID) {
							row.SetSubFocusIndex(ci)
							break
						}
					}
				}
				var focusCmd tea.Cmd
				if m.focusedSection != i {
					m.focusedSection = i
					m.focusedItem = FocusList
				}
				focusCmd = m.updateSectionFocus()
				updated, cmd := sec.Update(msg)
				if s, ok := updated.(Content); ok {
					m.contentSections[i] = s
				}
				m.InvalidateCache()
				return m, tea.Batch(focusCmd, cmd), true
			}
		}
		return m, nil, false

	case tea.MouseWheelMsg:
		if m.focusedSection >= 0 && m.focusedSection < n {
			// Send the raw MouseWheelMsg so the section's Scroll.Update handles it
			// (3-step scroll with pending throttle), not the 1-step LayerWheelMsg path.
			updated, cmd := m.contentSections[m.focusedSection].Update(msg)
			if s, ok := updated.(Content); ok {
				m.contentSections[m.focusedSection] = s
			}
			m.InvalidateCache()
			return m, cmd, true
		}
		return m, nil, false

	case ScrollDoneMsg:
		// Route to whichever section owns this scrollbar ID so Pending gets cleared.
		for i, sec := range m.contentSections {
			if sec.ScrollID() == msg.ID {
				updated, cmd := sec.Update(msg)
				if s, ok := updated.(Content); ok {
					m.contentSections[i] = s
				}
				return m, cmd, true
			}
		}
		return m, nil, false

	case ToggleFocusedMsg:
		if m.focusedSection >= 0 && m.focusedSection < n {
			updated, cmd := m.contentSections[m.focusedSection].Update(msg)
			if s, ok := updated.(Content); ok {
				m.contentSections[m.focusedSection] = s
			}
			m.InvalidateCache()
			return m, cmd, true
		}
		return m, nil, false

	case LayerWheelMsg:
		// Route wheel to whichever section owns the hit ID.
		for i, sec := range m.contentSections {
			if sec.MatchesID(msg.ID) {
				var focusCmd tea.Cmd
				if m.focusedSection != i {
					m.focusedSection = i
					m.focusedItem = FocusList
					focusCmd = m.updateSectionFocus()
				}
				_ = focusCmd
				updated, cmd := sec.Update(msg)
				if s, ok := updated.(Content); ok {
					m.contentSections[i] = s
				}
				m.InvalidateCache()
				return m, cmd, true
			}
		}
		return m, nil, false
	case sinput.CutMsg, sinput.PasteMsg, sinput.SelectAllMsg:
		// Forwarded clipboard messages from the context menu — route to focused section.
		if m.focusedSection >= 0 && m.focusedSection < n && m.focusedItem == FocusList {
			updated, cmd := m.contentSections[m.focusedSection].Update(msg)
			if sec, ok := updated.(Content); ok {
				m.contentSections[m.focusedSection] = sec
			}
			m.InvalidateCache()
			return m, cmd, true
		}
	}
	return m, nil, false
}

// updateSectionFocus propagates SetSubFocused to all sections based on focusedSection.
// Returns any cmd from the newly-focused section (e.g. sinput blink).
func (m *MenuModel) updateSectionFocus() tea.Cmd {
	var cmd tea.Cmd
	for i, sec := range m.contentSections {
		c := sec.SetSubFocused(m.focused && i == m.focusedSection)
		if m.focused && i == m.focusedSection {
			cmd = c
		}
	}
	return cmd
}

// ApplySectionFocus propagates the current focusedSection state to all sections via SetSubFocused.
// Call this after SetFocused when the outer menu owns section focus management.
func (m *MenuModel) ApplySectionFocus() {
	m.updateSectionFocus()
}

// GetContentSections returns the current content sections.
func (m *MenuModel) GetContentSections() []Content {
	return m.contentSections
}

// ReplaceSections replaces all content sections with the provided ones.
func (m *MenuModel) ReplaceSections(sections ...Content) {
	for _, sec := range sections {
		sec.SetIsDialog(m.isDialog)
	}
	m.contentSections = sections
	m.InvalidateCache()
}

// SetFlowMode toggles horizontal flow layout for this menu (used by sections that
// should size to their intrinsic height rather than filling available space).
func (m *MenuModel) SetFlowMode(flow bool) {
	m.flowMode = flow
}

func (m *MenuModel) SetFlowColumns(n int) {
	m.flowColumns = n
}

func (m *MenuModel) SetMaxFlowRows(n int) {
	m.maxFlowRows = n
}

// largeTitleBarMinRemaining is the minimum rows that must remain in a
// section-layout budget (after subtracting LargeTitleBarOverhead) for
// calculateSectionLayout's DecideLargeTitleBar call to choose large. Kept as
// a named constant, referenced by both calculateSectionLayout and
// LargeTitleBarBudget, so the two can never drift out of sync the way the
// web-display dialog's hand-rolled height budget once did.
const largeTitleBarMinRemaining = 3

// LargeTitleBarBudget returns the extra height (beyond whatever this menu's
// content sections and buttons already need) a caller must add when sizing
// this menu externally to guarantee calculateSectionLayout's own
// DecideLargeTitleBar check will choose a large title bar. Use this instead
// of hand-adding LargeTitleBarOverhead when pre-computing a height to pass
// into SetSize -- LargeTitleBarOverhead alone pays for the title bar's own
// rows. When this menu has at least one expandable (variableHeight) content
// section, DecideLargeTitleBar also requires largeTitleBarMinRemaining rows
// of slack beyond that to protect that section's breathing room; with no
// expandable sections there's nothing to protect, so calculateSectionLayout
// relaxes that requirement to 0 and this mirrors it -- see calculateSectionLayout.
func (m *MenuModel) LargeTitleBarBudget() int {
	for _, sec := range m.contentSections {
		if sec.IsVariableHeight() {
			return LargeTitleBarOverhead + largeTitleBarMinRemaining
		}
	}
	return LargeTitleBarOverhead
}

// SectionHeight returns how many rows this MenuModel would occupy as a
// content section at the given content width (i.e. width already inset for
// the outer section's own border/margin — same convention calculateSectionLayout
// uses when calling GetFlowHeight). Covers every fixed section "kind": flow-grid,
// contentRenderer-based (e.g. sinput), and plain/checklist. variableHeight
// (expandable) sections have no fixed answer; callers needing their height
// must account for that separately (same as calculateSectionLayout itself
// does via expandableCount).
func (m *MenuModel) SectionHeight(sectionWidth int) int {
	layout := GetLayout()
	switch {
	case m.flowMode:
		flowContentW := sectionWidth - layout.BorderWidth()
		if flowContentW < 1 {
			flowContentW = 1
		}
		flowH := m.GetFlowHeight(flowContentW)
		if m.maxFlowRows > 0 && flowH > m.maxFlowRows {
			flowH = m.maxFlowRows
		}
		return flowH + layout.BorderHeight()
	case m.contentRenderer != nil:
		return 1 + layout.BorderHeight()
	default:
		return len(m.items) + layout.BorderHeight()
	}
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

	// Pass 1: measure fixed sections (flow mode or contentRenderer = intrinsic height).
	sectionHeights := make([]int, len(m.contentSections))
	fixedTotal := 0
	expandableCount := 0
	for i, sec := range m.contentSections {
		if sec.IsVariableHeight() {
			expandableCount++
			continue
		}
		sectionH := sec.SectionHeight(sectionWidth)
		sectionHeights[i] = sectionH
		fixedTotal += sectionH
	}

	// Remaining height for expandable sections.
	// Allocate every single remaining row to avoid gaps.
	const minExpandable = 4

	// Large titlebar: drop before buttons if space is tight. minRemaining
	// protects an expandable section's breathing room; with no expandable
	// sections there's nothing to protect, so only the title bar's own
	// overhead needs to fit (same relaxation buttonThreshold applies above).
	titleBarMinRemaining := largeTitleBarMinRemaining
	if expandableCount == 0 {
		titleBarMinRemaining = 0
	}
	enabled := m.title != "" && currentConfig.UI.LargeTitleBars
	useLargeTitleBar, _ := DecideLargeTitleBar(enabled, innerHeight-fixedTotal-buttonBudget, titleBarMinRemaining)
	if useLargeTitleBar {
		innerHeight -= LargeTitleBarOverhead
	}

	remaining := innerHeight - fixedTotal - buttonBudget

	// Height-based button border fallback: drop to flat only when expandable
	// sections would have no room at all. Fixed-only dialogs use remaining < 0
	// since they have no expandable budget to protect.
	buttonThreshold := minExpandable
	if expandableCount == 0 {
		buttonThreshold = 0
	}
	if m.showButtons && buttonHeight == DialogButtonHeight && remaining < buttonThreshold {
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

	// Compute ButtonY: outer top border + large title overhead + all section heights.
	buttonY := layout.SingleBorder()
	if useLargeTitleBar {
		buttonY += LargeTitleBarOverhead
	}
	for _, sec := range m.contentSections {
		buttonY += sec.Height()
	}

	m.layout = DialogLayout{
		Width:         m.width,
		Height:        m.height,
		ButtonHeight:  buttonHeight,
		ButtonY:       buttonY,
		ShadowHeight:  shadowHeight,
		LargeTitleBar: useLargeTitleBar,
	}
}

// GetFocusedSection returns the index of the currently focused content section (-1 = buttons).
func (m *MenuModel) GetFocusedSection() int {
	return m.focusedSection
}

// SetFocusedSection sets focus to the given content section index without triggering tab navigation.
func (m *MenuModel) SetFocusedSection(idx int) {
	n := len(m.contentSections)
	if idx < 0 || idx >= n {
		return
	}
	m.focusedSection = idx
	m.focusedItem = FocusList
	for i, sec := range m.contentSections {
		sec.SetSubFocused(i == idx)
	}
	m.InvalidateCache()
}
