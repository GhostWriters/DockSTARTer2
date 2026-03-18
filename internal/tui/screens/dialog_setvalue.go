package screens

import (
	"strings"

	"DockSTARTer2/internal/appenv"
	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/tui"
	"DockSTARTer2/internal/tui/components/sinput"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

type setValueFocus int

const (
	setValueFocusInput  setValueFocus = iota
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

	varName string
	appName string
	appDesc string
	origVal string // original value at open time (shown in heading)

	input        sinput.Model
	inputScreenX int // abs screen X of text start; set in GetHitRegions
	inputRelY    int // row of input text within dialog (for hardware cursor)
	listAbsTopY  int // absolute screen Y of first list item; set in GetHitRegions
	opts   []appenv.VarOption
	cursor int
	offset int
	maxVis int

	focus setValueFocus
}

// newSetValueDialog constructs the Set Value dialog.
func newSetValueDialog(
	varName, appName, appDesc, origVal string,
	opts []appenv.VarOption,
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
	ti.SetStyles(tiStyles)

	return &setValueDialogModel{
		varName: varName,
		appName: appName,
		appDesc: appDesc,
		origVal: origVal,
		input:   sinput.New(ti),
		opts:    opts,
		focus:   setValueFocusInput,
		maxVis:  8,
	}
}

func (m *setValueDialogModel) Init() tea.Cmd {
	return sinput.Blink
}

func (m *setValueDialogModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	closeWith := func(result any) tea.Cmd {
		return func() tea.Msg { return tui.CloseDialogMsg{Result: result} }
	}
	submit := func() tea.Cmd {
		return closeWith(ApplyVarValueMsg{VarName: m.varName, Value: m.input.Value()})
	}
	selectOpt := func(idx int) {
		if idx >= 0 && idx < len(m.opts) {
			m.input.SetValue(m.opts[idx].Value)
			m.input.CursorEnd()
		}
	}
	hasChanges := func() bool {
		return m.input.Value() != m.origVal
	}
	cancelOrConfirm := func() tea.Cmd {
		return func() tea.Msg {
			if hasChanges() && !tui.Confirm("Discard Changes", "Discard changes to "+m.varName+"?", false) {
				return nil
			}
			return tui.CloseDialogMsg{}
		}
	}
	confirmExit := tui.ConfirmExitAction

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.recalc()
		return m, nil

	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, tui.Keys.Esc), key.Matches(msg, tui.Keys.ForceQuit):
			return m, cancelOrConfirm()

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

		case key.Matches(msg, tui.Keys.Left):
			switch m.focus {
			case setValueFocusSave:
				m.focus = setValueFocusExit
			case setValueFocusCancel:
				m.focus = setValueFocusSave
			case setValueFocusExit:
				m.focus = setValueFocusCancel
			}
			return m, nil

		case key.Matches(msg, tui.Keys.Right):
			switch m.focus {
			case setValueFocusSave:
				m.focus = setValueFocusCancel
			case setValueFocusCancel:
				m.focus = setValueFocusExit
			case setValueFocusExit:
				m.focus = setValueFocusSave
			}
			return m, nil

		case key.Matches(msg, tui.Keys.Enter):
			switch m.focus {
			case setValueFocusInput:
				return m, submit()
			case setValueFocusList:
				// Enter on list does nothing — use Space to copy preset, buttons to submit
				return m, nil
			case setValueFocusSave:
				return m, submit()
			case setValueFocusCancel:
				return m, cancelOrConfirm()
			case setValueFocusExit:
				return m, confirmExit()
			}

		case msg.String() == "space" && m.focus == setValueFocusList:
			// Space: copy preset to input but stay on list for more browsing
			selectOpt(m.cursor)
			return m, nil
		}

	case tea.MouseWheelMsg:
		// Fallback: raw wheel (e.g. keyboard-generated) scrolls the presets list.
		if msg.Button == tea.MouseWheelDown {
			m.moveCursor(+1)
		} else if msg.Button == tea.MouseWheelUp {
			m.moveCursor(-1)
		}
		return m, nil

	case tui.LayerWheelMsg:
		// Semantic wheel from model_mouse.go IDListPanel path — scroll without focus snap.
		if msg.Button == tea.MouseWheelDown {
			m.moveCursor(+1)
		} else if msg.Button == tea.MouseWheelUp {
			m.moveCursor(-1)
		}
		return m, nil

	case tea.MouseMotionMsg:
		if m.input.IsSelecting() {
			m.input.HandleDragTo(msg.X)
		}
		return m, nil

	case tea.MouseReleaseMsg:
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
				return m, submit()
			}
			if strings.HasSuffix(msg.ID, ".Cancel") {
				return m, cancelOrConfirm()
			}
			if strings.HasSuffix(msg.ID, ".Exit") {
				return m, confirmExit()
			}
			if msg.ID == "setvalue_input" {
				m.focus = setValueFocusInput
				m.input.Focus()
				m.input.HandleClick(msg.X)
				return m, nil
			}
			if msg.ID == "setvalue_list" {
				idx := m.optIndexAt(msg.Y)
				if idx >= 0 {
					selectOpt(idx)
					m.cursor = idx
					m.focus = setValueFocusList
					m.input.Blur()
				}
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
	// Scroll down: check if cursor is beyond the visible row budget from offset.
	rows := 0
	lastVisible := m.offset
	for i := m.offset; i < len(m.opts); i++ {
		r := 1
		if m.opts[i].Value != "" {
			r = 2
		}
		if rows+r > m.maxVis {
			break
		}
		rows += r
		lastVisible = i
	}
	if m.cursor > lastVisible {
		// Scroll offset forward until cursor is within the visible window.
		for m.cursor > lastVisible && m.offset < m.cursor {
			m.offset++
			rows = 0
			lastVisible = m.offset
			for i := m.offset; i < len(m.opts); i++ {
				r := 1
				if m.opts[i].Value != "" {
					r = 2
				}
				if rows+r > m.maxVis {
					break
				}
				rows += r
				lastVisible = i
			}
		}
	}
}

func (m *setValueDialogModel) HelpText() string {
	if m.focus == setValueFocusList && m.cursor >= 0 && m.cursor < len(m.opts) {
		return m.opts[m.cursor].Help
	}
	return ""
}

func (m *setValueDialogModel) SetFocused(f bool) { m.focused = f }

func (m *setValueDialogModel) SetSize(w, h int) {
	m.width = w
	m.height = h
	m.recalc()
}

func (m *setValueDialogModel) recalc() {
	ctx := tui.GetActiveContext()
	contentW := m.innerWidth()
	headingRaw := FormatMenuHeading(MenuHeadingParams{
		AppName:        m.appName,
		AppDescription: m.appDesc,
		VarName:        m.varName,
		OriginalValue:  m.origVal,
		CurrentValue:   m.input.Value(),
	}, contentW)
	// Use the actual rendered height — Padding+Width can wrap long description lines.
	headingRenderedH := lipgloss.Height(ctx.Dialog.Padding(1, 2).Width(contentW).Render(console.ToANSI(headingRaw)))
	btnH := tui.ButtonRowHeight(contentW, 0, tui.ButtonSpec{Text: "Save"}, tui.ButtonSpec{Text: "Cancel"}, tui.ButtonSpec{Text: "Exit"})
	// overhead: outer border(2) + rendered heading + "Current Value" section(3) + "Presets" section borders(2) + spacer(1) + buttons
	fixed := 2 + headingRenderedH + 3 + 2 + 1 + btnH
	m.maxVis = m.height - fixed
	if m.maxVis < 2 {
		m.maxVis = 2
	}
}

func (m *setValueDialogModel) IsMaximized() bool { return true }

func (m *setValueDialogModel) innerWidth() int {
	w := m.width - 2
	if w < 20 {
		w = 20
	}
	return w
}

func (m *setValueDialogModel) optIndexAt(screenY int) int {
	// m.listAbsTopY is set by GetHitRegions each frame to the absolute screen Y
	// of the first visible list item. Use it directly so click coordinates match.
	rowY := m.listAbsTopY
	rowBudget := m.maxVis
	for i := m.offset; i < len(m.opts) && rowBudget > 0; i++ {
		h := 1
		if m.opts[i].Value != "" {
			h = 2
		}
		if h > rowBudget {
			break
		}
		rowBudget -= h
		if screenY >= rowY && screenY < rowY+h {
			return i
		}
		rowY += h
	}
	return -1
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

	bgStyle := tui.SemanticStyle("{{|Theme_Dialog|}}")
	normalStyle := tui.SemanticStyle("{{|Theme_Item|}}")
	selectedStyle := tui.SemanticStyle("{{|Theme_ItemSelected|}}")
	subLabelStyle := tui.SemanticStyle("{{|Theme_HelpItem|}}")

	// Heading — CurrentValue updates live as the user types
	headingRaw := FormatMenuHeading(MenuHeadingParams{
		AppName:        m.appName,
		AppDescription: m.appDesc,
		VarName:        m.varName,
		OriginalValue:  m.origVal,
		CurrentValue:   m.input.Value(),
	}, contentW)
	headingText := strings.TrimRight(
		ctx.Dialog.Padding(1, 2).Width(contentW).Render(console.ToANSI(headingRaw)), "\n")

	// "Current Value" section — titled bordered box, thick border when focused
	inputFocused := m.focus == setValueFocusInput
	inputContent := strings.TrimRight(ctx.Dialog.Padding(0, 1).Width(sInnerW).Render(m.input.View()), "\n")
	inputTitleTag := "Theme_TitleSubMenu"
	if inputFocused {
		inputTitleTag = "Theme_TitleSubMenuFocused"
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
	maxItemW := sInnerW - 3 - tui.ScrollbarGutterWidth // cursor(1) + space(1) + trailing space(1) + gutter

	// Compute scroll metrics for the scrollbar / scroll indicator.
	totalRows := 0
	offsetRows := 0
	for i, opt := range m.opts {
		r := 1
		if opt.Value != "" {
			r = 2
		}
		if i < m.offset {
			offsetRows += r
		}
		totalRows += r
	}

	var listLines []string
	rowBudget := m.maxVis // m.maxVis is a row budget, not an item count
	for i := m.offset; i < len(m.opts) && rowBudget > 0; i++ {
		opt := m.opts[i]
		itemRows := 1
		if opt.Value != "" {
			itemRows = 2
		}
		if itemRows > rowBudget {
			break
		}
		rowBudget -= itemRows

		focused := i == m.cursor && listFocused
		cursor := " "
		if focused {
			cursor = ">"
		}
		label := opt.Display
		if lipgloss.Width(label) > maxItemW {
			label = tui.TruncateRight(label, maxItemW)
		}
		pad := maxItemW - lipgloss.Width(label)
		if pad < 0 {
			pad = 0
		}
		line := cursor + " " + label + strings.Repeat(" ", pad) + " "
		if focused {
			listLines = append(listLines, tui.MaintainBackground(selectedStyle.Render(line), selectedStyle))
		} else {
			listLines = append(listLines, tui.MaintainBackground(normalStyle.Background(bgStyle.GetBackground()).Render(line), bgStyle))
		}
		if opt.Value != "" {
			sl := opt.Value
			if lipgloss.Width(sl) > maxItemW {
				sl = tui.TruncateRight(sl, maxItemW)
			}
			slPad := maxItemW - lipgloss.Width(sl)
			if slPad < 0 {
				slPad = 0
			}
			slLine := "  " + sl + strings.Repeat(" ", slPad) + " "
			if focused {
				listLines = append(listLines, tui.MaintainBackground(selectedStyle.Render(slLine), selectedStyle))
			} else {
				listLines = append(listLines, tui.MaintainBackground(subLabelStyle.Background(bgStyle.GetBackground()).Render(slLine), bgStyle))
			}
		}
	}

	// Apply scrollbar column (always reserves the gutter, shows track+thumb when enabled+needed).
	listContent, sbInfo := tui.ApplyScrollbarColumnTracked(
		strings.Join(listLines, "\n"),
		totalRows, m.maxVis, offsetRows,
		tui.IsScrollbarEnabled(), ctx.LineCharacters, ctx,
	)

	listTitleTag := "Theme_TitleSubMenu"
	if listFocused {
		listTitleTag = "Theme_TitleSubMenuFocused"
	}
	presetsSection := strings.TrimRight(tui.RenderBorderedBoxCtx(
		"Preset Values", listContent, sInnerW, 0, listFocused, true, true,
		ctx.SubmenuTitleAlign, listTitleTag, ctx,
	), "\n")

	// Replace bottom border with scroll indicator when list overflows.
	if sbInfo.Needed {
		scrollPct := 0.0
		if totalRows > m.maxVis {
			scrollPct = float64(offsetRows) / float64(totalRows-m.maxVis)
			if scrollPct > 1.0 {
				scrollPct = 1.0
			}
		}
		presetLines := strings.Split(presetsSection, "\n")
		if len(presetLines) > 0 {
			bottomLine := tui.BuildScrollPercentBottomBorder(sInnerW+2, scrollPct, listFocused, ctx)
			presetLines[len(presetLines)-1] = bottomLine
			presetsSection = strings.Join(presetLines, "\n")
		}
	}

	// Render buttons first so we know the actual height (1 flat or 3 bordered).
	buttonRow := strings.TrimRight(tui.RenderCenteredButtonsCtx(
		contentW, ctx,
		tui.ButtonSpec{Text: "Save", Active: m.focus == setValueFocusInput || m.focus == setValueFocusSave},
		tui.ButtonSpec{Text: "Cancel", Active: m.focus == setValueFocusCancel},
		tui.ButtonSpec{Text: "Exit", Active: m.focus == setValueFocusExit},
	), "\n")

	// Dynamic spacer pushes buttons to the bottom.
	headingRenderedH := lipgloss.Height(headingText)
	currentValueSectionH := lipgloss.Height(currentValueSection)
	presetsSectionH := lipgloss.Height(presetsSection)
	buttonRowH := lipgloss.Height(buttonRow)
	spacerH := m.height - 2 - headingRenderedH - currentValueSectionH - presetsSectionH - buttonRowH
	if spacerH < 1 {
		spacerH = 1
	}
	spacer := strings.TrimRight(strings.Repeat(bgStyle.Width(contentW).Render("")+"\n", spacerH), "\n")

	title := "Set Value: " + m.varName
	parts := []string{headingText, currentValueSection, presetsSection, spacer, buttonRow}
	return tui.RenderDialogWithType(title, lipgloss.JoinVertical(lipgloss.Left, parts...), m.focused, m.height, tui.DialogTypeInfo)
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

	headingRaw := FormatMenuHeading(MenuHeadingParams{
		AppName: m.appName, AppDescription: m.appDesc,
		VarName: m.varName, OriginalValue: m.origVal, CurrentValue: m.input.Value(),
	}, contentW)
	headingH := lipgloss.Height(ctx.Dialog.Padding(1, 2).Width(contentW).Render(console.ToANSI(headingRaw)))
	// outer border(1) + headingH + "Current Value" section(3) + "Presets" title border(1)
	listTop := 1 + headingH + 3 + 1

	listH := 0
	rowBudget := m.maxVis
	for i := m.offset; i < len(m.opts) && rowBudget > 0; i++ {
		h := 1
		if m.opts[i].Value != "" {
			h = 2
		}
		if h > rowBudget {
			break
		}
		rowBudget -= h
		listH += h
	}

	// Input hit region: outer_border(1) + headingH + section_top_border(1) = input row Y
	// Text starts at: outer_border(1) + section_border(1) + padding(1) + promptW
	inputY := 1 + headingH + 1
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
	})
	m.listAbsTopY = offsetY + listTop
	if listH > 0 {
		regions = append(regions, tui.HitRegion{
			ID:     "setvalue_list",
			X:      offsetX + 1,
			Y:      offsetY + listTop,
			Width:  contentW + 2,
			Height: listH,
			ZOrder: tui.ZDialog + 10,
		})
	}

	// buttons are at the bottom: outer border(1) + content fills to m.height-2, bottom border at m.height-1
	btnH := tui.ButtonRowHeight(contentW, 0, tui.ButtonSpec{Text: "Save"}, tui.ButtonSpec{Text: "Cancel"}, tui.ButtonSpec{Text: "Exit"})
	buttonY := m.height - 1 - btnH
	regions = append(regions, tui.GetButtonHitRegions(
		"setvalue_dialog", offsetX+1, offsetY+buttonY, contentW, tui.ZDialog+10,
		tui.ButtonSpec{Text: "Save", ZoneID: "Save"},
		tui.ButtonSpec{Text: "Cancel", ZoneID: "Cancel"},
		tui.ButtonSpec{Text: "Exit", ZoneID: "Exit"},
	)...)

	return regions
}
