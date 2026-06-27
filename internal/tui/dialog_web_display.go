package tui

import (
	"encoding/json"
	"fmt"

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
	menu     *MenuModel
	settings WebDisplaySettings
}

type applyWebDisplayMsg struct{}

var (
	webDisplayFontFamilies = []string{
		"monospace",
		"'Courier New', monospace",
		"'Fira Code', monospace",
		"'JetBrains Mono', monospace",
		"'Cascadia Code', monospace",
		"'Source Code Pro', monospace",
		"'Ubuntu Mono', monospace",
	}

	webDisplayFontSizes = []int{10, 11, 12, 13, 14, 15, 16, 18, 20, 24}
)

func fontFamilyLabel(f string) string {
	switch f {
	case "monospace":
		return "monospace (browser default)"
	case "'Courier New', monospace":
		return "Courier New"
	case "'Fira Code', monospace":
		return "Fira Code"
	case "'JetBrains Mono', monospace":
		return "JetBrains Mono"
	case "'Cascadia Code', monospace":
		return "Cascadia Code"
	case "'Source Code Pro', monospace":
		return "Source Code Pro"
	case "'Ubuntu Mono', monospace":
		return "Ubuntu Mono"
	default:
		return f
	}
}

// NewWebDisplayDialog creates a new WebDisplayDialog with the given current settings.
func NewWebDisplayDialog(current WebDisplaySettings) *WebDisplayDialog {
	var items []MenuItem

	// Font family section header
	items = append(items, MenuItem{
		Tag:         "Font Family",
		IsSeparator: true,
		Selectable:  false,
	})
	for _, f := range webDisplayFontFamilies {
		fCopy := f
		items = append(items, MenuItem{
			Tag:        fontFamilyLabel(fCopy),
			IsCheckbox: true,
			Selectable: true,
			Checked:    fCopy == current.FontFamily,
			Selected:   fCopy == current.FontFamily,
			Metadata:   map[string]string{"type": "fontFamily", "value": fCopy},
		})
	}

	// Font size section header
	items = append(items, MenuItem{
		Tag:         "Font Size",
		IsSeparator: true,
		Selectable:  false,
	})
	for _, s := range webDisplayFontSizes {
		sCopy := s
		items = append(items, MenuItem{
			Tag:        fmt.Sprintf("%d px", sCopy),
			IsCheckbox: true,
			Selectable: true,
			Checked:    sCopy == current.FontSize,
			Selected:   sCopy == current.FontSize,
			Metadata:   map[string]string{"type": "fontSize", "value": fmt.Sprintf("%d", sCopy)},
		})
	}

	menu := NewMenuModel("web_display", "Display Settings", "Configure web terminal font and size", items, CloseDialog())
	menu.SetCheckboxMode(true)
	menu.SetMaximized(false)
	menu.SetIsDialog(true)
	menu.SetDialogType(DialogTypeConfirm)
	menu.SetButtonLabels("Apply", "", "")
	menu.SetShowExit(false)
	menu.SetEnterAction(func() tea.Msg { return applyWebDisplayMsg{} })
	menu.SetEscAction(CloseDialog())

	return &WebDisplayDialog{menu: menu, settings: current}
}

func (d *WebDisplayDialog) Init() tea.Cmd { return d.menu.Init() }

func (d *WebDisplayDialog) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg.(type) {
	case applyWebDisplayMsg:
		settings := d.collectSettings()
		data, _ := json.Marshal(map[string]any{
			"type":       "display-settings",
			"fontFamily": settings.FontFamily,
			"fontSize":   settings.FontSize,
		})
		SendWebMsg(data)
		return d, CloseDialog()
	}

	var cmd tea.Cmd
	var newMenu tea.Model
	newMenu, cmd = d.menu.Update(msg)
	if menu, ok := newMenu.(*MenuModel); ok {
		d.menu = menu
	}
	return d, cmd
}

func (d *WebDisplayDialog) collectSettings() WebDisplaySettings {
	s := DefaultWebDisplaySettings()
	for _, it := range d.menu.GetItems() {
		if !it.Selected || it.Metadata == nil {
			continue
		}
		switch it.Metadata["type"] {
		case "fontFamily":
			s.FontFamily = it.Metadata["value"]
		case "fontSize":
			if n, err := fmt.Sscanf(it.Metadata["value"], "%d", &s.FontSize); n != 1 || err != nil {
				s.FontSize = DefaultWebDisplaySettings().FontSize
			}
		}
	}
	return s
}

func (d *WebDisplayDialog) View() tea.View        { return d.menu.View() }
func (d *WebDisplayDialog) ViewString() string    { return d.menu.ViewString() }
func (d *WebDisplayDialog) IsMaximized() bool     { return d.menu.IsMaximized() }
func (d *WebDisplayDialog) SetFocused(f bool)     { d.menu.SetFocused(f) }
func (d *WebDisplayDialog) HelpText() string      { return d.menu.HelpText() }
func (d *WebDisplayDialog) IsScrollbarDragging() bool { return d.menu.IsScrollbarDragging() }

func (d *WebDisplayDialog) SetSize(width, height int) {
	if width > 60 {
		width = 60
	}
	d.menu.SetSize(width, height)
}

func (d *WebDisplayDialog) Layers() []*lipgloss.Layer {
	return d.menu.Layers()
}

func (d *WebDisplayDialog) GetHitRegions(offsetX, offsetY int) []HitRegion {
	return d.menu.GetHitRegions(offsetX, offsetY)
}
