package tui

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"DockSTARTer2/internal/tui/components/sinput"
	"DockSTARTer2/internal/webmsg"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// WebDisplaySettings holds font and refresh-rate settings for the xterm.js terminal.
type WebDisplaySettings struct {
	FontFamily  string `json:"fontFamily"`
	FontSize    int    `json:"fontSize"`
	RefreshRate int    `json:"-"`
}

// DefaultWebDisplaySettings returns sensible defaults matching xterm.js defaults.
func DefaultWebDisplaySettings() WebDisplaySettings {
	return WebDisplaySettings{
		FontFamily:  "monospace",
		FontSize:    14,
		RefreshRate: 100,
	}
}

// WebDisplayDialog is the dialog for configuring web terminal display settings.
type WebDisplayDialog struct {
	outer          *MenuModel
	defaultSection *MenuModel
	familyMenu     *MenuModel
	sizeSection    *MenuModel
	sizeInput      *sinput.Model
	refreshSection *MenuModel
	refreshInput   *sinput.Model
	initial        WebDisplaySettings // settings at dialog open time, for Cancel
}

type applyWebDisplayMsg struct{}
type resetWebDisplayMsg struct{}
type cancelWebDisplayMsg struct{}

// isDefaultFont returns true when the font family value represents the browser default.
func isDefaultFont(family string) bool {
	return family == "" || family == "monospace"
}

var webDisplayFontFamilies = []struct {
	value string
	label string
}{
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

func buildDefaultSection(isDefault bool) *MenuModel {
	items := []MenuItem{
		{
			Tag:          "Use default browser font",
			IsCheckbox:   true,
			Selectable:   true,
			Checked:      isDefault,
			Selected:     isDefault,
		},
	}
	m := NewMenuModel("web_display_default", "", "", items)
	m.SetSubMenuMode(true)
	m.SetVariableHeight(false)
	m.SetIsDialog(false)
	m.SetButtons([]ButtonDef{})
	m.SetShowLockGutter(false)
	m.SetNoLeftMargin(true)
	m.SetFlowMode(false)
	return m
}

func buildFamilyMenu(current string) *MenuModel {
	useDefault := isDefaultFont(current)
	var items []MenuItem
	checkedIdx := 0
	for i, f := range webDisplayFontFamilies {
		fCopy := f
		checked := !useDefault && fCopy.value == current
		if checked {
			checkedIdx = i
		}
		items = append(items, MenuItem{
			Tag:           fCopy.label,
			IsRadioButton: true,
			Selectable:    true,
			Checked:       checked,
			Selected:      checked,
			Metadata:      map[string]string{"value": fCopy.value},
		})
	}
	m := NewMenuModel("web_display_family", "Font Family", "", items)
	m.SetSubMenuMode(true)
	m.SetVariableHeight(false)
	m.SetIsDialog(false)
	m.SetButtons([]ButtonDef{})
	m.SetMaximized(true)
	m.SetShowLockGutter(false)
	m.SetNoLeftMargin(true)
	m.SetFlowMode(true)
	m.SetFlowColumns(2)
	m.SetUpdateInterceptor(radioGroupInterceptor("web_display_family"))
	if !useDefault {
		m.Select(checkedIdx)
	}
	return m
}

// radioGroupInterceptor returns an interceptor that enforces single-selection
// among all radio items in the menu.  When the menu is in scrollable column mode
// (maxFlowRows > 0 and total rows exceed the viewport), Up/Down navigate
// across columns at the same row rather than down the item list.
func radioGroupInterceptor(id string) func(tea.Msg, *MenuModel) (tea.Cmd, bool) {
	return func(msg tea.Msg, m *MenuModel) (tea.Cmd, bool) {
		// Reading-order navigation when scrollbar is active (viewport is clipped).
		// Up/Down step through items in reading order (left→right per row) so the
		// visible row stays stable and the scrollbar doesn't jump.
		if kp, ok := msg.(tea.KeyPressMsg); ok && m.flowColumns >= 2 && m.maxFlowRows > 0 {
			totalRows := m.ScrollTotal()
			if totalRows > m.maxFlowRows {
				isUp := key.Matches(kp, Keys.Up)
				isDown := key.Matches(kp, Keys.Down)
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
					numCols := m.flowColumns
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
							visible := m.layout.ViewportHeight
							if visible > 0 {
								if cursorRow < m.viewStartY {
									m.viewStartY = cursorRow
								} else if cursorRow >= m.viewStartY+visible {
									m.viewStartY = cursorRow - visible + 1
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
		if kp, ok := msg.(tea.KeyPressMsg); ok && m.flowColumns >= 2 {
			isUp := key.Matches(kp, Keys.Up)
			isDown := key.Matches(kp, Keys.Down)
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

		if _, ok := msg.(ToggleFocusedMsg); !ok {
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
	d := &WebDisplayDialog{initial: current}
	d.defaultSection = buildDefaultSection(isDefaultFont(current.FontFamily))
	d.familyMenu = buildFamilyMenu(current.FontFamily)
	d.sizeSection, d.sizeInput = NewNumberSinputSection("web_display_size", "Font Size", strconv.Itoa(current.FontSize))
	refreshRate := current.RefreshRate
	if refreshRate <= 0 {
		refreshRate = 100
	}
	d.refreshSection, d.refreshInput = NewNumberSinputSection("web_display_refresh", "Refresh Rate (ms, applies on next reload)", strconv.Itoa(refreshRate))

	outer := NewMenuModel("web_display", "Browser Settings", "", nil)
	outer.SetMaximized(false)
	outer.SetIsDialog(true)
	outer.SetDialogType(DialogTypeConfirm)
	outer.SetShowButtons(true)
	outer.SetEscAction(func() tea.Msg { return CloseDialogMsg{} })
	outer.SetButtons([]ButtonDef{
		{Label: "Apply", ZoneID: "btn-select", Action: func() tea.Msg { return applyWebDisplayMsg{} }, Help: "Apply settings to the browser terminal."},
		{Label: "Reset", ZoneID: "btn-reset", Action: func() tea.Msg { return resetWebDisplayMsg{} }, Help: "Reset to default font settings."},
		{Label: "Back", ZoneID: "btn-back", Action: func() tea.Msg { return CloseDialogMsg{} }, Help: "Close without reverting changes."},
		{Label: "Cancel", ZoneID: "btn-cancel", Action: func() tea.Msg { return cancelWebDisplayMsg{} }, Help: "Revert to original settings and close."},
	})
	d.familyMenu.SetDisabled(isDefaultFont(current.FontFamily))
	outer.AddContentSection(d.defaultSection)
	outer.AddContentSection(d.familyMenu)
	outer.AddContentSection(d.sizeSection)
	outer.AddContentSection(d.refreshSection)
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
	// When "Use browser default" is checked, block all interaction with the font list.
	if d.isBrowserDefault() {
		switch m := msg.(type) {
		case LayerHitMsg:
			if strings.Contains(m.ID, "web_display_family") {
				return d, nil
			}
		case LayerWheelMsg:
			if strings.Contains(m.ID, "web_display_family") {
				return d, nil
			}
		case tea.MouseWheelMsg:
			// Block wheel when family section is focused
			if d.outer.GetFocusedSection() == 1 {
				return d, nil
			}
		}
	}

	if kp, ok := msg.(tea.KeyPressMsg); ok && d.isBrowserDefault() {
		if key.Matches(kp, Keys.CycleTab) {
			fs := d.outer.GetFocusedSection()
			if fs == 0 {
				// Tab from default section → skip familyMenu, go to size section
				d.outer.SetFocusedSection(2)
				return d, nil
			}
		}
		if key.Matches(kp, Keys.CycleShiftTab) {
			fs := d.outer.GetFocusedSection()
			if fs == 2 {
				// Shift-Tab from size section → skip familyMenu, go to default section
				d.outer.SetFocusedSection(0)
				return d, nil
			}
		}
	}

	switch msg.(type) {
	case applyWebDisplayMsg:
		secs := d.outer.GetContentSections()
		if len(secs) >= 1 {
			d.defaultSection = secs[0]
		}
		if len(secs) >= 2 {
			d.familyMenu = secs[1]
		}
		if len(secs) >= 3 {
			d.sizeSection = secs[2]
		}
		if len(secs) >= 4 {
			d.refreshSection = secs[3]
		}
		settings := d.collectSettings()
		d.outer.ClearProcessingState()
		d.sendAndStore(settings)
		// Freeze during reflow, thaw after 500ms fallback (WindowSizeMsg thaws early).
		return d, tea.Batch(
			func() tea.Msg { return FreezeDisplayMsg{} },
			tea.Tick(500*time.Millisecond, func(time.Time) tea.Msg { return ThawDisplayMsg{} }),
		)

	case cancelWebDisplayMsg:
		d.sendAndStore(d.initial)
		return d, CloseDialog()

	case resetWebDisplayMsg:
		defaults := DefaultWebDisplaySettings()
		d.defaultSection = buildDefaultSection(isDefaultFont(defaults.FontFamily))
		d.familyMenu = buildFamilyMenu(defaults.FontFamily)
		d.sizeSection, d.sizeInput = NewNumberSinputSection("web_display_size", "Font Size", strconv.Itoa(defaults.FontSize))
		d.refreshSection, d.refreshInput = NewNumberSinputSection("web_display_refresh", "Refresh Rate (ms, applies on next reload)", strconv.Itoa(defaults.RefreshRate))
		d.outer.ReplaceSections(d.defaultSection, d.familyMenu, d.sizeSection, d.refreshSection)
		w, h := d.outer.Width(), d.outer.Height()
		d.outer.SetSize(w, h)
		d.outer.ClearProcessingState()
		d.outer.SetFocusedItem(FocusList)
		d.sendAndStore(defaults)
		return d, nil
	}

	newOuter, cmd := d.outer.Update(msg)
	if m, ok := newOuter.(*MenuModel); ok {
		d.outer = m
	}
	// Sync d.defaultSection from outer so isBrowserDefault() sees current state,
	// then apply disabled to familyMenu immediately.
	secs := d.outer.GetContentSections()
	if len(secs) >= 1 {
		d.defaultSection = secs[0]
	}
	if len(secs) >= 2 {
		secs[1].SetDisabled(d.isBrowserDefault())
	}
	return d, cmd
}

func (d *WebDisplayDialog) sendAndStore(s WebDisplaySettings) {
	data, _ := json.Marshal(map[string]any{
		"type":       "display-settings",
		"fontFamily": s.FontFamily,
		"fontSize":   s.FontSize,
	})
	SendWebMsg(data)
	webmsg.SetDisplaySettings(webToken, webmsg.DisplaySettings{
		FontFamily:  s.FontFamily,
		FontSize:    s.FontSize,
		RefreshRate: s.RefreshRate,
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
		s.RefreshRate = v
	}
	return s
}

func (d *WebDisplayDialog) View() tea.View               { return d.outer.View() }
func (d *WebDisplayDialog) ViewString() string           { return d.outer.ViewString() }
func (d *WebDisplayDialog) IsMaximized() bool            { return d.outer.IsMaximized() }
func (d *WebDisplayDialog) SetFocused(f bool) {
	d.outer.SetFocused(f)
	d.outer.ApplySectionFocus()
}
func (d *WebDisplayDialog) HelpText() string             { return d.outer.HelpText() }
func (d *WebDisplayDialog) IsScrollbarDragging() bool    { return d.outer.IsScrollbarDragging() }
func (d *WebDisplayDialog) Layers() []*lipgloss.Layer    { return d.outer.Layers() }
func (d *WebDisplayDialog) GetHitRegions(offsetX, offsetY int) []HitRegion {
	return d.outer.GetHitRegions(offsetX, offsetY)
}

// GetInputCursor implements InputCursorProvider so AppModel positions the hardware cursor
// over the Font Size sinput field when it is focused.
func (d *WebDisplayDialog) GetInputCursor() (relX, relY int, shape tea.CursorShape, ok bool) {
	// Only show cursor when size section (index 2) is focused.
	sections := d.outer.GetContentSections()
	if len(sections) < 3 {
		return 0, 0, tea.CursorBar, false
	}
	if !sections[2].IsActive() {
		return 0, 0, tea.CursorBar, false
	}
	layout := GetLayout()
	largeTitleOffset := 0
	if d.outer.layout.LargeTitleBar {
		largeTitleOffset = LargeTitleBarOverhead
	}
	defaultH := layout.BorderHeight() + len(d.defaultSection.GetItems())
	// Use the capped row count (maxFlowRows) so the cursor Y matches the rendered height.
	familyRows := d.familyMenu.maxFlowRows
	if familyRows == 0 {
		familyRows = d.familyMenu.GetFlowHeight(d.familyMenu.Width() - layout.BorderWidth())
	}
	familyH := familyRows + layout.BorderHeight()
	relY = layout.SingleBorder() + largeTitleOffset + defaultH + familyH + layout.SingleBorder()
	relX = layout.SingleBorder() + layout.SingleMargin() + (*d.sizeInput).PromptWidth() + (*d.sizeInput).CursorColumn()
	if (*d.sizeInput).IsOverwrite() {
		shape = tea.CursorBlock
	} else {
		shape = tea.CursorBar
	}
	return relX, relY, shape, true
}

func (d *WebDisplayDialog) SetSize(width, height int) {
	if width > 60 {
		width = 60
	}
	layout := GetLayout()
	contentW := width - layout.BorderWidth() - layout.ContentMarginWidth()
	if contentW < 1 {
		contentW = 1
	}

	// Fixed budget for every section OTHER than the font-family flow-grid,
	// via the same per-section formula calculateSectionLayout uses
	// internally (SectionHeight) -- no re-derivation here.
	defaultH := d.defaultSection.SectionHeight(contentW)
	sizeH := d.sizeSection.SectionHeight(contentW)
	refreshH := d.refreshSection.SectionHeight(contentW)
	btnH := ButtonRowHeight(contentW, 0, d.outer.getButtonSpecs()...)
	largeTitleOverhead := 0
	if currentConfig.UI.LargeTitleBars {
		largeTitleOverhead = LargeTitleBarOverhead
	}

	// Total rows the font family column layout wants (uncapped).
	d.familyMenu.SetMaxFlowRows(0)
	totalFamilyRows := d.familyMenu.GetFlowHeight(contentW - layout.BorderWidth())

	const minFamilyRows = 3

	// Space available for the family section after all other fixed sections,
	// buttons, the large-title-bar overhead, and both outer/family borders.
	// This dialog has no expandable content sections, so
	// calculateSectionLayout's DecideLargeTitleBar check only needs
	// largeTitleOverhead of slack (not the extra minRemaining it reserves for
	// dialogs with an expandable section to protect) -- see LargeTitleBarBudget.
	otherFixed := defaultH + sizeH + refreshH + btnH
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
