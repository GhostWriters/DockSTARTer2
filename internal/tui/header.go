package tui

import (
	"os"

	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/paths"
	"DockSTARTer2/internal/strutil"
	"DockSTARTer2/internal/update"
	"DockSTARTer2/internal/version"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// HeaderFocus states for interactive elements
type HeaderFocus int

const (
	HeaderFocusNone HeaderFocus = iota
	HeaderFocusApp
	HeaderFocusTmpl
)

// Zone IDs
const (
	ZoneAppVersion  = "header_app_ver"
	ZoneTmplVersion = "header_tmpl_ver"
)

// HeaderModel represents the header bar at the top of the TUI
type HeaderModel struct {
	width int

	// Cached values
	hostname string
	flags    []string
	focus    HeaderFocus
}

// NewHeaderModel creates a new header model with default values
func NewHeaderModel() *HeaderModel {
	hostname, _ := os.Hostname()

	var flags []string
	if console.Verbose() {
		flags = append(flags, "VERBOSE")
	}
	if console.Debug() {
		flags = append(flags, "DEBUG")
	}
	if console.Force() {
		flags = append(flags, "FORCE")
	}
	if console.AssumeYes() {
		flags = append(flags, "YES")
	}

	return &HeaderModel{
		hostname: hostname,
		flags:    flags,
		focus:    HeaderFocusNone,
	}
}

// Init implements tea.Model
func (m *HeaderModel) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model
func (m *HeaderModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case LayerHitMsg:
		if msg.ID == IDStatusBar {
			// Clicking the status bar background → focus App version if nothing is focused.
			if m.focus == HeaderFocusNone {
				m.focus = HeaderFocusApp
			}
			return m, nil
		}
		_, cmd := m.HandleHit(msg.ID)
		return m, cmd

	case LayerWheelMsg:
		if msg.ID == IDStatusBar {
			// Scroll wheel cycles between App and Tmpl version focus.
			if msg.Button == tea.MouseWheelUp {
				m.focus = HeaderFocusApp
			} else if msg.Button == tea.MouseWheelDown {
				m.focus = HeaderFocusTmpl
			}
			return m, nil
		}
	}

	// Middle-click (ToggleFocusedMsg) activates the currently focused version item.
	if _, ok := msg.(ToggleFocusedMsg); ok {
		switch m.focus {
		case HeaderFocusApp:
			return m, TriggerAppUpdate()
		case HeaderFocusTmpl:
			return m, TriggerTemplateUpdate()
		}
		return m, nil
	}

	return m, nil
}

// SetWidth sets the header width
func (m *HeaderModel) SetWidth(width int) {
	m.width = width
}

// Refresh updates the header (called when update status changes)
func (m *HeaderModel) Refresh() {
	// Nothing to cache currently, but could be used for update status
}

// SetFocus sets the focus state of the header
func (m *HeaderModel) SetFocus(f HeaderFocus) {
	m.focus = f
}

// GetFocus returns the current focus state
func (m *HeaderModel) GetFocus() HeaderFocus {
	return m.focus
}

// HandleHit handles a hit result from the compositor
func (m *HeaderModel) HandleHit(id string) (bool, tea.Cmd) {
	switch id {
	case IDAppVersion:
		m.SetFocus(HeaderFocusApp)
		return true, TriggerAppUpdate()
	case IDTmplVersion:
		m.SetFocus(HeaderFocusTmpl)
		return true, TriggerTemplateUpdate()
	}
	return false, nil
}

// Height returns the number of lines the header currently occupies.
func (m *HeaderModel) Height() int {
	return lipgloss.Height(m.ViewString())
}

// View implements tea.Model
func (m *HeaderModel) View() tea.View {
	return tea.NewView(m.ViewString())
}

// ViewString renders the header as a string (used by backdrop for compositing)
func (m *HeaderModel) ViewString() string {
	styles := GetStyles()

	left := m.renderLeft()
	center := m.renderCenter()
	appVer, tmplVer := m.renderVersions()

	leftW := WidthWithoutZones(left)
	centerW := WidthWithoutZones(center)
	appW := WidthWithoutZones(appVer)
	tmplW := WidthWithoutZones(tmplVer)

	// Branding (center) ideal position
	centerX := (m.width - centerW) / 2
	if centerX < 0 {
		centerX = 0
	}

	// 1. Single Line Layout: [Left] [Center] [App] [Tmpl]
	// Requirements:
	// - Left doesn't collide with Center (min 1 space)
	// - Center doesn't collide with Right (min 1 space)
	// - Right fits in terminal
	rightW := appW + tmplW
	fitsLine1 := true
	if leftW+1 > centerX {
		fitsLine1 = false
	}
	if centerX+centerW+1+rightW > m.width {
		fitsLine1 = false
	}

	if fitsLine1 {
		fullLine := left + strutil.Repeat(" ", centerX-leftW) + center + strutil.Repeat(" ", m.width-(centerX+centerW)-rightW) + appVer + tmplVer
		return MaintainBackground(fullLine, styles.HeaderBG)
	}

	// 2. Wrap Stage 1: [Left] [Center] [App] on Line 1, [Tmpl] on Line 2
	// Verify if Line 1 fits
	fitsStage1 := true
	if leftW+1 > centerX {
		fitsStage1 = false
	}
	if centerX+centerW+1+appW > m.width {
		fitsStage1 = false
	}

	if fitsStage1 {
		line1 := left + strutil.Repeat(" ", centerX-leftW) + center + strutil.Repeat(" ", m.width-(centerX+centerW)-appW) + appVer
		line2 := strutil.Repeat(" ", m.width-tmplW) + tmplVer
		return MaintainBackground(line1, styles.HeaderBG) + "\n" + MaintainBackground(line2, styles.HeaderBG)
	}

	// 3. Wrap Stage 2: [Left] [Center] on Line 1, [App] on Line 2, [Tmpl] on Line 3
	// Verify if Line 1 fits
	if leftW+1 <= centerX {
		line1 := left + strutil.Repeat(" ", centerX-leftW) + center
		line1 = line1 + strutil.Repeat(" ", m.width-WidthWithoutZones(line1))
		line2 := strutil.Repeat(" ", m.width-appW) + appVer
		line3 := strutil.Repeat(" ", m.width-tmplW) + tmplVer
		return MaintainBackground(line1, styles.HeaderBG) + "\n" +
			MaintainBackground(line2, styles.HeaderBG) + "\n" +
			MaintainBackground(line3, styles.HeaderBG)
	}

	// Fallback: Total Stacked Layout
	line1 := left
	if WidthWithoutZones(line1) < m.width {
		line1 += strutil.Repeat(" ", m.width-WidthWithoutZones(line1))
	}
	line2 := center
	if WidthWithoutZones(line2) < m.width {
		line2 += strutil.Repeat(" ", m.width-WidthWithoutZones(line2))
	}
	line3 := strutil.Repeat(" ", m.width-appW) + appVer
	line4 := strutil.Repeat(" ", m.width-tmplW) + tmplVer

	return MaintainBackground(line1, styles.HeaderBG) + "\n" +
		MaintainBackground(line2, styles.HeaderBG) + "\n" +
		MaintainBackground(line3, styles.HeaderBG) + "\n" +
		MaintainBackground(line4, styles.HeaderBG)
}

func (m HeaderModel) renderLeft() string {
	styles := GetStyles()

	// Build hostname with theme tag
	leftText := "{{|Theme_Hostname|}}" + m.hostname + "{{[-]}}"

	// Add flags if present
	if len(m.flags) > 0 {
		leftText += " {{|Theme_ApplicationFlagsBrackets|}}|{{[-]}}"
		for i, flag := range m.flags {
			if i > 0 {
				leftText += "{{|Theme_ApplicationFlagsSpace|}}|{{[-]}}"
			}
			leftText += "{{|Theme_ApplicationFlags|}}" + flag + "{{[-]}}"
		}
		leftText += "{{|Theme_ApplicationFlagsBrackets|}}|{{[-]}}"
	}

	// Translate theme tags and render with lipgloss, using header background as default
	return MaintainBackground(RenderThemeText(leftText, styles.HeaderBG), styles.HeaderBG)
}

func (m HeaderModel) renderCenter() string {
	styles := GetStyles()
	centerText := "{{|Theme_ApplicationName|}}" + version.ApplicationName + "{{[-]}}"
	return MaintainBackground(RenderThemeText(centerText, styles.HeaderBG), styles.HeaderBG)
}

// renderVersions returns app version and template version as separate strings
func (m HeaderModel) renderVersions() (appText, tmplText string) {
	styles := GetStyles()
	appVer := version.Version
	tmplVer := paths.GetTemplatesVersion()

	// Helper to render version blocks
	// format: [StatusIcon] [Label][ [Version] ]
	renderVersionBlock := func(ver string, label string, isAvailable bool, isError bool, isFocused bool) string {
		var text string

		// 1. Status Icon / Prefix
		if isError {
			text += "{{|Theme_ApplicationUpdate|}}?{{|Theme_StatusBar|}}"
		} else if isAvailable {
			text += "{{|Theme_ApplicationUpdate|}}*{{|Theme_StatusBar|}}"
		} else {
			text += " "
		}

		// 2. Label + Open Bracket (Standard or Update color)
		text += "{{|Theme_ApplicationVersion|}}" + label + ":[{{|Theme_StatusBar|}}"

		// 3. Version Number (The Interactive Part)
		var verStyled string
		if isFocused {
			verStyled = "{{|Theme_VersionSelected|}}" + ver + "{{|Theme_StatusBar|}}"
		} else if isError || isAvailable {
			verStyled = "{{|Theme_ApplicationUpdate|}}" + ver + "{{|Theme_StatusBar|}}"
		} else {
			verStyled = "{{|Theme_ApplicationVersion|}}" + ver + "{{|Theme_StatusBar|}}"
		}
		text += verStyled

		// 4. Close Bracket
		text += "{{|Theme_ApplicationVersion|}}]{{|Theme_StatusBar|}}"

		return MaintainBackground(RenderThemeText(text, styles.HeaderBG), styles.HeaderBG)
	}

	appText = renderVersionBlock(appVer, "A", update.AppUpdateAvailable, update.UpdateCheckError, m.focus == HeaderFocusApp)
	tmplText = renderVersionBlock(tmplVer, "T", update.TmplUpdateAvailable, update.UpdateCheckError, m.focus == HeaderFocusTmpl)

	return appText, tmplText
}

// GetHitRegions returns clickable regions for the header version labels
func (m *HeaderModel) GetHitRegions(offsetX, offsetY int) []HitRegion {
	var regions []HitRegion

	left := m.renderLeft()
	center := m.renderCenter()
	appVer, tmplVer := m.renderVersions()

	leftW := WidthWithoutZones(left)
	centerW := WidthWithoutZones(center)
	appW := WidthWithoutZones(appVer)
	tmplW := WidthWithoutZones(tmplVer)

	centerX := (m.width - centerW) / 2
	if centerX < 0 {
		centerX = 0
	}

	rightW := appW + tmplW
	fitsLine1 := leftW+1 <= centerX && centerX+centerW+1+rightW <= m.width

	if fitsLine1 {
		// Line 1: [Left] [Center] [App] [Tmpl]
		appX := m.width - rightW
		tmplX := m.width - tmplW
		regions = append(regions, HitRegion{ID: IDAppVersion, X: offsetX + appX, Y: offsetY, Width: appW, Height: 1, ZOrder: ZBackdrop + 1})
		regions = append(regions, HitRegion{ID: IDTmplVersion, X: offsetX + tmplX, Y: offsetY, Width: tmplW, Height: 1, ZOrder: ZBackdrop + 1})
	} else {
		fitsStage1 := leftW+1 <= centerX && centerX+centerW+1+appW <= m.width

		if fitsStage1 {
			appX := m.width - appW
			tmplX := m.width - tmplW
			regions = append(regions, HitRegion{ID: IDAppVersion, X: offsetX + appX, Y: offsetY, Width: appW, Height: 1, ZOrder: ZBackdrop + 1})
			regions = append(regions, HitRegion{ID: IDTmplVersion, X: offsetX + tmplX, Y: offsetY + 1, Width: tmplW, Height: 1, ZOrder: ZBackdrop + 1})
		} else if leftW+1 <= centerX {
			appX := m.width - appW
			tmplX := m.width - tmplW
			regions = append(regions, HitRegion{ID: IDAppVersion, X: offsetX + appX, Y: offsetY + 1, Width: appW, Height: 1, ZOrder: ZBackdrop + 1})
			regions = append(regions, HitRegion{ID: IDTmplVersion, X: offsetX + tmplX, Y: offsetY + 2, Width: tmplW, Height: 1, ZOrder: ZBackdrop + 1})
		} else {
			appX := m.width - appW
			tmplX := m.width - tmplW
			regions = append(regions, HitRegion{ID: IDAppVersion, X: offsetX + appX, Y: offsetY + 2, Width: appW, Height: 1, ZOrder: ZBackdrop + 1})
			regions = append(regions, HitRegion{ID: IDTmplVersion, X: offsetX + tmplX, Y: offsetY + 3, Width: tmplW, Height: 1, ZOrder: ZBackdrop + 1})
		}
	}

	return regions
}

// renderRight returns both versions combined (for backwards compatibility)
func (m HeaderModel) renderRight() string {
	appText, tmplText := m.renderVersions()
	return lipgloss.JoinHorizontal(lipgloss.Top, appText, tmplText)
}
