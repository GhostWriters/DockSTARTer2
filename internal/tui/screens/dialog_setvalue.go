package screens

import (
	"strings"
	"time"

	"DockSTARTer2/internal/appenv"
	"DockSTARTer2/internal/theme"
	"DockSTARTer2/internal/tui"
	"DockSTARTer2/internal/tui/components/sinput"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

type setValueFocus int

const (
	setValueFocusInput setValueFocus = iota
	setValueFocusList
	setValueFocusSave
	setValueFocusCancel
	setValueFocusExit
)

// setValueDialogModel is the "Set Value" dialog (F2) that mimics bash menu_value_prompt.sh.
// It shows a FormatMenuHeading block (with live current-value update), a text input
// pre-filled with the current value, and a scrollable list of preset options.
type setValueDialogModel struct {
	width   int
	height  int
	focused bool

	tui.TitleBarFocus
	buttons *tui.ButtonRow

	varName     string
	appName     string
	appDesc     string
	filePath    string // shown in heading; empty = omit
	origVal     string // original value at open time (shown in heading)
	varHelp     string
	docMarkdown string
	docAppName  string

	input        sinput.Model
	inputScreenX int // abs screen X of text start; set in GetHitRegions
	inputRelY    int // row of input text within dialog (for hardware cursor)
	sbAbsTopY    int // absolute screen Y of scrollbar top (up arrow); set in GetHitRegions
	sbDrag       tui.ScrollbarDragState
	lastSbInfo   tui.ScrollbarInfo
	opts         []appenv.VarOption
	cursor       int
	offset       int
	maxVis       int

	focus    setValueFocus
	onSave   func(string) tea.Cmd // non-nil in standalone mode: write value directly
	onCancel tea.Cmd              // non-nil in standalone mode: quit instead of CloseDialogMsg{}
}

// newSetValueDialog constructs the Set Value dialog.
// onSave and onCancel are nil in normal (tabbed editor) mode.
// In standalone CLI mode, onSave writes the value to the env file and quits;
// onCancel is tea.Quit so Cancel/Esc exits the TUI instead of returning to a parent screen.
func newSetValueDialog(
	varName, appName, appDesc, filePath, origVal string,
	opts []appenv.VarOption,
	helpText, docMarkdown, docAppName string,
	onSave func(string) tea.Cmd,
	onCancel tea.Cmd,
) *setValueDialogModel {
	ti := textinput.New()
	ti.SetValue(origVal)
	ti.CursorEnd()
	ti.CharLimit = 512
	ti.Focus()

	styles := tui.GetStyles()
	bg := styles.Dialog.GetBackground()
	tiStyles := textinput.DefaultStyles(true)
	tiStyles.Focused.Prompt = styles.ItemNormal.Background(bg)
	tiStyles.Focused.Text = styles.ItemNormal.Background(bg)
	tiStyles.Blurred.Prompt = styles.ItemNormal.Background(bg)
	tiStyles.Blurred.Text = styles.ItemNormal.Background(bg)
	tiStyles.Cursor.Color = tui.TextCursorColor()
	ti.SetStyles(tiStyles)

	m := &setValueDialogModel{
		varName:     varName,
		appName:     appName,
		appDesc:     appDesc,
		filePath:    filePath,
		origVal:     origVal,
		varHelp:     helpText,
		docMarkdown: docMarkdown,
		docAppName:  docAppName,
		input:       sinput.New(ti),
		opts:        opts,
		focus:       setValueFocusInput,
		maxVis:      8,
		onSave:      onSave,
		onCancel:    onCancel,
	}
	m.buttons = tui.NewButtonRow([]tui.ButtonDef{
		{Label: "Save", ZoneID: "Save"},
		{Label: "Cancel", ZoneID: "Cancel"},
		{Label: "Exit", ZoneID: "Exit"},
	})
	return m
}

// setValueTitle returns the dialog title, prefixing with the app nicename for app-specific files.
func (m *setValueDialogModel) setValueTitle() string {
	if m.appName != "" && strings.Contains(m.filePath, ".env.app.") {
		return "Set Value: " + m.appName + ":" + m.varName
	}
	return "Set Value: " + m.varName
}

// Title implements tui.ScreenModel for standalone use.
func (m *setValueDialogModel) Title() string { return m.setValueTitle() }

// MenuName implements tui.ScreenModel; the standalone var editor has no menu alias.
func (m *setValueDialogModel) MenuName() string { return "" }

// HasDialog implements tui.ScreenModel.
func (m *setValueDialogModel) HasDialog() bool { return false }

func (m *setValueDialogModel) IsDestructive() bool {
	return true
}

func (m *setValueDialogModel) Init() tea.Cmd {
	return sinput.Blink
}

func (m *setValueDialogModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if tickCmd, ok := m.buttons.Update(msg); ok {
		return m, tickCmd
	}

	if m.HandleWidgetClearPress(msg) {
		return m, nil
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.recalc()
		return m, nil

	case tea.KeyPressMsg:
		if handled, cmd := m.HandleTitleBarKey(msg, nil); handled {
			m.focus = setValueFocusCancel
			return m, tea.Batch(cmd, m.buttons.SetProcessing("Cancel", m.cancelOrConfirm()))
		}
		switch {
		case key.Matches(msg, tui.Keys.Esc):
			m.focus = setValueFocusCancel
			return m, m.buttons.SetProcessing("Cancel", m.cancelOrConfirm())
		case key.Matches(msg, tui.Keys.ForceQuit):
			return m, m.cancelOrConfirm()

		case key.Matches(msg, tui.Keys.Tab), key.Matches(msg, tui.Keys.CycleTab):
			m.cycleFocus(+1)
			return m, nil

		case key.Matches(msg, tui.Keys.ShiftTab), key.Matches(msg, tui.Keys.CycleShiftTab):
			m.cycleFocus(-1)
			return m, nil

		case key.Matches(msg, tui.Keys.Up):
			if m.focus == setValueFocusList {
				m.moveCursor(-1)
			}
			return m, nil

		case key.Matches(msg, tui.Keys.Down):
			if m.focus == setValueFocusList {
				m.moveCursor(+1)
			}
			return m, nil

		case key.Matches(msg, tui.Keys.Left) && m.focus != setValueFocusInput:
			switch m.focus {
			case setValueFocusSave:
				m.focus = setValueFocusExit
			case setValueFocusCancel:
				m.focus = setValueFocusSave
			case setValueFocusExit:
				m.focus = setValueFocusCancel
			default: // list — jump to last button
				m.focus = setValueFocusExit
			}
			return m, nil

		case key.Matches(msg, tui.Keys.Right) && m.focus != setValueFocusInput:
			switch m.focus {
			case setValueFocusSave:
				m.focus = setValueFocusCancel
			case setValueFocusCancel:
				m.focus = setValueFocusExit
			case setValueFocusExit:
				m.focus = setValueFocusSave
			default: // list — jump to first button
				m.focus = setValueFocusSave
			}
			return m, nil

		case key.Matches(msg, tui.Keys.Enter):
			switch m.focus {
			case setValueFocusInput:
				m.focus = setValueFocusSave
				return m, m.buttons.SetProcessing("Save", m.submit())
			case setValueFocusList:
				return m, nil
			case setValueFocusSave:
				return m, m.buttons.SetProcessing("Save", m.submit())
			case setValueFocusCancel:
				return m, m.buttons.SetProcessing("Cancel", m.cancelOrConfirm())
			case setValueFocusExit:
				return m, m.buttons.SetProcessing("Exit", m.confirmExit())
			}

		case msg.String() == "space" && m.focus == setValueFocusList:
			// Space: copy preset to input but stay on list for more browsing
			m.selectOpt(m.cursor)
			return m, nil
		}

	case tui.DragDoneMsg:
		if m.sbDrag.Dragging && msg.ID == "setvalue_preset_box" {
			m.sbDrag.DragPending = false
			// Catch up to any position skipped while the render was in-flight.
			if m.sbDrag.PendingDragY != m.sbDrag.LastDragY {
				lastY := m.sbDrag.PendingDragY
				if m.applySbDrag(lastY) {
					m.sbDrag.LastDragY = lastY
					m.sbDrag.DragPending = true
					return m, tui.DragDoneCmd("setvalue_preset_box")
				}
				m.sbDrag.LastDragY = lastY
			}
		}
		return m, nil

	case tea.MouseWheelMsg:
		// Fallback: raw wheel (e.g. keyboard-generated) scrolls the presets list.
		switch msg.Button {
		case tea.MouseWheelDown:
			m.moveCursor(+1)
		case tea.MouseWheelUp:
			m.moveCursor(-1)
		}
		return m, nil

	case tui.LayerWheelMsg:
		// Semantic wheel from model_mouse.go IDListPanel path — scroll without focus snap.
		switch msg.Button {
		case tea.MouseWheelDown:
			m.moveCursor(+1)
		case tea.MouseWheelUp:
			m.moveCursor(-1)
		}
		return m, nil

	case tea.MouseMotionMsg:
		if m.sbDrag.Dragging {
			m.sbDrag.PendingDragY = msg.Y // always record latest, even if render in-flight
			if !m.sbDrag.DragPending {
				if m.applySbDrag(msg.Y) {
					m.sbDrag.LastDragY = msg.Y
					m.sbDrag.DragPending = true
					return m, tui.DragDoneCmd("setvalue_preset_box")
				}
			}
		}
		if m.input.IsSelecting() {
			m.input.HandleDragTo(msg.X)
		}
		return m, nil

	case tea.MouseReleaseMsg:
		if m.sbDrag.Dragging {
			m.sbDrag.StopDrag()
			return m, nil
		}
		m.input.EndDrag()
		return m, nil

	case sinput.PasteMsg, sinput.CutMsg, sinput.SelectAllMsg:
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd

	case tui.LayerHitMsg:
		if msg.Button == tea.MouseMiddle {
			return m, nil
		}
		if msg.Button == tea.MouseLeft && strings.HasSuffix(msg.ID, "."+tui.IDInsOvr) {
			m.input.ToggleOverwrite()
			return m, nil
		}
		if msg.Button == tea.MouseLeft && strings.HasSuffix(msg.ID, "."+tui.IDTitleWidgetClose) {
			m.BlurTitleBar()
			m.focus = setValueFocusCancel
			return m, m.buttons.SetProcessing("Cancel", m.cancelOrConfirm())
		}
		if msg.Button == tea.MouseLeft && strings.HasSuffix(msg.ID, "."+tui.IDTitleWidgetHelp) {
			m.BlurTitleBar()
			return m, func() tea.Msg { return tui.TriggerHelpMsg{ScreenLevelOnly: true} }
		}
		if msg.Button == tea.MouseRight && msg.ID == "setvalue_input" {
			return m, tui.ShowInputContextMenu(m.input, msg.X, msg.Y, m.width, m.height)
		}
		// Hover focus from wheel routing: switch focus to list without selecting.
		if msg.Button == tui.HoverButton && msg.ID == tui.IDListPanel {
			m.focus = setValueFocusList
			m.input.Blur()
			return m, nil
		}
		if msg.Button == tea.MouseLeft {
			if strings.HasSuffix(msg.ID, ".Save") {
				m.focus = setValueFocusSave
				return m, m.buttons.SetProcessing("Save", m.submit())
			}
			if strings.HasSuffix(msg.ID, ".Cancel") {
				m.focus = setValueFocusCancel
				return m, m.buttons.SetProcessing("Cancel", m.cancelOrConfirm())
			}
			if strings.HasSuffix(msg.ID, ".Exit") {
				m.focus = setValueFocusExit
				return m, m.buttons.SetProcessing("Exit", m.confirmExit())
			}
			if msg.ID == "setvalue_input" {
				m.focus = setValueFocusInput
				m.input.Focus()
				m.input.HandleClick(msg.X)
				return m, nil
			}
			if msg.ID == "setvalue_list" {
				m.focus = setValueFocusList
				m.input.Blur()
				idx := m.optIndexAt(msg.Y)
				if idx >= 0 {
					m.cursor = idx
					m.selectOpt(idx)
				}
				return m, nil
			}
			if msg.ID == "setvalue_preset_box" {
				m.focus = setValueFocusList
				m.input.Blur()
				return m, nil
			}

			// Granular scrollbar interaction:
			newOff, changed := tui.HandleScrollbarLayerHit("setvalue_preset_box", msg, m.offset, len(m.opts), m.maxVis)
			if changed {
				m.focus = setValueFocusList
				m.input.Blur()
				m.offset = newOff
				return m, nil
			}
			if strings.HasSuffix(msg.ID, ".sb.thumb") && strings.HasPrefix(msg.ID, "setvalue_preset_box") {
				m.focus = setValueFocusList
				m.input.Blur()
				m.sbDrag.StartDrag(msg.Y, m.sbAbsTopY, m.lastSbInfo)
				return m, nil
			}
		}
	}

	if m.focus == setValueFocusInput {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m *setValueDialogModel) cycleFocus(dir int) {
	order := []setValueFocus{setValueFocusInput, setValueFocusList, setValueFocusSave, setValueFocusCancel, setValueFocusExit}
	cur := 0
	for i, f := range order {
		if f == m.focus {
			cur = i
			break
		}
	}
	next := ((cur + dir) + len(order)) % len(order)
	if order[next] == setValueFocusList && len(m.opts) == 0 {
		next = ((next + dir) + len(order)) % len(order)
	}
	m.focus = order[next]
	if m.focus == setValueFocusInput {
		m.input.Focus()
	} else {
		m.input.Blur()
	}
}

func (m *setValueDialogModel) moveCursor(delta int) {
	m.cursor += delta
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor >= len(m.opts) {
		m.cursor = len(m.opts) - 1
	}
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	// Scroll down: check if cursor is beyond the visible window.
	lastVisible := m.offset + m.maxVis - 1
	if lastVisible >= len(m.opts) {
		lastVisible = len(m.opts) - 1
	}
	if m.cursor > lastVisible {
		m.offset = m.cursor - m.maxVis + 1
		if m.offset < 0 {
			m.offset = 0
		}
	}
}

func (m *setValueDialogModel) HelpText() string {
	if m.focus == setValueFocusList && m.cursor >= 0 && m.cursor < len(m.opts) {
		return m.opts[m.cursor].Help
	}
	return ""
}

func (m *setValueDialogModel) HelpContext(maxWidth int) tui.HelpContext {
	pageText := m.appDesc
	if m.varHelp != "" {
		if pageText != "" {
			pageText += "\n\n"
		}
		pageText += m.varHelp
	}

	h := tui.HelpContext{
		ScreenName:  m.setValueTitle(),
		PageTitle:   "Variable Info",
		PageText:    pageText,
		Legend:      "| {{|MarkerAdded|}}+{{[-]}} Added | {{|MarkerDeleted|}}-{{[-]}} Deleted | {{|MarkerModified|}}~{{[-]}} Changed | {{|MarkerInvalid|}}!{{[-]}} Invalid |",
		DocMarkdown: m.docMarkdown,
		DocAppName:  m.docAppName,
	}

	switch m.focus {
	case setValueFocusInput:
		h.PageTitle = "Editing"
		h.PageText = "Type to enter a custom value for " + m.varName + "."
		h.ItemText = "Press Enter to save or Esc to cancel."
	case setValueFocusList:
		h.ItemTitle = "Preset Values"
		if m.cursor >= 0 && m.cursor < len(m.opts) {
			h.ItemText = "Selected: " + m.opts[m.cursor].Display + "\n\n" + m.opts[m.cursor].Help
		}
	case setValueFocusSave:
		h.ItemTitle = "Save"
		h.ItemText = "Save the current value and return."
	case setValueFocusCancel:
		h.ItemTitle = "Cancel"
		h.ItemText = "Cancel and return to the editor."
	case setValueFocusExit:
		h.ItemTitle = "Exit"
		h.ItemText = "Close the editor and return to the main menu."
	}

	return h
}

func (m *setValueDialogModel) SetFocused(f bool) { m.focused = f }

func (m *setValueDialogModel) IsScrollbarDragging() bool { return m.sbDrag.Dragging }

func (m *setValueDialogModel) applySbDrag(mouseY int) bool {
	total := len(m.opts)
	visible := m.maxVis
	maxOff := total - visible
	if maxOff < 0 {
		maxOff = 0
	}
	newOff, _ := m.sbDrag.ScrollOffset(mouseY, m.sbAbsTopY, maxOff, m.lastSbInfo)
	m.offset = newOff
	// Sync selection (cursor) to stay within the new visible range
	maxIdx := len(m.opts) - 1
	if m.cursor < m.offset {
		m.cursor = m.offset
	} else if m.cursor >= m.offset+m.maxVis {
		m.cursor = m.offset + m.maxVis - 1
		if m.cursor > maxIdx {
			m.cursor = maxIdx
		}
	}
	return true
}

func (m *setValueDialogModel) SetSize(w, h int) {
	m.width = w
	m.height = h
	m.recalc()
}

func (m *setValueDialogModel) recalc() {
	ctx := tui.GetActiveContext()
	contentW := m.innerWidth()

	// Robust layout calculation: Render components at the current width to get their true heights.
	headingRaw := FormatMenuHeading(MenuHeadingParams{
		AppName:        m.appName,
		AppDescription: m.appDesc,
		FilePath:       m.filePath,
		VarName:        m.varName,
		OriginalValue:  m.origVal,
		CurrentValue:   m.input.Value(),
	}, contentW)
	headingStyle := ctx.Dialog.Padding(1, 2).Width(contentW).Border(lipgloss.Border{})
	headingH := lipgloss.Height(headingStyle.Render(theme.ToANSI(headingRaw, "")))

	// "Current Value" input section height
	// Top border(1) + InputRow(1) + Bottom border(1) = 3
	currentValueH := 3

	// Buttons height
	btnH := tui.ButtonRowHeight(contentW, 0, tui.ButtonSpec{Text: "Save"}, tui.ButtonSpec{Text: "Cancel"}, tui.ButtonSpec{Text: "Exit"})

	// Total overhead:
	// - outer dialog border top + bottom: 2
	// - large titlebar extra rows (when enabled): LargeTitleBarOverhead
	// - rendered heading: headingH
	// - "Current Value" input section: currentValueH
	// - "Presets" section borders: 2
	// - buttons: btnH
	titleBudget := m.height - 2 - headingH - currentValueH - 2 - btnH
	useLarge, _ := tui.DecideLargeTitleBar(ctx.LargeTitleBars, titleBudget, 3)
	largeTitleOverhead := 0
	if useLarge {
		largeTitleOverhead = tui.LargeTitleBarOverhead
	}
	overhead := 2 + largeTitleOverhead + headingH + currentValueH + 2 + btnH
	m.maxVis = m.height - overhead
	if m.maxVis < 2 {
		m.maxVis = 2
	}
	// Re-validate cursor/offset against the new row budget so the scrollbar
	// thumb is correct immediately after a terminal resize.
	if len(m.opts) > 0 {
		m.moveCursor(0)
	}
}

func (m *setValueDialogModel) IsMaximized() bool { return true }

// MinHeight returns the minimum content-area height for the Set Value dialog.
// Breakdown: outer border(2) + heading min(3) + currentValue box(3) + presets min(5: border+3 items+border) + flat buttons(1) = 14.
// Increases by LargeTitleBarOverhead when large titlebars are enabled.
func (m *setValueDialogModel) MinHeight() int {
	base := 14
	if tui.GetActiveContext().LargeTitleBars {
		base += tui.LargeTitleBarOverhead
	}
	return base
}

func (m *setValueDialogModel) innerWidth() int {
	w := m.width - 2
	if w < 20 {
		w = 20
	}
	return w
}

func (m *setValueDialogModel) optIndexAt(screenY int) int {
	// m.sbAbsTopY is set by GetHitRegions each frame to the absolute screen Y
	// of the first visible list item. Use it directly so click coordinates match.
	idx := screenY - m.sbAbsTopY
	if idx < 0 || idx >= m.maxVis {
		return -1
	}
	i := m.offset + idx
	if i >= len(m.opts) {
		return -1
	}
	return i
}

// GetInputCursor returns the hardware cursor position relative to the dialog's
// top-left corner, cursor shape, and whether to show it.
func (m *setValueDialogModel) GetInputCursor() (relX, relY int, shape tea.CursorShape, ok bool) {
	if m.focus != setValueFocusInput || !m.input.Focused() {
		return 0, 0, tea.CursorBar, false
	}
	relX = 3 + m.input.CursorColumn()
	relY = m.inputRelY
	if m.input.IsOverwrite() {
		shape = tea.CursorBlock
	} else {
		shape = tea.CursorBar
	}
	return relX, relY, shape, true
}

func (m *setValueDialogModel) ViewString() string {
	if m.width == 0 {
		return ""
	}
	ctx := tui.GetActiveContext()
	contentW := m.innerWidth()
	sInnerW := contentW - 2 // inner width of each section (section border adds 2)

	// Heading — CurrentValue updates live as the user types
	headingRaw := FormatMenuHeading(MenuHeadingParams{
		AppName:        m.appName,
		AppDescription: m.appDesc,
		FilePath:       m.filePath,
		VarName:        m.varName,
		OriginalValue:  m.origVal,
		CurrentValue:   m.input.Value(),
	}, contentW)
	headingText := strings.TrimRight(
		ctx.Dialog.Padding(1, 2).Width(contentW).Render(theme.ToANSI(headingRaw, "")), "\n")

	// "Current Value" section — titled bordered box, thick border when focused
	inputFocused := m.focus == setValueFocusInput
	inputContent := strings.TrimRight(ctx.Dialog.Padding(0, 1).Width(sInnerW).Render(m.input.View()), "\n")
	inputTitleTag := "TitleSubMenu"
	if inputFocused {
		inputTitleTag = "TitleSubMenuFocused"
	}
	currentValueSection := strings.TrimRight(tui.RenderBorderedBoxCtx(
		"Current Value", inputContent, sInnerW, 0, inputFocused, true, true,
		ctx.SubmenuTitleAlign, inputTitleTag, ctx,
	), "\n")
	// Inject INS/OVR label into the bottom-left of the Current Value section border.
	modeLabel := "INS"
	if m.input.IsOverwrite() {
		modeLabel = "OVR"
	}
	cvLines := strings.Split(currentValueSection, "\n")
	if len(cvLines) > 0 {
		cvLines[len(cvLines)-1] = tui.BuildLabeledBottomBorderCtx(sInnerW+2, modeLabel, inputFocused, ctx)
		currentValueSection = strings.Join(cvLines, "\n")
	}

	// "Preset Values" section — titled bordered box, thick border when focused
	listFocused := m.focus == setValueFocusList
	// Always reserve one column for the scrollbar gutter so width is stable.
	maxItemW := sInnerW - 2 // cursor(1) + space(1) + scrollbar appended by ApplyScrollbarColumnTracked(1)

	// Scroll metrics — each option is always 1 row.
	totalRows := len(m.opts)
	offsetRows := m.offset

	// Compute max label width across all options for column alignment
	maxLabelW := 0
	for _, opt := range m.opts {
		if w := lipgloss.Width(opt.Display); w > maxLabelW {
			maxLabelW = w
		}
	}
	// Cap so label+gap+value fits within maxItemW
	if maxLabelW > maxItemW-4 {
		maxLabelW = maxItemW - 4
	}

	// Button row — rendered before presets so we can derive the presets height budget.
	svBtnSpecs := m.buttons.ApplySpinner([]tui.ButtonSpec{
		{Text: "Save", Active: m.focus == setValueFocusSave || m.buttons.IsProcessingID("Save"), ZoneID: "Save"},
		{Text: "Cancel", Active: m.focus == setValueFocusCancel || m.buttons.IsProcessingID("Cancel"), ZoneID: "Cancel"},
		{Text: "Exit", Active: m.focus == setValueFocusExit || m.buttons.IsProcessingID("Exit"), ZoneID: "Exit"},
	})
	buttonRow := strings.TrimRight(tui.RenderCenteredButtonsCtx(contentW, ctx, svBtnSpecs...), "\n")

	// Size the presets box to fill all remaining space above the buttons.
	buttonRowH := lipgloss.Height(buttonRow)
	headingH := lipgloss.Height(headingText)
	currentValueH := lipgloss.Height(currentValueSection)
	titleBudget2 := m.height - 2 - headingH - currentValueH - buttonRowH - 2
	useLarge2, _ := tui.DecideLargeTitleBar(ctx.LargeTitleBars, titleBudget2, 3)
	largeTitleOverhead := 0
	if useLarge2 {
		largeTitleOverhead = tui.LargeTitleBarOverhead
	}
	// Sync with recalc() logic:
	// presetTargetH is the total physical height of the "Preset Values" box.
	// We subtract outer borders (2), large titlebar extra rows, heading, current value box, and buttons.
	presetTargetH := m.height - 2 - largeTitleOverhead - headingH - currentValueH - buttonRowH
	if presetTargetH < 3 {
		presetTargetH = 3
	}
	// The logical item budget (m.maxVis) should always be exactly inner physical height.
	m.maxVis = presetTargetH - 2
	if m.maxVis < 1 {
		m.maxVis = 1
	}

	var listLines []string
	end := m.offset + m.maxVis
	if end > len(m.opts) {
		end = len(m.opts)
	}
	for i := m.offset; i < end; i++ {
		opt := m.opts[i]
		focused := i == m.cursor && listFocused
		listLines = append(listLines, RenderTwoColumnRow(
			opt.Display, opt.Value,
			i == m.cursor, focused,
			maxLabelW, maxItemW, ctx,
		))
	}

	var sbInfo tui.ScrollbarInfo
	presetsSection, sbInfo := RenderListInBorderedBox(
		"Preset Values", listLines,
		totalRows, m.maxVis, offsetRows,
		sInnerW, presetTargetH, listFocused, ctx,
	)
	m.lastSbInfo = sbInfo

	title := m.setValueTitle()
	parts := []string{headingText, currentValueSection, presetsSection, buttonRow}
	return tui.RenderDialogWithTypeAndWidgets(title, lipgloss.JoinVertical(lipgloss.Left, parts...), m.focused || m.TitleBarFocused(), m.height, tui.DialogTypeInfo, tui.TitleBarState{Show: true, Focused: m.TitleBarFocused(), ActiveWidget: m.ActiveWidget(), PressedWidget: m.PressedWidget()})
}

func (m *setValueDialogModel) View() tea.View {
	return tea.NewView(m.ViewString())
}

func (m *setValueDialogModel) Layers() []*lipgloss.Layer {
	return []*lipgloss.Layer{
		lipgloss.NewLayer(m.ViewString()).Z(tui.ZScreen).ID("setvalue_dialog"),
	}
}

func (m *setValueDialogModel) GetHitRegions(offsetX, offsetY int) []tui.HitRegion {
	ctx := tui.GetActiveContext()
	contentW := m.innerWidth()

	largeTitleOverhead := 0
	if ctx.LargeTitleBars {
		largeTitleOverhead = tui.LargeTitleBarOverhead
	}

	headingRaw := FormatMenuHeading(MenuHeadingParams{
		AppName: m.appName, AppDescription: m.appDesc,
		FilePath: m.filePath,
		VarName:  m.varName, OriginalValue: m.origVal, CurrentValue: m.input.Value(),
	}, contentW)
	headingH := lipgloss.Height(ctx.Dialog.Padding(1, 2).Width(contentW).Render(theme.ToANSI(headingRaw, "")))
	// outer border(1) + largeTitleOverhead + headingH + "Current Value" section(3)
	listTop := 1 + largeTitleOverhead + headingH + 3

	// Cover the full preset content area (including blank rows) so clicking
	// anywhere in the box focuses the list.
	// Input hit region: outer_border(1) + largeTitleOverhead + headingH + section_top_border(1) = input row Y
	// Text starts at: outer_border(1) + section_border(1) + padding(1) + promptW
	inputY := 1 + largeTitleOverhead + headingH + 1
	m.inputRelY = inputY
	m.inputScreenX = offsetX + 1 + 1 + 1 + m.input.PromptWidth()
	m.input.SetScreenTextX(m.inputScreenX)

	var regions []tui.HitRegion
	regions = append(regions, tui.HitRegion{
		ID:     "setvalue_input",
		X:      offsetX + 1,
		Y:      offsetY + inputY,
		Width:  contentW,
		Height: 1,
		ZOrder: tui.ZDialog + 10,
		Label:  "Value Input",
		Help: &tui.HelpContext{
			ScreenName:  m.setValueTitle(),
			PageTitle:   "Editing",
			PageText:    "Type to enter a custom value for " + m.varName + ".",
			ItemText:    "Press Enter to save or Esc to cancel.",
			DocMarkdown: m.docMarkdown,
			DocAppName:  m.docAppName,
		},
	})
	// INS/OVR hit region — bottom-left of the "Current Value" section border.
	// inputY = outer_border(1) + largeTitleOverhead + headingH + section_top_border(1); bottom border = inputY+1
	insOvrY := 1 + largeTitleOverhead + headingH + 2
	regions = append(regions, tui.HitRegion{
		ID:     "setvalue_dialog." + tui.IDInsOvr,
		X:      offsetX + 2, // outer border(1) + section border corner(1)
		Y:      offsetY + insOvrY,
		Width:  3,
		Height: 1,
		ZOrder: tui.ZDialog + 15,
		Label:  "INS/OVR",
		Help:   &tui.HelpContext{ScreenName: m.setValueTitle(), PageTitle: "Insert/Overwrite", PageText: "Toggle between insert and overwrite mode."},
	})

	btnH := tui.ButtonRowHeight(contentW, 0, tui.ButtonSpec{Text: "Save"}, tui.ButtonSpec{Text: "Cancel"}, tui.ButtonSpec{Text: "Exit"})
	// Use exactly the same layout math as ViewString()
	presetTargetH := m.height - 2 - headingH - 3 - btnH

	// Physical inner height: presetTargetH - 2 (borders)
	innerH := presetTargetH - 2
	if innerH < 1 {
		innerH = 1
	}
	m.sbAbsTopY = offsetY + listTop + 1
	// Use the physical inner height for hit regions to match ApplyScrollbarColumnTracked padding.
	sbInfo := tui.ComputeScrollbarInfo(len(m.opts), innerH, m.offset, innerH)
	m.lastSbInfo = sbInfo

	pageText := m.appDesc
	if m.varHelp != "" {
		if pageText != "" {
			pageText += "\n\n"
		}
		pageText += m.varHelp
	}

	regions = append(regions, ListBoxHitRegions(
		"setvalue_preset_box", "setvalue_list",
		offsetX+1, offsetY+listTop,
		contentW, innerH,
		tui.ZDialog+5,
		"Preset Values",
		sbInfo,
		&tui.HelpContext{
			ScreenName:  m.setValueTitle(),
			PageTitle:   "Variable Info",
			PageText:    pageText,
			DocMarkdown: m.docMarkdown,
			DocAppName:  m.docAppName,
		},
	)...)

	// Dialog background
	regions = append(regions, tui.HitRegion{
		ID:     "setvalue_dialog",
		X:      offsetX,
		Y:      offsetY,
		Width:  m.width,
		Height: m.height,
		ZOrder: tui.ZDialog,
		Label:  "Set Value",
		Help: &tui.HelpContext{
			ScreenName:  m.setValueTitle(),
			PageTitle:   "Variable Info",
			PageText:    pageText,
			DocMarkdown: m.docMarkdown,
			DocAppName:  m.docAppName,
		},
	})

	// buttons are at the bottom: outer border(1) + content fills to m.height-2, bottom border at m.height-1
	btnH = tui.ButtonRowHeight(contentW, 0, tui.ButtonSpec{Text: "Save"}, tui.ButtonSpec{Text: "Cancel"}, tui.ButtonSpec{Text: "Exit"})
	buttonY := m.height - 1 - btnH
	regions = append(regions, tui.HitRegion{
		ID:     "setvalue_buttons",
		X:      offsetX + 1,
		Y:      offsetY + buttonY,
		Width:  contentW,
		Height: btnH,
		ZOrder: tui.ZDialog + 5,
		Label:  "Actions",
		Help: &tui.HelpContext{
			ScreenName:  m.setValueTitle(),
			PageTitle:   "Variable Info",
			PageText:    pageText,
			DocMarkdown: m.docMarkdown,
			DocAppName:  m.docAppName,
		},
	})
	regions = append(regions, tui.GetButtonHitRegions(
		tui.HelpContext{
			ScreenName:  m.setValueTitle(),
			PageTitle:   "Variable Info",
			PageText:    pageText,
			DocMarkdown: m.docMarkdown,
			DocAppName:  m.docAppName,
		},
		"setvalue_dialog", offsetX+1, offsetY+buttonY, contentW, tui.ZDialog+20,
		tui.ButtonSpec{Text: "Save", ZoneID: "Save", Help: "Save the current value and return."},
		tui.ButtonSpec{Text: "Cancel", ZoneID: "Cancel", Help: "Cancel and return to the editor."},
		tui.ButtonSpec{Text: "Exit", ZoneID: "Exit", Help: "Close the editor and return to the main menu."},
	)...)

	// Title widget regions
	const widgetTotalWidth = 7
	const endPad = 1
	widgetsStartX := offsetX + m.width - 1 - endPad - widgetTotalWidth
	widgetY := tui.TitleBarWidgetY(offsetY, ctx.LargeTitleBars)
	regions = append(regions,
		tui.HitRegion{
			ID: "setvalue_dialog." + tui.IDTitleWidgetHelp,
			X: widgetsStartX, Y: widgetY, Width: 3, Height: 1,
			ZOrder: tui.ZDialog + 25,
			Label:  "Help",
			Help:   &tui.HelpContext{ScreenName: m.setValueTitle(), PageTitle: "Help", PageText: "Open help for this dialog."},
		},
		tui.HitRegion{
			ID: "setvalue_dialog." + tui.IDTitleWidgetClose,
			X: widgetsStartX + 4, Y: widgetY, Width: 3, Height: 1,
			ZOrder: tui.ZDialog + 25,
			Label:  "Close",
			Help:   &tui.HelpContext{ScreenName: m.setValueTitle(), PageTitle: "Close", PageText: "Close this dialog."},
		},
	)

	return regions
}

func (m *setValueDialogModel) closeWith(result any) tea.Cmd {
	return func() tea.Msg { return tui.CloseDialogMsg{Result: result} }
}

func (m *setValueDialogModel) submit() tea.Cmd {
	val := m.input.Value()
	if m.onSave != nil {
		return m.onSave(val)
	}
	return m.closeWith(ApplyVarValueMsg{VarName: m.varName, Value: val})
}

// EscapeAction implements tui.EscapeActioner: same as clicking Cancel.
func (m *setValueDialogModel) EscapeAction() tea.Cmd {
	return m.cancelOrConfirm()
}

func (m *setValueDialogModel) cancelOrConfirm() tea.Cmd {
	return func() tea.Msg {
		if m.hasChanges() && !tui.Confirm("Discard Changes", "Discard changes to "+m.varName+"?", false) {
			return nil
		}
		if m.onCancel != nil {
			return m.onCancel()
		}
		return tui.CloseDialogMsg{}
	}
}

func (m *setValueDialogModel) confirmExit() tea.Cmd {
	return tui.ConfirmExitAction()
}

func (m *setValueDialogModel) selectOpt(idx int) {
	if idx >= 0 && idx < len(m.opts) {
		m.input.SetValue(m.opts[idx].Value)
		m.input.CursorEnd()
	}
}

func (m *setValueDialogModel) hasChanges() bool {
	return m.input.Value() != m.origVal
}

func (m *setValueDialogModel) AdvanceSpinners(now time.Time) bool {
	return m.buttons.AdvanceSpinner(now)
}
