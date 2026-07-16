package tui

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/displayengine"
	"DockSTARTer2/internal/tui/components/sinput"
	"DockSTARTer2/internal/webmsg"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// WebDisplaySettings holds font and refresh-rate settings for the xterm.js terminal.
type WebDisplaySettings struct {
	FontFamily string `json:"fontFamily"`
	FontSize   int    `json:"fontSize"`
	// UseDefaultFont is whether "Use default browser font" is checked.
	// Explicit flag, not inferred from FontFamily -- FontFamily can equal
	// "monospace" even when this is false (the user explicitly picked
	// "Default (Monospace)" from the Font Family list).
	UseDefaultFont bool `json:"-"`
	RefreshRate    int  `json:"-"`
}

// DefaultWebDisplaySettings returns sensible defaults matching xterm.js defaults.
// RefreshRate defaults to the Appearance menu's configured refresh rate, not
// a hardcoded value, so web and local sessions agree unless the browser
// explicitly overrides it.
func DefaultWebDisplaySettings() WebDisplaySettings {
	return WebDisplaySettings{
		FontFamily:     "monospace",
		FontSize:       14,
		UseDefaultFont: true,
		RefreshRate:    defaultRefreshRate(),
	}
}

// defaultRefreshRate returns the Appearance menu's configured refresh rate,
// falling back to the config package's default if unset.
func defaultRefreshRate() int {
	if rate := displayengine.CurrentConfig().UI.RefreshRate; rate > 0 {
		return rate
	}
	return config.DefaultConfig().UI.RefreshRate
}

// refreshRateHelp is the helpline text shown while the Refresh Rate field is focused.
func refreshRateHelp() string {
	return fmt.Sprintf("How often the terminal repaints, in milliseconds (%d-%d). Lower is smoother but uses more bandwidth.", config.MinRefreshRateMS, config.MaxRefreshRateMS)
}

// WebDisplayDialog is the dialog for configuring web terminal display settings.
type WebDisplayDialog struct {
	outer          *displayengine.MenuModel
	defaultSection *displayengine.MenuModel
	familyMenu     *displayengine.MenuModel
	sizeSection    *displayengine.MenuModel
	sizeInput      *sinput.Model
	refreshSection *displayengine.MenuModel
	refreshInput   *sinput.Model
	initial        WebDisplaySettings // settings at dialog open time, for Cancel
	appliedRefresh int                // refresh rate as of the most recent Apply
	appliedFont    WebDisplaySettings // font-related fields as of the most recent Apply (RefreshRate unused)
}

type applyWebDisplayMsg struct{}
type resetWebDisplayMsg struct{}
type cancelWebDisplayMsg struct{}
type backWebDisplayMsg struct{}

var webDisplayFontFamilies = []struct {
	value string
	label string
}{
	{"monospace", "Default (Monospace)"},
	{"'Courier New', monospace", "Courier New"},
	{"'Fira Code', monospace", "Fira Code"},
	{"'JetBrains Mono', monospace", "JetBrains Mono"},
	{"'Cascadia Code', monospace", "Cascadia Code"},
	{"'Source Code Pro', monospace", "Source Code Pro"},
	{"'Ubuntu Mono', monospace", "Ubuntu Mono"},
	{"'Consolas', monospace", "Consolas"},
	{"'Monaco', monospace", "Monaco"},
	{"'Menlo', monospace", "Menlo"},
	{"'Inconsolata', monospace", "Inconsolata"},
	{"'Hack', monospace", "Hack"},
	{"'Roboto Mono', monospace", "Roboto Mono"},
	{"'IBM Plex Mono', monospace", "IBM Plex Mono"},
}

func buildDefaultSection(isDefault bool) *displayengine.MenuModel {
	items := []displayengine.MenuItem{
		{
			Tag:        "Use default browser font",
			IsCheckbox: true,
			Selectable: true,
			Checked:    isDefault,
			Selected:   isDefault,
		},
	}
	m := displayengine.NewMenuModel("web_display_default", "", "", items)
	m.SetSubMenuMode(true)
	m.SetVariableHeight(false)
	m.SetIsDialog(false)
	m.SetButtons([]displayengine.ButtonDef{})
	m.SetShowLockGutter(false)
	m.SetNoLeftMargin(true)
	m.SetFlowMode(false)
	return m
}

func buildFamilyMenu(current string) *displayengine.MenuModel {
	// "Default (Monospace)" is a literal entry in webDisplayFontFamilies
	// (value "monospace"), so it's matched the same way as every other
	// font -- no special-casing needed here. Whether the family list is
	// disabled/forced is a separate concern driven by the checkbox
	// (WebDisplaySettings.UseDefaultFont), not by what value happens to be
	// selected here.
	effectiveCurrent := current
	if effectiveCurrent == "" {
		effectiveCurrent = "monospace"
	}
	var items []displayengine.MenuItem
	checkedIdx := 0
	for i, f := range webDisplayFontFamilies {
		fCopy := f
		checked := fCopy.value == effectiveCurrent
		if checked {
			checkedIdx = i
		}
		items = append(items, displayengine.MenuItem{
			Tag:           fCopy.label,
			IsRadioButton: true,
			Selectable:    true,
			Checked:       checked,
			Selected:      checked,
			Metadata:      map[string]string{"value": fCopy.value},
		})
	}
	m := displayengine.NewMenuModel("web_display_family", "Font Family", "", items)
	m.SetSubMenuMode(true)
	m.SetVariableHeight(false)
	m.SetIsDialog(false)
	m.SetButtons([]displayengine.ButtonDef{})
	m.SetMaximized(true)
	m.SetShowLockGutter(false)
	m.SetNoLeftMargin(true)
	m.SetFlowMode(true)
	m.SetFlowColumns(2)
	m.SetUpdateInterceptor(radioGroupInterceptor("web_display_family"))
	m.Select(checkedIdx)
	return m
}

// radioGroupInterceptor returns an interceptor that enforces single-selection
// among all radio items in the menu.  When the menu is in scrollable column mode
// (maxFlowRows > 0 and total rows exceed the viewport), Up/Down navigate
// across columns at the same row rather than down the item list.
func radioGroupInterceptor(id string) func(tea.Msg, *displayengine.MenuModel) (tea.Cmd, bool) {
	return func(msg tea.Msg, m *displayengine.MenuModel) (tea.Cmd, bool) {
		// Reading-order navigation when scrollbar is active (viewport is clipped).
		// Up/Down step through items in reading order (left→right per row) so the
		// visible row stays stable and the scrollbar doesn't jump.
		if kp, ok := msg.(tea.KeyPressMsg); ok && m.FlowColumns >= 2 && m.MaxFlowRows > 0 {
			totalRows := m.ScrollTotal()
			if totalRows > m.MaxFlowRows {
				isUp := key.Matches(kp, displayengine.Keys.Up)
				isDown := key.Matches(kp, displayengine.Keys.Down)
				if isUp || isDown {
					its := m.GetItems()
					// Count total non-separator items and find current non-sep index (ni).
					n := 0
					ni := -1
					for i, it := range its {
						if it.IsSeparator {
							continue
						}
						if i == m.Index() {
							ni = n
						}
						n++
					}
					if ni < 0 {
						return nil, false
					}
					// Convert column-major ni to reading-order index, step ±1, convert back.
					numCols := m.FlowColumns
					col := ni / totalRows
					row := ni % totalRows
					readIdx := row*numCols + col
					if isDown {
						readIdx++
					} else {
						readIdx--
					}
					if readIdx < 0 || readIdx >= n {
						return nil, true // at boundary: consume and do nothing (no wrap)
					}
					// Convert reading-order index back to column-major ni.
					targetCol := readIdx % numCols
					targetRow := readIdx / numCols
					targetNI := targetCol*totalRows + targetRow
					if targetNI < 0 || targetNI >= n {
						return nil, false
					}
					// Map targetNI to item index.
					count := 0
					for i, it := range its {
						if it.IsSeparator {
							continue
						}
						if count == targetNI {
							m.Select(i)
							// Update viewStartY so the selection stays in the visible viewport.
							totalRows := m.ScrollTotal()
							cursorRow := targetNI % totalRows
							visible := m.Layout.ViewportHeight
							if visible > 0 {
								if cursorRow < m.ViewStartY {
									m.ViewStartY = cursorRow
								} else if cursorRow >= m.ViewStartY+visible {
									m.ViewStartY = cursorRow - visible + 1
								}
							}
							m.InvalidateCache()
							return nil, true
						}
						count++
					}
				}
			}
		}

		// Non-scrolling column mode: block Up/Down from wrapping past the list ends.
		if kp, ok := msg.(tea.KeyPressMsg); ok && m.FlowColumns >= 2 {
			isUp := key.Matches(kp, displayengine.Keys.Up)
			isDown := key.Matches(kp, displayengine.Keys.Down)
			if isUp || isDown {
				its := m.GetItems()
				n := 0
				ni := -1
				for i, it := range its {
					if it.IsSeparator {
						continue
					}
					if i == m.Index() {
						ni = n
					}
					n++
				}
				if isDown && ni == n-1 {
					return nil, true // at last item, block wrap
				}
				if isUp && ni == 0 {
					return nil, true // at first item, block wrap
				}
			}
		}

		if _, ok := msg.(displayengine.ToggleFocusedMsg); !ok {
			return nil, false
		}
		idx := m.Index()
		its := m.GetItems()
		if idx < 0 || idx >= len(its) || !its[idx].IsRadioButton {
			return nil, false
		}
		for i, it := range its {
			if it.IsRadioButton {
				it.Checked = (i == idx)
				it.Selected = it.Checked
				m.SetItem(i, it)
			}
		}
		return nil, false
	}
}

// NewWebDisplayDialog creates a new WebDisplayDialog with the given current settings.
func NewWebDisplayDialog(current WebDisplaySettings) *WebDisplayDialog {
	d := &WebDisplayDialog{initial: current, appliedRefresh: current.RefreshRate, appliedFont: current}
	d.defaultSection = buildDefaultSection(current.UseDefaultFont)
	d.familyMenu = buildFamilyMenu(current.FontFamily)
	// When the checkbox was checked, show the true default size (not
	// current.FontSize, which may be a stale leftover from a previous
	// custom size) -- matches familyMenu showing "Default (Monospace)"
	// selected rather than some other font.
	initialSize := current.FontSize
	if current.UseDefaultFont {
		initialSize = DefaultWebDisplaySettings().FontSize
	}
	d.sizeSection, d.sizeInput = displayengine.NewNumberSinputSection("web_display_size", "Font Size", strconv.Itoa(initialSize))
	refreshRate := current.RefreshRate
	if refreshRate <= 0 {
		refreshRate = defaultRefreshRate()
	}
	d.refreshSection, d.refreshInput = displayengine.NewNumberSinputSection("web_display_refresh", "Refresh Rate (ms)", strconv.Itoa(refreshRate))
	d.refreshSection.SectionHelp = refreshRateHelp()

	outer := displayengine.NewMenuModel("web_display", "Browser Settings", "", nil)
	outer.SetMaximized(false)
	outer.SetIsDialog(true)
	outer.SetDialogType(displayengine.DialogTypeConfirm)
	outer.SetShowButtons(true)
	outer.SetEscAction(func() tea.Msg { return displayengine.CloseDialogMsg{} })
	outer.SetButtons([]displayengine.ButtonDef{
		{Label: "Apply", ZoneID: "btn-select", Action: func() tea.Msg { return applyWebDisplayMsg{} }, Help: "Apply settings to the browser terminal."},
		{Label: "Reset", ZoneID: "btn-reset", Action: func() tea.Msg { return resetWebDisplayMsg{} }, Help: "Reset to default font settings."},
		{Label: "Back", ZoneID: "btn-back", Action: func() tea.Msg { return backWebDisplayMsg{} }, Help: "Close without reverting changes."},
		{Label: "Cancel", ZoneID: "btn-cancel", Action: func() tea.Msg { return cancelWebDisplayMsg{} }, Help: "Revert to original settings and close."},
	})
	d.familyMenu.SetDisabled(current.UseDefaultFont)
	d.sizeSection.SetDisabled(current.UseDefaultFont)
	outer.AddContentSection(d.defaultSection)
	outer.AddContentSection(d.familyMenu)
	outer.AddContentRow(d.sizeSection, d.refreshSection)
	d.outer = outer
	return d
}

func (d *WebDisplayDialog) Init() tea.Cmd {
	return d.outer.Init()
}

func (d *WebDisplayDialog) isBrowserDefault() bool {
	for _, it := range d.defaultSection.GetItems() {
		if it.Checked {
			return true
		}
	}
	return false
}

func (d *WebDisplayDialog) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// When "Use browser default" is checked, block all interaction with the
	// font family list and font size input -- both are forced back to the
	// default (see collectSettings) regardless of what's clicked/typed.
	if d.isBrowserDefault() {
		switch m := msg.(type) {
		case displayengine.LayerHitMsg:
			if strings.Contains(m.ID, "web_display_family") || strings.Contains(m.ID, "web_display_size") {
				return d, nil
			}
		case displayengine.LayerWheelMsg:
			if strings.Contains(m.ID, "web_display_family") || strings.Contains(m.ID, "web_display_size") {
				return d, nil
			}
		case tea.MouseWheelMsg:
			// Block wheel when family section is focused, or when the
			// size/refresh row is focused with sizeSection (sub-index 0) active.
			fs := d.outer.GetFocusedSection()
			if fs == 1 {
				return d, nil
			}
			if fs == 2 {
				if row, ok := d.outer.GetContentSections()[2].(*displayengine.ContentRow); ok && row.SubFocusIndex() == 0 {
					return d, nil
				}
			}
		}
	}

	if kp, ok := msg.(tea.KeyPressMsg); ok && d.isBrowserDefault() {
		if key.Matches(kp, displayengine.Keys.CycleTab) {
			fs := d.outer.GetFocusedSection()
			if fs == 0 {
				// Tab from default section → skip familyMenu (disabled) and
				// sizeSection (also disabled, forced to the default), land
				// on refreshSection, the row's second child.
				d.outer.SetFocusedSection(2)
				if row, ok := d.outer.GetContentSections()[2].(*displayengine.ContentRow); ok {
					row.SetSubFocusIndex(1)
				}
				return d, nil
			}
		}
		if key.Matches(kp, displayengine.Keys.CycleShiftTab) {
			fs := d.outer.GetFocusedSection()
			row, isRow := d.outer.GetContentSections()[2].(*displayengine.ContentRow)
			if fs == 2 && (!isRow || row.SubFocusIndex() == 1) {
				// Shift-Tab from refreshSection (or the row generally, if we
				// can't tell which child) → skip sizeSection/familyMenu
				// (both disabled), go to default section.
				d.outer.SetFocusedSection(0)
				return d, nil
			}
		}
	}

	switch msg.(type) {
	case applyWebDisplayMsg:
		secs := d.outer.GetContentSections()
		if len(secs) >= 1 {
			if mm, ok := secs[0].(*displayengine.MenuModel); ok {
				d.defaultSection = mm
			}
		}
		if len(secs) >= 2 {
			if mm, ok := secs[1].(*displayengine.MenuModel); ok {
				d.familyMenu = mm
			}
		}
		if len(secs) >= 3 {
			if row, ok := secs[2].(*displayengine.ContentRow); ok && len(row.Items()) >= 2 {
				if mm, ok := row.Items()[0].(*displayengine.MenuModel); ok {
					d.sizeSection = mm
				}
				if mm, ok := row.Items()[1].(*displayengine.MenuModel); ok {
					d.refreshSection = mm
				}
			}
		}
		settings := d.collectSettings()
		d.outer.ClearProcessingState()
		d.sendAndStore(settings)
		d.appliedRefresh = settings.RefreshRate
		// Font changes reflow the browser's terminal grid (cell size changes),
		// which needs the freeze/thaw dance below to hide the resize. Refresh
		// rate alone never resizes anything, so skip it when only that changed.
		fontChanged := settings.FontFamily != d.appliedFont.FontFamily ||
			settings.FontSize != d.appliedFont.FontSize ||
			settings.UseDefaultFont != d.appliedFont.UseDefaultFont
		d.appliedFont = settings
		if !fontChanged {
			return d, nil
		}
		// Freeze during reflow, thaw after 500ms fallback (WindowSizeMsg thaws early).
		return d, tea.Batch(
			func() tea.Msg { return FreezeDisplayMsg{} },
			tea.Tick(500*time.Millisecond, func(time.Time) tea.Msg { return ThawDisplayMsg{} }),
		)

	case cancelWebDisplayMsg:
		d.sendAndStore(d.initial)
		return d, CloseDialog()

	case backWebDisplayMsg:
		// The refresh rate can only take effect on a new tea.Program (its FPS
		// is fixed at construction), so if it changed during this dialog
		// session, quitting -- same as Exit -- ends this web session and the
		// browser auto-reconnects with the new rate. Only do this on a safe
		// page, mirroring RestartForConfigChange's local-session guard.
		if GetConnType() == "web" && d.appliedRefresh != d.initial.RefreshRate && isRestartSafeLocally() {
			return d, tea.Quit
		}
		return d, CloseDialog()

	case resetWebDisplayMsg:
		defaults := DefaultWebDisplaySettings()
		d.defaultSection = buildDefaultSection(defaults.UseDefaultFont)
		d.familyMenu = buildFamilyMenu(defaults.FontFamily)
		d.sizeSection, d.sizeInput = displayengine.NewNumberSinputSection("web_display_size", "Font Size", strconv.Itoa(defaults.FontSize))
		d.refreshSection, d.refreshInput = displayengine.NewNumberSinputSection("web_display_refresh", "Refresh Rate (ms)", strconv.Itoa(defaults.RefreshRate))
		d.refreshSection.SectionHelp = refreshRateHelp()
		d.familyMenu.SetDisabled(defaults.UseDefaultFont)
		d.sizeSection.SetDisabled(defaults.UseDefaultFont)
		d.outer.ReplaceSections(d.defaultSection, d.familyMenu, displayengine.NewContentRow(d.sizeSection, d.refreshSection))
		w, h := d.outer.Width(), d.outer.Height()
		d.outer.SetSize(w, h)
		d.outer.ClearProcessingState()
		d.outer.SetFocusedItem(displayengine.FocusList)
		d.sendAndStore(defaults)
		return d, nil
	}

	newOuter, cmd := d.outer.Update(msg)
	if m, ok := newOuter.(*displayengine.MenuModel); ok {
		d.outer = m
	}
	// Sync d.defaultSection from outer so isBrowserDefault() sees current state,
	// then apply disabled to familyMenu and sizeSection immediately -- font
	// size is silently forced back to the default whenever "Use default
	// browser font" is checked (see collectSettings), so its input must be
	// disabled the same way the family list already is, or a typed value
	// looks live but is discarded on Apply.
	secs := d.outer.GetContentSections()
	if len(secs) >= 1 {
		if mm, ok := secs[0].(*displayengine.MenuModel); ok {
			d.defaultSection = mm
		}
	}
	if len(secs) >= 2 {
		if mm, ok := secs[1].(*displayengine.MenuModel); ok {
			mm.SetDisabled(d.isBrowserDefault())
		}
	}
	if len(secs) >= 3 {
		if row, ok := secs[2].(*displayengine.ContentRow); ok && len(row.Items()) >= 1 {
			if mm, ok := row.Items()[0].(*displayengine.MenuModel); ok {
				disabled := d.isBrowserDefault()
				mm.SetDisabled(disabled)
				// Show the true default size while disabled, not whatever
				// custom value was previously typed/loaded -- a disabled
				// field claiming to represent "the default" must actually
				// display it, matching familyMenu showing no radio selected
				// rather than leaving some other font's radio checked.
				if disabled {
					want := strconv.Itoa(DefaultWebDisplaySettings().FontSize)
					if d.sizeInput.Value() != want {
						d.sizeInput.SetValue(want)
						mm.InvalidateCache()
					}
				}
			}
		}
	}
	return d, cmd
}

func (d *WebDisplayDialog) sendAndStore(s WebDisplaySettings) {
	data, _ := json.Marshal(map[string]any{
		"type":           "display-settings",
		"fontFamily":     s.FontFamily,
		"fontSize":       s.FontSize,
		"useDefaultFont": s.UseDefaultFont,
		"refreshRate":    s.RefreshRate,
	})
	SendWebMsg(data)
	webmsg.SetDisplaySettings(webToken, webmsg.DisplaySettings{
		FontFamily:     s.FontFamily,
		FontSize:       s.FontSize,
		UseDefaultFont: s.UseDefaultFont,
		RefreshRate:    s.RefreshRate,
	})
}

func (d *WebDisplayDialog) collectSettings() WebDisplaySettings {
	s := DefaultWebDisplaySettings()
	// Check if "Use browser default" is selected.
	useDefault := false
	for _, it := range d.defaultSection.GetItems() {
		if it.Checked {
			useDefault = true
			break
		}
	}
	s.UseDefaultFont = useDefault
	if !useDefault {
		// Otherwise read the radio selection.
		for _, it := range d.familyMenu.GetItems() {
			if it.Checked && it.Metadata != nil {
				s.FontFamily = it.Metadata["value"]
				break
			}
		}
		if v, err := strconv.Atoi(d.sizeInput.Value()); err == nil && v > 0 {
			s.FontSize = v
		}
	}
	if v, err := strconv.Atoi(d.refreshInput.Value()); err == nil && v > 0 {
		switch {
		case v < config.MinRefreshRateMS:
			v = config.MinRefreshRateMS
		case v > config.MaxRefreshRateMS:
			v = config.MaxRefreshRateMS
		}
		s.RefreshRate = v
	}
	return s
}

func (d *WebDisplayDialog) View() tea.View     { return d.outer.View() }
func (d *WebDisplayDialog) ViewString() string { return d.outer.ViewString() }
func (d *WebDisplayDialog) IsMaximized() bool  { return d.outer.IsMaximized() }
func (d *WebDisplayDialog) SetFocused(f bool) {
	d.outer.SetFocused(f)
	d.outer.ApplySectionFocus()
}
func (d *WebDisplayDialog) HelpText() string          { return d.outer.HelpText() }
func (d *WebDisplayDialog) IsScrollbarDragging() bool { return d.outer.IsScrollbarDragging() }
func (d *WebDisplayDialog) Layers() []*lipgloss.Layer { return d.outer.Layers() }
func (d *WebDisplayDialog) GetHitRegions(offsetX, offsetY int) []displayengine.HitRegion {
	return d.outer.GetHitRegions(offsetX, offsetY)
}

// GetInputCursor implements InputCursorProvider so AppModel positions the hardware cursor
// over whichever of the Font Size / Refresh Rate sinput fields is focused.
func (d *WebDisplayDialog) GetInputCursor() (relX, relY int, shape tea.CursorShape, ok bool) {
	// Only show cursor when the size/refresh row (index 2) is focused.
	sections := d.outer.GetContentSections()
	if len(sections) < 3 {
		return 0, 0, tea.CursorBar, false
	}
	row, isRow := sections[2].(*displayengine.ContentRow)
	if !isRow || d.outer.GetFocusedSection() != 2 {
		return 0, 0, tea.CursorBar, false
	}
	layout := displayengine.GetLayout()
	largeTitleOffset := 0
	if d.outer.Layout.LargeTitleBar {
		largeTitleOffset = displayengine.LargeTitleBarOverhead
	}
	defaultH := layout.BorderHeight() + len(d.defaultSection.GetItems())
	// Use the capped row count (maxFlowRows) so the cursor Y matches the rendered height.
	familyRows := d.familyMenu.MaxFlowRows
	if familyRows == 0 {
		familyRows = d.familyMenu.GetFlowHeight(d.familyMenu.Width() - layout.BorderWidth())
	}
	familyH := familyRows + layout.BorderHeight()
	relY = layout.SingleBorder() + largeTitleOffset + defaultH + familyH + layout.SingleBorder()

	switch row.SubFocusIndex() {
	case 0:
		relX = layout.SingleBorder() + layout.SingleMargin() + (*d.sizeInput).PromptWidth() + (*d.sizeInput).CursorColumn()
		if (*d.sizeInput).IsOverwrite() {
			shape = tea.CursorBlock
		} else {
			shape = tea.CursorBar
		}
	case 1:
		// Refresh input starts at the size child's assigned width (the two
		// children split the row's width evenly).
		relX = row.ItemWidth(0) + layout.SingleBorder() + layout.SingleMargin() + (*d.refreshInput).PromptWidth() + (*d.refreshInput).CursorColumn()
		if (*d.refreshInput).IsOverwrite() {
			shape = tea.CursorBlock
		} else {
			shape = tea.CursorBar
		}
	default:
		return 0, 0, tea.CursorBar, false
	}
	return relX, relY, shape, true
}

func (d *WebDisplayDialog) SetSize(width, height int) {
	if width > 60 {
		width = 60
	}
	layout := displayengine.GetLayout()
	contentW := width - layout.BorderWidth() - layout.ContentMarginWidth()
	if contentW < 1 {
		contentW = 1
	}

	// Fixed budget for every section other than the font-family flow-grid,
	// via the same per-section SectionHeight formula calculateSectionLayout
	// uses internally. Font Size and Refresh Rate share one
	// displayengine.ContentRow, so their combined contribution is the max of
	// the two (side by side), not the sum -- summing would double-count and
	// leave the row's actual (shorter) height as unclaimed blank space.
	defaultH := d.defaultSection.SectionHeight(contentW)
	rowContentW := displayengine.SplitWidth(contentW, 2)
	sizeRowH := d.sizeSection.SectionHeight(rowContentW[0])
	if h := d.refreshSection.SectionHeight(rowContentW[1]); h > sizeRowH {
		sizeRowH = h
	}
	btnH := displayengine.ButtonRowHeight(contentW, 0, d.outer.GetButtonSpecsForState()...)
	largeTitleOverhead := 0
	if displayengine.CurrentConfig().UI.LargeTitleBars {
		largeTitleOverhead = displayengine.LargeTitleBarOverhead
	}

	// Total rows the font family column layout wants (uncapped).
	d.familyMenu.SetMaxFlowRows(0)
	totalFamilyRows := d.familyMenu.GetFlowHeight(contentW - layout.BorderWidth())

	const minFamilyRows = 3

	// Space available for the family section after all other fixed sections,
	// buttons, the large-title-bar overhead, and both outer/family borders.
	// This dialog has no expandable content sections, so
	// calculateSectionLayout's displayengine.DecideLargeTitleBar check only needs
	// largeTitleOverhead of slack (not the extra minRemaining it reserves for
	// dialogs with an expandable section to protect) -- see LargeTitleBarBudget.
	otherFixed := defaultH + sizeRowH + btnH
	availableForFamily := height - layout.BorderHeight() - largeTitleOverhead - otherFixed - layout.BorderHeight()
	if availableForFamily < minFamilyRows {
		availableForFamily = minFamilyRows
	}

	// Cap family rows to available space; no cap needed if everything fits.
	maxRows := totalFamilyRows
	if availableForFamily < totalFamilyRows {
		maxRows = availableForFamily
	}
	if maxRows < minFamilyRows {
		maxRows = minFamilyRows
	}
	d.familyMenu.SetMaxFlowRows(maxRows)

	familyH := maxRows + layout.BorderHeight()
	natural := layout.BorderHeight() + largeTitleOverhead + otherFixed + familyH
	if natural < height {
		height = natural
	}
	d.outer.SetSize(width, height)
}
