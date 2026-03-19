package screens

import (
	"strings"

	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/tui"
	"DockSTARTer2/internal/tui/components/sinput"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// addVarItemKind classifies each row in the Add Variable list.
type addVarItemKind int

const (
	addVarKindTemplate  addVarItemKind = iota // prefix template — fills input on select
	addVarKindSeparator                        // non-selectable divider
	addVarKindAddAll                           // "Add All Stock Variables" action
	addVarKindStock                            // individual stock variable
)

type addVarItem struct {
	kind     addVarItemKind
	name     string // full var name or prefix (no "…")
	label    string // display text (includes "…" for templates)
	subLabel string // shown below label (default value for stock)
	help     string
}

type addVarFocus int

const (
	addVarFocusInput  addVarFocus = iota
	addVarFocusList
	addVarFocusCreate
	addVarFocusCancel
)

// addVarDialogModel is the "Add Variable" dialog that mimics bash menu_add_var.sh.
// It shows a FormatMenuHeading block, an editable text input for the new variable
// name, and a scrollable list of template prefixes and stock variables.
type addVarDialogModel struct {
	width   int
	height  int
	focused bool

	appName string
	appDesc string

	input        sinput.Model
	inputScreenX int
	inputRelY    int // row of input text within dialog (for hardware cursor)
	items  []addVarItem
	cursor int
	offset int
	maxVis int

	focus addVarFocus

	addAllVars     []string
	addAllDefaults map[string]string
}

// newAddVarDialog constructs the Add Variable dialog.
func newAddVarDialog(
	appName, appDesc string,
	templates []struct{ prefix, label, help string },
	stockItems []addVarItem,
	addAllVars []string,
	addAllDefaults map[string]string,
) *addVarDialogModel {
	ti := textinput.New()
	if len(appName) > 0 {
		ti.Placeholder = strings.ToUpper(appName) + "__"
	}
	ti.CharLimit = 256
	ti.Focus()

	styles := tui.GetStyles()
	bg := styles.Dialog.GetBackground()
	tiStyles := textinput.DefaultStyles(true)
	tiStyles.Focused.Prompt = styles.ItemNormal.Background(bg)
	tiStyles.Focused.Text = styles.ItemNormal.Background(bg)
	tiStyles.Blurred.Prompt = styles.ItemNormal.Background(bg)
	tiStyles.Blurred.Text = styles.ItemNormal.Background(bg)
	ti.SetStyles(tiStyles)

	var items []addVarItem
	for _, t := range templates {
		items = append(items, addVarItem{
			kind:  addVarKindTemplate,
			name:  t.prefix,
			label: t.label,
			help:  t.help,
		})
	}
	if len(stockItems) > 0 {
		items = append(items, addVarItem{kind: addVarKindSeparator})
		if len(addAllVars) > 0 {
			items = append(items, addVarItem{
				kind:  addVarKindAddAll,
				label: "Add All Stock Variables",
				help:  "Add all stock variables listed below with their default values.",
			})
		}
		items = append(items, stockItems...)
	}

	return &addVarDialogModel{
		appName:        appName,
		appDesc:        appDesc,
		input:          sinput.New(ti),
		items:          items,
		focus:          addVarFocusInput,
		addAllVars:     addAllVars,
		addAllDefaults: addAllDefaults,
		maxVis:         8,
	}
}

func (m *addVarDialogModel) Init() tea.Cmd {
	return sinput.Blink
}

func (m *addVarDialogModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	closeWith := func(result any) tea.Cmd {
		return func() tea.Msg { return tui.CloseDialogMsg{Result: result} }
	}

	selectItem := func(idx int) tea.Cmd {
		if idx < 0 || idx >= len(m.items) {
			return nil
		}
		item := m.items[idx]
		switch item.kind {
		case addVarKindTemplate, addVarKindStock:
			m.input.SetValue(item.name)
			m.input.CursorEnd()
			m.focus = addVarFocusInput
			m.input.Focus()
		case addVarKindAddAll:
			return closeWith(envAddAllStockMsg{vars: m.addAllVars, defaults: m.addAllDefaults})
		}
		return nil
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.recalc()
		return m, nil

	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, tui.Keys.Esc), key.Matches(msg, tui.Keys.ForceQuit):
			return m, closeWith(nil)

		case key.Matches(msg, tui.Keys.Tab), key.Matches(msg, tui.Keys.CycleTab):
			m.cycleFocus(+1)
			return m, nil

		case key.Matches(msg, tui.Keys.ShiftTab), key.Matches(msg, tui.Keys.CycleShiftTab):
			m.cycleFocus(-1)
			return m, nil

		case key.Matches(msg, tui.Keys.Up):
			if m.focus == addVarFocusList {
				m.moveCursor(-1)
			}
			return m, nil

		case key.Matches(msg, tui.Keys.Down):
			if m.focus == addVarFocusList {
				m.moveCursor(+1)
			}
			return m, nil

		case key.Matches(msg, tui.Keys.Left), key.Matches(msg, tui.Keys.Right):
			if m.focus == addVarFocusCreate {
				m.focus = addVarFocusCancel
				return m, nil
			} else if m.focus == addVarFocusCancel {
				m.focus = addVarFocusCreate
				return m, nil
			}
			// When focus is on input, fall through to input routing below

		case key.Matches(msg, tui.Keys.Enter):
			switch m.focus {
			case addVarFocusInput:
				v := strings.TrimSpace(m.input.Value())
				if v != "" {
					return m, closeWith(envAddVarMsg{key: strings.ToUpper(v)})
				}
			case addVarFocusList:
				if cmd := selectItem(m.cursor); cmd != nil {
					return m, cmd
				}
			case addVarFocusCreate:
				v := strings.TrimSpace(m.input.Value())
				if v != "" {
					return m, closeWith(envAddVarMsg{key: strings.ToUpper(v)})
				}
			case addVarFocusCancel:
				return m, closeWith(nil)
			}
			return m, nil
		}

	case tea.MouseWheelMsg:
		// Fallback: raw wheel scrolls the list.
		if msg.Button == tea.MouseWheelDown {
			m.moveCursor(+1)
		} else if msg.Button == tea.MouseWheelUp {
			m.moveCursor(-1)
		}
		return m, nil

	case tui.LayerWheelMsg:
		// Semantic wheel from IDListPanel path — scroll without focus snap.
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
		if msg.Button == tea.MouseRight && msg.ID == "addvar_input" {
			return m, tui.ShowInputContextMenu(m.input, msg.X, msg.Y, m.width, m.height)
		}
		// Hover focus from wheel routing: switch focus to list without selecting.
		if msg.Button == tui.HoverButton && msg.ID == tui.IDListPanel {
			m.focus = addVarFocusList
			m.input.Blur()
			return m, nil
		}
		if msg.Button == tea.MouseLeft {
			if strings.HasSuffix(msg.ID, ".Create") {
				v := strings.TrimSpace(m.input.Value())
				if v != "" {
					return m, closeWith(envAddVarMsg{key: strings.ToUpper(v)})
				}
				return m, nil
			}
			if strings.HasSuffix(msg.ID, ".Cancel") {
				return m, closeWith(nil)
			}
			if msg.ID == "addvar_input" {
				m.focus = addVarFocusInput
				m.input.Focus()
				m.input.HandleClick(msg.X)
				return m, nil
			}
			if msg.ID == "addvar_list" {
				idx := m.itemIndexAt(msg.Y)
				if idx >= 0 {
					if cmd := selectItem(idx); cmd != nil {
						return m, cmd
					}
					m.cursor = idx
				}
				return m, nil
			}
		}
	}

	if m.focus == addVarFocusInput {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m *addVarDialogModel) cycleFocus(dir int) {
	order := []addVarFocus{addVarFocusInput, addVarFocusList, addVarFocusCreate, addVarFocusCancel}
	cur := 0
	for i, f := range order {
		if f == m.focus {
			cur = i
			break
		}
	}
	next := ((cur + dir) + len(order)) % len(order)
	if order[next] == addVarFocusList && len(m.selectableItems()) == 0 {
		next = ((next + dir) + len(order)) % len(order)
	}
	m.focus = order[next]
	if m.focus == addVarFocusInput {
		m.input.Focus()
	} else {
		m.input.Blur()
	}
}

func (m *addVarDialogModel) selectableItems() []addVarItem {
	var out []addVarItem
	for _, item := range m.items {
		if item.kind != addVarKindSeparator {
			out = append(out, item)
		}
	}
	return out
}

func (m *addVarDialogModel) moveCursor(delta int) {
	// Move cursor, skipping separators.
	for {
		m.cursor += delta
		if m.cursor < 0 {
			m.cursor = 0
			break
		}
		if m.cursor >= len(m.items) {
			m.cursor = len(m.items) - 1
			break
		}
		if m.items[m.cursor].kind != addVarKindSeparator {
			break
		}
	}
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	// Scroll down: check if cursor is beyond the visible row budget from offset.
	rows := 0
	lastVisible := m.offset
	for i := m.offset; i < len(m.items); i++ {
		r := 1
		if m.items[i].subLabel != "" {
			r = 2
		}
		if rows+r > m.maxVis {
			break
		}
		rows += r
		lastVisible = i
	}
	if m.cursor > lastVisible {
		for m.cursor > lastVisible && m.offset < m.cursor {
			m.offset++
			rows = 0
			lastVisible = m.offset
			for i := m.offset; i < len(m.items); i++ {
				r := 1
				if m.items[i].subLabel != "" {
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

func (m *addVarDialogModel) HelpText() string {
	if m.focus == addVarFocusList && m.cursor >= 0 && m.cursor < len(m.items) {
		return m.items[m.cursor].help
	}
	return ""
}

func (m *addVarDialogModel) SetFocused(f bool) { m.focused = f }

func (m *addVarDialogModel) SetSize(w, h int) {
	m.width = w
	m.height = h
	m.recalc()
}

func (m *addVarDialogModel) recalc() {
	ctx := tui.GetActiveContext()
	contentW := m.innerWidth()
	headingRaw := FormatMenuHeading(MenuHeadingParams{
		AppName:        m.appName,
		AppDescription: m.appDesc,
	}, contentW)
	headingRenderedH := lipgloss.Height(ctx.Dialog.Padding(1, 2).Width(contentW).Render(console.ToANSI(headingRaw)))
	btnH := tui.ButtonRowHeight(contentW, 0, tui.ButtonSpec{Text: "Create"}, tui.ButtonSpec{Text: "Cancel"})
	// overhead: outer border(2) + rendered heading + "Variable Name" section(3) + "Available Variables" borders(2) + spacer(1) + buttons
	fixed := 2 + headingRenderedH + 3 + 2 + 1 + btnH
	m.maxVis = m.height - fixed
	if m.maxVis < 2 {
		m.maxVis = 2
	}
}

func (m *addVarDialogModel) IsMaximized() bool { return true }

func (m *addVarDialogModel) innerWidth() int {
	w := m.width - 2
	if w < 20 {
		w = 20
	}
	return w
}

func (m *addVarDialogModel) itemIndexAt(screenY int) int {
	ctx := tui.GetActiveContext()
	contentW := m.innerWidth()
	headingRaw := FormatMenuHeading(MenuHeadingParams{AppName: m.appName, AppDescription: m.appDesc}, contentW)
	headingH := lipgloss.Height(ctx.Dialog.Padding(1, 2).Width(contentW).Render(console.ToANSI(headingRaw)))
	// list starts at: outer border(1) + headingH + "Variable Name" section(3) + "Available Variables" title border(1)
	listTop := 1 + headingH + 3 + 1
	rowY := listTop
	rowBudget := m.maxVis
	for i := m.offset; i < len(m.items) && rowBudget > 0; i++ {
		item := m.items[i]
		if item.kind == addVarKindSeparator {
			if rowBudget < 1 {
				break
			}
			rowBudget--
			rowY++
			continue
		}
		h := 1
		if item.subLabel != "" {
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
func (m *addVarDialogModel) GetInputCursor() (relX, relY int, shape tea.CursorShape, ok bool) {
	if m.focus != addVarFocusInput || !m.input.Focused() {
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

func (m *addVarDialogModel) ViewString() string {
	if m.width == 0 {
		return ""
	}
	ctx := tui.GetActiveContext()
	contentW := m.innerWidth()
	sInnerW := contentW - 2 // inner width of each bordered section

	bgStyle := tui.SemanticStyle("{{|Theme_Dialog|}}")
	normalStyle := tui.SemanticStyle("{{|Theme_Item|}}")
	selectedStyle := tui.SemanticStyle("{{|Theme_ItemSelected|}}")
	subLabelStyle := tui.SemanticStyle("{{|Theme_HelpItem|}}")
	sepChar := "─"
	if !ctx.LineCharacters {
		sepChar = "-"
	}

	// Heading
	headingRaw := FormatMenuHeading(MenuHeadingParams{
		AppName:        m.appName,
		AppDescription: m.appDesc,
	}, contentW)
	headingText := strings.TrimRight(
		ctx.Dialog.Padding(1, 2).Width(contentW).Render(console.ToANSI(headingRaw)), "\n")

	// "Variable Name" section — titled bordered box, thick border when focused
	inputFocused := m.focus == addVarFocusInput
	inputContent := strings.TrimRight(ctx.Dialog.Padding(0, 1).Width(sInnerW).Render(m.input.View()), "\n")
	inputTitleTag := "Theme_TitleSubMenu"
	if inputFocused {
		inputTitleTag = "Theme_TitleSubMenuFocused"
	}
	varNameSection := strings.TrimRight(tui.RenderBorderedBoxCtx(
		"Variable Name", inputContent, sInnerW, 0, inputFocused, true, true,
		ctx.SubmenuTitleAlign, inputTitleTag, ctx,
	), "\n")
	// Inject INS/OVR label into the bottom-left of the Variable Name section border.
	modeLabel := "INS"
	if m.input.IsOverwrite() {
		modeLabel = "OVR"
	}
	vnLines := strings.Split(varNameSection, "\n")
	if len(vnLines) > 0 {
		vnLines[len(vnLines)-1] = tui.BuildLabeledBottomBorderCtx(sInnerW+2, modeLabel, inputFocused, ctx)
		varNameSection = strings.Join(vnLines, "\n")
	}

	// "Available Variables" section — titled bordered box, thick border when focused
	listFocused := m.focus == addVarFocusList
	// Always reserve one column for the scrollbar gutter so width is stable.
	maxItemW := sInnerW - 3 - tui.ScrollbarGutterWidth // cursor(1) + space(1) + trailing space(1) + gutter

	// Compute scroll metrics for the scrollbar / scroll indicator.
	totalRows := 0
	offsetRows := 0
	for i, item := range m.items {
		r := 1
		if item.subLabel != "" {
			r = 2
		}
		if i < m.offset {
			offsetRows += r
		}
		totalRows += r
	}

	var listLines []string
	rowBudget := m.maxVis
	for i := m.offset; i < len(m.items) && rowBudget > 0; i++ {
		item := m.items[i]
		if item.kind == addVarKindSeparator {
			if rowBudget < 1 {
				break
			}
			rowBudget--
			sepW := sInnerW - tui.ScrollbarGutterWidth - 2
			if sepW < 0 {
				sepW = 0
			}
			listLines = append(listLines, bgStyle.Render(" "+strings.Repeat(sepChar, sepW)+" "))
			continue
		}
		itemRows := 1
		if item.subLabel != "" {
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
		label := item.label
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
		if item.subLabel != "" {
			sl := item.subLabel
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
	availableSection := strings.TrimRight(tui.RenderBorderedBoxCtx(
		"Available Variables", listContent, sInnerW, 0, listFocused, true, true,
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
		sectionLines := strings.Split(availableSection, "\n")
		if len(sectionLines) > 0 {
			bottomLine := tui.BuildScrollPercentBottomBorder(sInnerW+2, scrollPct, listFocused, ctx)
			sectionLines[len(sectionLines)-1] = bottomLine
			availableSection = strings.Join(sectionLines, "\n")
		}
	}

	// Buttons
	buttonRow := strings.TrimRight(tui.RenderCenteredButtonsCtx(
		contentW, ctx,
		tui.ButtonSpec{Text: "Create", Active: m.focus == addVarFocusInput || m.focus == addVarFocusCreate},
		tui.ButtonSpec{Text: "Cancel", Active: m.focus == addVarFocusCancel},
	), "\n")

	// Dynamic spacer pushes buttons to the bottom.
	headingRenderedH := lipgloss.Height(headingText)
	varNameSectionH := lipgloss.Height(varNameSection)
	availableSectionH := lipgloss.Height(availableSection)
	buttonRowH := lipgloss.Height(buttonRow)
	spacerH := m.height - 2 - headingRenderedH - varNameSectionH - availableSectionH - buttonRowH
	if spacerH < 1 {
		spacerH = 1
	}
	spacer := strings.TrimRight(strings.Repeat(bgStyle.Width(contentW).Render("")+"\n", spacerH), "\n")

	parts := []string{headingText, varNameSection, availableSection, spacer, buttonRow}
	return tui.RenderDialogWithType("Add Variable", lipgloss.JoinVertical(lipgloss.Left, parts...), m.focused, m.height, tui.DialogTypeInfo)
}

func (m *addVarDialogModel) View() tea.View {
	return tea.NewView(m.ViewString())
}

func (m *addVarDialogModel) Layers() []*lipgloss.Layer {
	return []*lipgloss.Layer{
		lipgloss.NewLayer(m.ViewString()).Z(tui.ZScreen).ID("addvar_dialog"),
	}
}

func (m *addVarDialogModel) GetHitRegions(offsetX, offsetY int) []tui.HitRegion {
	ctx := tui.GetActiveContext()
	contentW := m.innerWidth()

	headingRaw := FormatMenuHeading(MenuHeadingParams{AppName: m.appName, AppDescription: m.appDesc}, contentW)
	headingH := lipgloss.Height(ctx.Dialog.Padding(1, 2).Width(contentW).Render(console.ToANSI(headingRaw)))
	// list starts at: outer border(1) + headingH + "Variable Name" section(3) + "Available Variables" title border(1)
	listTop := 1 + headingH + 3 + 1

	listH := 0
	rowBudget := m.maxVis
	for i := m.offset; i < len(m.items) && rowBudget > 0; i++ {
		item := m.items[i]
		h := 1
		if item.subLabel != "" {
			h = 2
		}
		if h > rowBudget {
			break
		}
		rowBudget -= h
		listH += h
	}

	// Input hit region: outer_border(1) + headingH + section_top_border(1)
	inputY := 1 + headingH + 1
	m.inputRelY = inputY
	m.inputScreenX = offsetX + 1 + 1 + 1 + m.input.PromptWidth()
	m.input.SetScreenTextX(m.inputScreenX)

	var regions []tui.HitRegion
	regions = append(regions, tui.HitRegion{
		ID:     "addvar_input",
		X:      offsetX + 1,
		Y:      offsetY + inputY,
		Width:  contentW,
		Height: 1,
		ZOrder: tui.ZDialog + 10,
		Label:  "Variable Name",
		Help: &tui.HelpContext{
			ScreenName: "Add Variable",
			PageTitle:  "Editing",
			PageText:   "Enter a name for the new environment variable.",
			ItemText:   "Type the name and press Enter to create, or Esc to cancel.",
		},
	})
	if listH > 0 {
		regions = append(regions, tui.HitRegion{
			ID:     "addvar_list",
			X:      offsetX + 1,
			Y:      offsetY + listTop,
			Width:  contentW + 2,
			Height: listH,
			ZOrder: tui.ZDialog + 10,
			Label:  "Available Variables",
		})
	}

	// Dialog background
	regions = append(regions, tui.HitRegion{
		ID:     "addvar_dialog",
		X:      offsetX,
		Y:      offsetY,
		Width:  m.width,
		Height: m.height,
		ZOrder: tui.ZDialog,
		Label:  "Add Variable",
		Help: &tui.HelpContext{
			ScreenName: "Add Variable",
			PageTitle:  "Description",
			PageText:   "Enter a name for the new environment variable.",
		},
	})

	btnH := tui.ButtonRowHeight(contentW, 0, tui.ButtonSpec{Text: "Create"}, tui.ButtonSpec{Text: "Cancel"})
	buttonY := m.height - 1 - btnH
	regions = append(regions, tui.HitRegion{
		ID:     "addvar_buttons",
		X:      offsetX + 1,
		Y:      offsetY + buttonY,
		Width:  contentW,
		Height: btnH,
		ZOrder: tui.ZDialog + 5,
		Label:  "Actions",
		Help: &tui.HelpContext{
			ScreenName: "Add Variable",
			PageTitle:  "Description",
			PageText:   "Enter a name for the new environment variable.",
		},
	})
	regions = append(regions, tui.GetButtonHitRegions(
		tui.HelpContext{
			ScreenName: "Add Variable",
			PageTitle:  "Description",
			PageText:   "Enter a name for the new environment variable.",
		},
		"addvar_dialog", offsetX+1, offsetY+buttonY, contentW, tui.ZDialog+20,
		tui.ButtonSpec{Text: "Create", ZoneID: "Create", Help: "Create the new variable with the entered name."},
		tui.ButtonSpec{Text: "Cancel", ZoneID: "Cancel", Help: "Cancel and return to the editor."},
	)...)

	return regions
}
