package tui

import (
	"strings"

	"DockSTARTer2/internal/tui/components/sinput"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
)

// AddContentSection appends a sub-menu as a stacked section rendered inside this menu's border.
// When sections are present the standard list is not rendered.
func (m *MenuModel) AddContentSection(section *MenuModel) {
	// Inherit isDialog so section hit regions use ZDialog base, staying above
	// the outer dialog's frame catch-all region (ZDialog-1).
	section.isDialog = m.isDialog
	m.contentSections = append(m.contentSections, section)
	// First section added gets focus; move focusedItem away from buttons.
	if len(m.contentSections) == 1 {
		m.focusedSection = 0
		m.focusedItem = FocusList
		m.updateSectionFocus()
	}
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
			next := m.focusedSection + 1
			if next >= n {
				m.focusedSection = -1
				m.focusedItem = FocusSelectBtn
			} else {
				m.focusedSection = next
				m.focusedItem = FocusList
			}
			cmd := m.updateSectionFocus()
			m.InvalidateCache()
			return m, cmd, true
		}
		if key.Matches(msg, Keys.CycleShiftTab) {
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
			cmd := m.updateSectionFocus()
			m.InvalidateCache()
			return m, cmd, true
		}
		// Route keyboard to focused section when a section has focus.
		// Left/Right navigate buttons while keeping section highlighted (dual focus).
		// Up/Down always go to the section even in dual-focus (button highlighted) mode.
		if m.focusedSection >= 0 && m.focusedSection < n {
			if m.focusedItem == FocusList {
				if key.Matches(msg, Keys.Left) || key.Matches(msg, Keys.Right) {
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
				if sec, ok := updated.(*MenuModel); ok {
					m.contentSections[m.focusedSection] = sec
				}
				m.InvalidateCache()
				return m, cmd, true
			}
			// Dual-focus: button is highlighted but section still active.
			// Up/Down/Space route to section; Left/Right/Enter stay in button navigation (fall through).
			if m.focusedItem == FocusBtn && (key.Matches(msg, Keys.Up) || key.Matches(msg, Keys.Down) || key.Matches(msg, Keys.Space)) {
				updated, cmd := m.contentSections[m.focusedSection].Update(msg)
				if sec, ok := updated.(*MenuModel); ok {
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
			if strings.Contains(msg.ID, sec.id) {
				var focusCmd tea.Cmd
				if m.focusedSection != i {
					m.focusedSection = i
					m.focusedItem = FocusList
					focusCmd = m.updateSectionFocus()
				}
				updated, cmd := sec.Update(msg)
				if s, ok := updated.(*MenuModel); ok {
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
			if s, ok := updated.(*MenuModel); ok {
				m.contentSections[m.focusedSection] = s
			}
			m.InvalidateCache()
			return m, cmd, true
		}
		return m, nil, false

	case ScrollDoneMsg:
		// Route to whichever section owns this scrollbar ID so Pending gets cleared.
		for i, sec := range m.contentSections {
			if sec.Scroll.ID == msg.ID {
				updated, cmd := sec.Update(msg)
				if s, ok := updated.(*MenuModel); ok {
					m.contentSections[i] = s
				}
				return m, cmd, true
			}
		}
		return m, nil, false

	case ToggleFocusedMsg:
		if m.focusedSection >= 0 && m.focusedSection < n {
			updated, cmd := m.contentSections[m.focusedSection].Update(msg)
			if s, ok := updated.(*MenuModel); ok {
				m.contentSections[m.focusedSection] = s
			}
			m.InvalidateCache()
			return m, cmd, true
		}
		return m, nil, false

	case LayerWheelMsg:
		// Route wheel to whichever section owns the hit ID.
		for i, sec := range m.contentSections {
			if strings.Contains(msg.ID, sec.id) {
				var focusCmd tea.Cmd
				if m.focusedSection != i {
					m.focusedSection = i
					m.focusedItem = FocusList
					focusCmd = m.updateSectionFocus()
				}
				_ = focusCmd
				updated, cmd := sec.Update(msg)
				if s, ok := updated.(*MenuModel); ok {
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
			if sec, ok := updated.(*MenuModel); ok {
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
func (m *MenuModel) GetContentSections() []*MenuModel {
	return m.contentSections
}

// ReplaceSections replaces all content sections with the provided ones.
func (m *MenuModel) ReplaceSections(sections ...*MenuModel) {
	for _, sec := range sections {
		sec.isDialog = m.isDialog
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
		if sec.flowMode {
			// The section renders its flow content at (sectionWidth - BorderWidth),
			// because viewSubMenu subtracts the border from m.width to get contentWidth.
			// Pass that same inner width to GetFlowHeight so the line count matches.
			flowContentW := sectionWidth - layout.BorderWidth()
			if flowContentW < 1 {
				flowContentW = 1
			}
			flowH := sec.GetFlowHeight(flowContentW)
			if sec.maxFlowRows > 0 && flowH > sec.maxFlowRows {
				flowH = sec.maxFlowRows
			}
			sectionH := flowH + layout.BorderHeight()
			sectionHeights[i] = sectionH
			fixedTotal += sectionH
		} else if sec.contentRenderer != nil {
			// contentRenderer sections render exactly 1 content line + 2 border rows.
			sectionH := 1 + layout.BorderHeight()
			sectionHeights[i] = sectionH
			fixedTotal += sectionH
		} else if !sec.variableHeight {
			// Fixed list: item count + 2 border rows.
			sectionH := len(sec.items) + layout.BorderHeight()
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
		if tentativeRemaining < 0 {
			useLargeTitleBar = false // not enough room; titlebar stays small
		} else {
			innerHeight -= LargeTitleBarOverhead
		}
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
		buttonY += sec.height
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
