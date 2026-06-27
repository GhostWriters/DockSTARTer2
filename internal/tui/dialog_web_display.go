package tui

import (
	"encoding/json"
	"strconv"

	"DockSTARTer2/internal/tui/components/sinput"
	"DockSTARTer2/internal/webmsg"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// WebDisplaySettings holds font settings for the xterm.js terminal.
type WebDisplaySettings struct {
	FontFamily string `json:"fontFamily"`
	FontSize   int    `json:"fontSize"`
}

// DefaultWebDisplaySettings returns sensible defaults matching xterm.js defaults.
func DefaultWebDisplaySettings() WebDisplaySettings {
	return WebDisplaySettings{
		FontFamily: "monospace",
		FontSize:   14,
	}
}

// WebDisplayDialog is the dialog for configuring web terminal display settings.
type WebDisplayDialog struct {
	outer       *MenuModel
	familyMenu  *MenuModel
	sizeSection *MenuModel
	sizeInput   *sinput.Model
	initial     WebDisplaySettings // settings at dialog open time, for Cancel
}

type applyWebDisplayMsg struct{}
type resetWebDisplayMsg struct{}
type cancelWebDisplayMsg struct{}

var webDisplayFontFamilies = []struct {
	value string
	label string
}{
	{"monospace", "monospace (browser default)"},
	{"'Courier New', monospace", "Courier New"},
	{"'Fira Code', monospace", "Fira Code"},
	{"'JetBrains Mono', monospace", "JetBrains Mono"},
	{"'Cascadia Code', monospace", "Cascadia Code"},
	{"'Source Code Pro', monospace", "Source Code Pro"},
	{"'Ubuntu Mono', monospace", "Ubuntu Mono"},
}

func buildFamilyMenu(current string) *MenuModel {
	var items []MenuItem
	checkedIdx := 0
	for i, f := range webDisplayFontFamilies {
		fCopy := f
		checked := fCopy.value == current
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
	m.SetUpdateInterceptor(radioGroupInterceptor("web_display_family"))
	m.Select(checkedIdx)
	return m
}

// radioGroupInterceptor returns an interceptor that enforces single-selection
// among all radio items in the menu.
func radioGroupInterceptor(id string) func(tea.Msg, *MenuModel) (tea.Cmd, bool) {
	return func(msg tea.Msg, m *MenuModel) (tea.Cmd, bool) {
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
	d.familyMenu = buildFamilyMenu(current.FontFamily)
	d.sizeSection, d.sizeInput = NewNumberSinputSection("web_display_size", "Font Size", strconv.Itoa(current.FontSize))

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
	outer.AddContentSection(d.familyMenu)
	outer.AddContentSection(d.sizeSection)
	d.outer = outer
	return d
}

func (d *WebDisplayDialog) Init() tea.Cmd {
	return d.outer.Init()
}

func (d *WebDisplayDialog) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg.(type) {
	case applyWebDisplayMsg:
		secs := d.outer.GetContentSections()
		if len(secs) >= 1 {
			d.familyMenu = secs[0]
		}
		d.sendAndStore(d.collectSettings())
		d.outer.ClearProcessingState()
		return d, nil

	case cancelWebDisplayMsg:
		d.sendAndStore(d.initial)
		return d, CloseDialog()

	case resetWebDisplayMsg:
		defaults := DefaultWebDisplaySettings()
		d.familyMenu = buildFamilyMenu(defaults.FontFamily)
		d.sizeSection, d.sizeInput = NewNumberSinputSection("web_display_size", "Font Size", strconv.Itoa(defaults.FontSize))
		d.outer.ReplaceSections(d.familyMenu, d.sizeSection)
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
		FontFamily: s.FontFamily,
		FontSize:   s.FontSize,
	})
}

func (d *WebDisplayDialog) collectSettings() WebDisplaySettings {
	s := DefaultWebDisplaySettings()
	for _, it := range d.familyMenu.GetItems() {
		if it.Checked && it.Metadata != nil {
			s.FontFamily = it.Metadata["value"]
			break
		}
	}
	if v, err := strconv.Atoi(d.sizeInput.Value()); err == nil && v > 0 {
		s.FontSize = v
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
	// Only show cursor when size section is focused.
	sections := d.outer.GetContentSections()
	if len(sections) < 2 {
		return 0, 0, tea.CursorBar, false
	}
	if !sections[1].IsActive() {
		return 0, 0, tea.CursorBar, false
	}
	layout := GetLayout()
	largeTitleOffset := 0
	if d.outer.layout.LargeTitleBar {
		largeTitleOffset = LargeTitleBarOverhead
	}
	familyH := layout.BorderHeight() + len(d.familyMenu.GetItems())
	// outer border(1) + large title overhead + familyH + size section border top(1) = input row
	relY = layout.SingleBorder() + largeTitleOffset + familyH + layout.SingleBorder()
	// outer border(1) + ContentSideMargin(1) + cursor col (section border absorbed by margin)
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
	familyH := layout.BorderHeight() + len(d.familyMenu.GetItems())
	// Size input section: 1 content line + 2 border rows = 3
	sizeH := 1 + layout.BorderHeight()
	btnH := DialogButtonHeight
	largeTitleOverhead := 0
	if currentConfig.UI.LargeTitleBars {
		largeTitleOverhead = LargeTitleBarOverhead
	}
	natural := layout.BorderHeight() + largeTitleOverhead + familyH + sizeH + btnH
	if natural < height {
		height = natural
	}
	d.outer.SetSize(width, height)
}
