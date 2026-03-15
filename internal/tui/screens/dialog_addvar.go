package screens

import (
	"strings"

	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/tui"

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

	input  textinput.Model
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
		input:          ti,
		items:          items,
		focus:          addVarFocusInput,
		addAllVars:     addAllVars,
		addAllDefaults: addAllDefaults,
		maxVis:         8,
	}
}

func (m *addVarDialogModel) Init() tea.Cmd {
	return textinput.Blink
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

		case key.Matches(msg, tui.Keys.Tab):
			m.cycleFocus(+1)
			return m, nil

		case key.Matches(msg, tui.Keys.ShiftTab):
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
			} else if m.focus == addVarFocusCancel {
				m.focus = addVarFocusCreate
			}
			return m, nil

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

	case tui.LayerHitMsg:
		if msg.Button == tea.MouseMiddle {
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
	if m.cursor >= m.offset+m.maxVis {
		m.offset = m.cursor - m.maxVis + 1
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
	contentW := m.innerWidth()
	headingRaw := FormatMenuHeading(MenuHeadingParams{
		AppName:        m.appName,
		AppDescription: m.appDesc,
	}, contentW)
	headingH := strings.Count(headingRaw, "\n") + 1
	// overhead: border(2) + heading padding(2) + headingH + spacer(1) + input(3) + spacer(1) + buttons(1)
	fixed := 2 + 2 + headingH + 1 + 3 + 1 + 1
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
	// list starts at: border(1) + headingH + spacer(1) + input(3) + spacer(1)
	listTop := 1 + headingH + 1 + 3 + 1
	rowY := listTop
	for i := m.offset; i < len(m.items) && i < m.offset+m.maxVis; i++ {
		if m.items[i].kind == addVarKindSeparator {
			rowY++
			continue
		}
		h := 1
		if m.items[i].subLabel != "" {
			h = 2
		}
		if screenY >= rowY && screenY < rowY+h {
			return i
		}
		rowY += h
	}
	return -1
}

func (m *addVarDialogModel) ViewString() string {
	if m.width == 0 {
		return ""
	}
	ctx := tui.GetActiveContext()
	contentW := m.innerWidth()

	bgStyle := tui.SemanticStyle("{{|Theme_Dialog|}}")
	normalStyle := tui.SemanticStyle("{{|Theme_Item|}}")
	selectedStyle := tui.SemanticStyle("{{|Theme_ItemSelected|}}")
	subLabelStyle := tui.SemanticStyle("{{|Theme_HelpItem|}}")

	// Heading
	headingRaw := FormatMenuHeading(MenuHeadingParams{
		AppName:        m.appName,
		AppDescription: m.appDesc,
	}, contentW)
	headingText := strings.TrimRight(
		ctx.Dialog.Padding(1, 2).Width(contentW).Render(console.ToANSI(headingRaw)), "\n")

	// Input
	inputFocused := m.focus == addVarFocusInput
	borderedInput := tui.ApplyInnerBorderCtx(ctx.Dialog.Padding(0, 1).Width(contentW), inputFocused, ctx)
	renderedInput := strings.TrimRight(borderedInput.Render("Variable: "+m.input.View()), "\n")

	// List
	sepChar := "─"
	if !ctx.LineCharacters {
		sepChar = "-"
	}
	var listLines []string
	for i := m.offset; i < len(m.items) && i < m.offset+m.maxVis; i++ {
		item := m.items[i]
		if item.kind == addVarKindSeparator {
			listLines = append(listLines, bgStyle.Render(" "+strings.Repeat(sepChar, contentW)+" "))
			continue
		}
		label := item.label
		if lipgloss.Width(label) > contentW {
			label = tui.TruncateRight(label, contentW)
		}
		pad := contentW - lipgloss.Width(label)
		if pad < 0 {
			pad = 0
		}
		line := " " + label + strings.Repeat(" ", pad) + " "
		focused := i == m.cursor && m.focus == addVarFocusList
		if focused {
			listLines = append(listLines, tui.MaintainBackground(selectedStyle.Render(line), selectedStyle))
		} else {
			listLines = append(listLines, tui.MaintainBackground(normalStyle.Background(bgStyle.GetBackground()).Render(line), bgStyle))
		}
		if item.subLabel != "" {
			sl := item.subLabel
			if lipgloss.Width(sl) > contentW {
				sl = tui.TruncateRight(sl, contentW)
			}
			slPad := contentW - lipgloss.Width(sl)
			if slPad < 0 {
				slPad = 0
			}
			slLine := " " + sl + strings.Repeat(" ", slPad) + " "
			if focused {
				listLines = append(listLines, tui.MaintainBackground(selectedStyle.Render(slLine), selectedStyle))
			} else {
				listLines = append(listLines, tui.MaintainBackground(subLabelStyle.Background(bgStyle.GetBackground()).Render(slLine), bgStyle))
			}
		}
	}

	spacer := bgStyle.Width(contentW + 2).Render("")
	buttonRow := strings.TrimRight(tui.RenderCenteredButtonsCtx(
		contentW, ctx,
		tui.ButtonSpec{Text: "Create", Active: m.focus == addVarFocusInput || m.focus == addVarFocusCreate},
		tui.ButtonSpec{Text: "Cancel", Active: m.focus == addVarFocusCancel},
	), "\n")

	parts := []string{headingText, renderedInput}
	if len(listLines) > 0 {
		parts = append(parts, strings.Join(listLines, "\n"))
	}
	parts = append(parts, spacer, buttonRow)

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
	inputH := 3
	listTop := 1 + headingH + 1 + inputH + 1

	listH := 0
	for i := m.offset; i < len(m.items) && i < m.offset+m.maxVis; i++ {
		if m.items[i].kind == addVarKindSeparator || m.items[i].subLabel == "" {
			listH++
		} else {
			listH += 2
		}
	}

	var regions []tui.HitRegion
	if listH > 0 {
		regions = append(regions, tui.HitRegion{
			ID:     "addvar_list",
			X:      offsetX + 1,
			Y:      offsetY + listTop,
			Width:  contentW + 2,
			Height: listH,
			ZOrder: tui.ZDialog + 10,
		})
	}

	buttonY := listTop + listH + 1
	regions = append(regions, tui.GetButtonHitRegions(
		"addvar_dialog", offsetX+1, offsetY+buttonY, contentW, tui.ZDialog+10,
		tui.ButtonSpec{Text: "Create", ZoneID: "Create"},
		tui.ButtonSpec{Text: "Cancel", ZoneID: "Cancel"},
	)...)

	return regions
}
