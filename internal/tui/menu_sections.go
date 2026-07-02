package tui

import (
	"DockSTARTer2/internal/tui/components/sinput"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// AddContentSection appends a sub-menu (or ContentRow) as a stacked section
// rendered inside this menu's border, one section = one row. When sections
// are present the standard list is not rendered.
func (m *MenuModel) AddContentSection(section Content) {
	// Inherit isDialog so section hit regions use ZDialog base, staying above
	// the outer dialog's frame catch-all region (ZDialog-1).
	section.SetIsDialog(m.isDialog)
	// Determine whether any section added before this one was already
	// focusable, so we only claim focus for the first focusable section
	// overall (e.g. a leading non-focusable plain-text subtitle section is
	// skipped, and the list added after it becomes the initial focus). Before
	// any focusable section exists, focus defaults to the buttons -- a
	// dialog whose sections are ALL non-focusable (a pure information box)
	// is a legitimate shape, and must land on its buttons, not get stuck on
	// a non-interactive section.
	hadFocusableSection := false
	for _, sec := range m.contentSections {
		if sec.Focusable() {
			hadFocusableSection = true
			break
		}
	}
	if len(m.contentSections) == 0 {
		m.focusedSection = -1
		m.focusedItem = FocusSelectBtn
	}
	m.contentSections = append(m.contentSections, section)
	if !hadFocusableSection && section.Focusable() {
		m.focusedSection = len(m.contentSections) - 1
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

// updateSection routes msg to sec (the section at index i), then -- if sec
// wasn't processing before but is now (e.g. its own item Action click just
// started) -- marks this outer menu's own first button spinning too. This
// mirrors the pre-Content-migration behavior where a single MenuModel's
// button row and its item spinner were the same object; once split into an
// outer container + inner section, nothing else tells the outer's visible
// Select-role button to react to an inner section's item click.
func (m *MenuModel) updateSection(i int, msg tea.Msg) tea.Cmd {
	sec := m.contentSections[i]
	wasProcessing := sec.IsProcessing()
	updated, cmd := sec.Update(msg)
	if s, ok := updated.(Content); ok {
		sec = s
		m.contentSections[i] = s
	}
	if !wasProcessing && sec.IsProcessing() && len(m.buttons) > 0 {
		m.btnRow.MarkProcessing(m.buttons[0].ZoneID)
	}
	return cmd
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

	// Every content section must see its own deferred-action messages
	// (button-row clicks, list-item Action clicks scheduled via deferAction)
	// regardless of which section currently has focus -- a section's
	// deferred menuDeferredActionMsg/buttonRowDeferredActionMsg is scoped to
	// its own instanceID and arrives one tick after the click, by which
	// point normal message routing may no longer reach it (e.g. it's not
	// the focused section, or the message type isn't one updateSections'
	// switch below has a case for at all). Without this, a section's click
	// is silently dropped -- the same bug class fixed earlier this session
	// via AbsorbMessage for DisplayOptionsScreen/ServerOptionsScreen, but
	// those screens wire it manually in their own custom Update(); a plain
	// *MenuModel container (no custom screen wrapper) has nowhere else to
	// call it from, so updateSections must do it unconditionally itself.
	for _, sec := range m.contentSections {
		if cmd := sec.AbsorbMessage(msg); cmd != nil {
			m.InvalidateCache()
			return m, cmd, true
		}
	}

	anyFocusable := false
	for _, sec := range m.contentSections {
		if sec.Focusable() {
			anyFocusable = true
			break
		}
	}

	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		if key.Matches(msg, Keys.CycleTab) && !anyFocusable {
			// Pure information box (no focusable section, e.g. all plain-text)
			// -- Tab has nothing to cycle to but the buttons, which already
			// have focus by construction (see AddContentSection); consume
			// the key without changing state.
			return m, nil, true
		}
		if key.Matches(msg, Keys.CycleShiftTab) && !anyFocusable {
			return m, nil, true
		}
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
			for next < n && !m.contentSections[next].Focusable() {
				next++
			}
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
			prev := m.focusedSection - 1
			if m.focusedSection == -1 {
				prev = n - 1
			}
			for prev >= 0 && !m.contentSections[prev].Focusable() {
				prev--
			}
			if prev < 0 {
				// No focusable section before this point -- clamp forward to
				// the first focusable section instead (mirrors the pre-skip
				// logic's clamp-to-0 behavior for the flat-index case).
				prev = 0
				for prev < n && !m.contentSections[prev].Focusable() {
					prev++
				}
			}
			m.focusedSection = prev
			m.focusedItem = FocusList
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
				cmd := m.updateSection(m.focusedSection, msg)
				m.InvalidateCache()
				return m, cmd, true
			}
			// Dual-focus: button is highlighted but section still active.
			// Up/Down/Space route to section; Left/Right/Enter stay in button navigation (fall through).
			if m.focusedItem == FocusBtn && (key.Matches(msg, Keys.Up) || key.Matches(msg, Keys.Down) || key.Matches(msg, Keys.Space)) {
				cmd := m.updateSection(m.focusedSection, msg)
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
				cmd := m.updateSection(i, msg)
				m.InvalidateCache()
				return m, tea.Batch(focusCmd, cmd), true
			}
		}
		return m, nil, false

	case tea.MouseWheelMsg:
		if m.focusedSection >= 0 && m.focusedSection < n {
			// Send the raw MouseWheelMsg so the section's Scroll.Update handles it
			// (3-step scroll with pending throttle), not the 1-step LayerWheelMsg path.
			cmd := m.updateSection(m.focusedSection, msg)
			m.InvalidateCache()
			return m, cmd, true
		}
		return m, nil, false

	case ScrollDoneMsg:
		// Route to whichever section owns this scrollbar ID so Pending gets cleared.
		for i, sec := range m.contentSections {
			if sec.ScrollID() == msg.ID {
				cmd := m.updateSection(i, msg)
				return m, cmd, true
			}
		}
		return m, nil, false

	case DragDoneMsg:
		// Route to whichever section owns this scrollbar ID so DragPending
		// gets cleared -- without this, a section's scrollbar thumb drag
		// gets stuck after the first motion event: the first MouseMotionMsg
		// sets DragPending=true and schedules a DragDoneMsg to clear it one
		// render cycle later, but with contentSections non-empty nothing
		// routed that message back to the section, so DragPending never
		// resets and every subsequent motion event is silently gated off by
		// the "if !s.Drag.DragPending" guard in Scroll.Update.
		for i, sec := range m.contentSections {
			if sec.ScrollID() == msg.ID {
				cmd := m.updateSection(i, msg)
				return m, cmd, true
			}
		}
		return m, nil, false

	case ToggleFocusedMsg:
		if m.focusedSection >= 0 && m.focusedSection < n {
			cmd := m.updateSection(m.focusedSection, msg)
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
				cmd := m.updateSection(i, msg)
				m.InvalidateCache()
				return m, cmd, true
			}
		}
		return m, nil, false
	case sinput.CutMsg, sinput.PasteMsg, sinput.SelectAllMsg:
		// Forwarded clipboard messages from the context menu — route to focused section.
		if m.focusedSection >= 0 && m.focusedSection < n && m.focusedItem == FocusList {
			cmd := m.updateSection(m.focusedSection, msg)
			m.InvalidateCache()
			return m, cmd, true
		}

	case tea.MouseMotionMsg, tea.MouseReleaseMsg:
		// Scrollbar thumb drag continuation/release: a drag started on the
		// focused section's scrollbar (via its own LayerHitMsg ".sb.thumb"
		// handling) needs these follow-up events routed back to that same
		// section regardless of the mouse's current on-screen position --
		// the top-level (non-sectioned) MenuModel.Update handles this
		// unconditionally at the very top for its own Scroll (menu_update.go),
		// but a section's Scroll is a different object nested one level
		// down, and nothing else forwards these message types to it once
		// contentSections is non-empty.
		if m.focusedSection >= 0 && m.focusedSection < n {
			cmd := m.updateSection(m.focusedSection, msg)
			m.InvalidateCache()
			return m, cmd, true
		}
	}

	// Catch-all: forward otherwise-unhandled message types to the focused
	// section only if it opted in via WantsAllMessages -- e.g. a streaming
	// viewport section needs raw key messages (PgUp/PgDn, etc.) and internal
	// bubbles/viewport messages that none of the explicit cases above match.
	// Every other section kind does not opt in, so this changes no existing
	// behavior.
	if m.focusedSection >= 0 && m.focusedSection < n && m.contentSections[m.focusedSection].WantsAllMessages() {
		cmd := m.updateSection(m.focusedSection, msg)
		m.InvalidateCache()
		return m, cmd, true
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
	case m.sectionHeightOverride != nil:
		// Caller-supplied fixed-height formula, checked before the generic
		// contentRenderer "1 + BorderHeight()" default -- e.g. a header
		// section whose height depends on dynamic subtitle/task-list/
		// progress-bar content, not a single fixed line.
		return m.sectionHeightOverride(sectionWidth)
	case m.plainText != "":
		// Borderless kind -- no BorderHeight contribution, unlike every other
		// case here. Measure the actual rendered line count (word-wrap aware)
		// rather than assuming 1, matching how the plain-list subtitle height
		// is measured in menu_update.go's calculateLayout. Measured at
		// sectionWidth directly (not via m.width) since SectionHeight is
		// called before this section's own SetSize has assigned m.width.
		return lipgloss.Height(m.renderPlainText(sectionWidth))
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

// SectionNaturalWidth returns how wide this section would naturally like to
// be within maxWidth. The plain-list kind measures from item Tag/Desc text
// (mirroring calculateMaxTagAndDescLength's use in the plain-list
// calculateLayout path, menu_update.go); the plain-text kind measures its
// own rendered text width (word-wrap-free at natural length, so a short
// subtitle doesn't force the whole dialog to maxWidth); flow-grid and
// sinput have no narrower natural width than whatever they're given, so
// they simply return maxWidth.
func (m *MenuModel) SectionNaturalWidth(maxWidth int) int {
	if m.plainText != "" {
		natural := lipgloss.Width(RenderThemeText(m.plainTextThemeTag + m.plainText))
		if natural > maxWidth {
			natural = maxWidth
		}
		return natural
	}
	if m.flowMode || m.contentRenderer != nil {
		return maxWidth
	}
	layout := GetLayout()
	maxTagLen, maxDescLen := calculateMaxTagAndDescLength(m.items)
	// Width = tag + spacing(2) + desc + margins(2) + buffer(4), same formula
	// as menu_update.go's calculateLayout -- plus the checkbox glyph prefix
	// (e.g. "[ ] ") when in checkbox mode, which the render path
	// (menu_render_list.go's menuPrefixWidth) adds but this formula's
	// tag/desc-only measurement doesn't otherwise account for.
	natural := maxTagLen + 2 + maxDescLen + 2 + 4
	if m.checkboxMode {
		natural += layout.CheckboxWidth()
	}
	if natural > maxWidth {
		natural = maxWidth
	}
	return natural
}

// calculateSectionLayout distributes available height among content sections.
// Fixed sections (flowMode) get their intrinsic height; the remaining height goes
// to expandable sections.  Called by calculateLayout when contentSections is set.
func (m *MenuModel) calculateSectionLayout() {
	layout := GetLayout()

	// Non-maximized dialogs shrink to their natural content width, the same
	// way calculateSectionLayout already shrinks to natural content height
	// below -- mirroring the plain-list path's own maximized check
	// (menu_update.go's calculateLayout). Take the max natural width across
	// all sections (the widest content dictates the dialog's width) at the
	// given max available width, then re-derive contentWidth/sectionWidth
	// from the (possibly shrunk) m.width below as normal.
	if !m.maximized {
		// Matches the plain-list path's own minWidth (menu_update.go's
		// calculateLayout) so a sectioned dialog's minimum width is
		// consistent with the un-sectioned convention it's replacing.
		const minSectionWidth = 34
		maxAvailable := m.width - layout.BorderWidth() - layout.ContentMarginWidth()
		if maxAvailable < 1 {
			maxAvailable = 1
		}
		naturalWidth := minSectionWidth
		for _, sec := range m.contentSections {
			w := sec.SectionNaturalWidth(maxAvailable)
			if w > naturalWidth {
				naturalWidth = w
			}
		}
		if naturalWidth > maxAvailable {
			naturalWidth = maxAvailable
		}
		shrunkWidth := naturalWidth + layout.BorderWidth() + layout.ContentMarginWidth()
		if shrunkWidth < m.width {
			m.width = shrunkWidth
		}
	}

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
	// 0 (not DialogButtonHeight) when buttons are hidden, so viewWithSections
	// doesn't reserve/center an empty button row and DecideLargeTitleBar's
	// budget isn't short-changed by phantom button space.
	buttonHeight := 0
	buttonBudget := 0
	if m.showButtons {
		buttonHeight = ButtonRowHeight(sectionWidth, 0, m.getButtonSpecs()...)
		buttonBudget = buttonHeight
	}

	// Available height inside the outer border (subtract only borders).
	// Shadow space is handled by the outer renderer; we use all inner rows for content.
	innerHeight := m.height - layout.BorderHeight()

	// Pass 1: measure fixed sections (flow mode or contentRenderer = intrinsic height).
	// Expandable sections' natural height is also measured (via the same
	// SectionHeight -- e.g. a plain list's len(items)+BorderHeight formula
	// works regardless of IsVariableHeight) but kept separate from fixedTotal:
	// it's only used below to compute a non-maximized dialog's natural total
	// height, never added to the fixed budget an expandable section would
	// otherwise be excluded from filling.
	sectionHeights := make([]int, len(m.contentSections))
	fixedTotal := 0
	expandableCount := 0
	expandableNaturalTotal := 0
	for i, sec := range m.contentSections {
		if sec.IsVariableHeight() {
			expandableCount++
			expandableNaturalTotal += sec.SectionHeight(sectionWidth)
			continue
		}
		sectionH := sec.SectionHeight(sectionWidth)
		sectionHeights[i] = sectionH
		fixedTotal += sectionH
	}

	// Non-maximized dialogs shrink to their natural content height instead
	// of filling whatever height AppModel handed them -- mirroring the
	// plain-list path's own maximized check (menu_update.go's calculateLayout,
	// "listHeight = totalItemHeight; if ... || m.maximized { shrink }"). Done
	// once here, generically, so every future non-maximized sectioned dialog
	// gets this for free instead of re-deriving its own natural-height clamp
	// per dialog (the exact duplication that caused the Browser Settings
	// large-title-bar/blank-space bug fixed earlier this session). Expandable
	// sections' natural height (expandableNaturalTotal, from Pass 1 above)
	// is folded in so a "grow to fit content, cap at available space, then
	// scroll" dialog (e.g. Config Apps Menu's app list) shrinks correctly
	// too -- with exactly one expandable section this composes cleanly with
	// Pass 2's remaining/expandableCount split below (remaining ends up
	// exactly expandableNaturalTotal after the shrink, so expandableH ==
	// that section's own natural height). With multiple expandable sections
	// in a non-maximized dialog, remaining still divides evenly rather than
	// per-section-natural -- not needed by any current screen.
	if !m.maximized {
		naturalInner := fixedTotal + expandableNaturalTotal + buttonBudget
		if enabled := m.title != "" && currentConfig.UI.LargeTitleBars; enabled {
			naturalInner += LargeTitleBarOverhead
		}
		naturalHeight := naturalInner + layout.BorderHeight()
		if naturalHeight < m.height {
			m.height = naturalHeight
			innerHeight = naturalInner
		}
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

	// Pass 2: size each section at the inset width. Dispatch on
	// IsVariableHeight() directly, not "sectionHeights[i] == 0" -- a FIXED
	// section (e.g. a header with no subtitle/tasks/progress bar to show)
	// can legitimately have a genuine natural height of 0, which the old
	// zero-as-sentinel check misread as "not set, treat as expandable,"
	// handing it a full share of expandableH and starving/overflowing every
	// section after it.
	for i, sec := range m.contentSections {
		var h int
		if sec.IsVariableHeight() {
			h = expandableH
			if remainder > 0 {
				h++
				remainder--
			}
		} else {
			h = sectionHeights[i]
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
