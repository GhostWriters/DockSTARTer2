package classic

import (
	"DockSTARTer2/internal/tui/components/sinput"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	semstyle "github.com/GhostWriters/semstyle/lg"
)

// SetInputPrompt draws prompt before an input section's text (see
// NewSinputSection) in the Prompt style.
func (m *MenuModel) SetInputPrompt(prompt string) {
	m.inputPrompt = prompt
	m.InvalidateCache()
}

// SetInputControls draws controls at the right of an input section's row
// (see NewSinputSection), narrowing the input to fit, with hit region IDs
// from InputControlID.
func (m *MenuModel) SetInputControls(controls func() []TitleControl) {
	m.inputControls = controls
	m.InvalidateCache()
}

// InputControlID returns the hit region ID of input control i.
func (m *MenuModel) InputControlID(i int) string { return m.id + ".inputctl" + strconv.Itoa(i) }

// inputControlPieces returns the input controls as drawn, and their total
// width with a space between each and before the first.
func (m *MenuModel) inputControlPieces() ([]TitleControl, []string, int) {
	if m.inputControls == nil {
		return nil, nil, 0
	}
	controls := m.inputControls()
	pieces := titleControlPiecesFor(controls, GetActiveContext(), false)
	width := 0
	for _, p := range pieces {
		width += 1 + WidthWithoutZones(p)
	}
	return controls, pieces, width
}

// NewSinputSection creates a MenuModel content section that renders a sinput
// (text input field) inside a titled bordered box, matching the style used by
// the set-value dialog's "Current Value" section.
// The returned *sinput.Model pointer is kept in sync by the section's interceptor;
// read inp.Value() to get the current text.
func NewSinputSection(id, title, initialValue string) (*MenuModel, *sinput.Model) {
	return newSinputSectionWithEcho(id, title, initialValue, textinput.EchoNormal)
}

// NewPasswordSinputSection is like NewSinputSection but masks input with '*',
// matching the password mode the prompt dialog's sensitive inputs use.
func NewPasswordSinputSection(id, title, initialValue string) (*MenuModel, *sinput.Model) {
	return newSinputSectionWithEcho(id, title, initialValue, textinput.EchoPassword)
}

func newSinputSectionWithEcho(id, title, initialValue string, echoMode textinput.EchoMode) (*MenuModel, *sinput.Model) {
	ti := textinput.New()
	if echoMode == textinput.EchoPassword {
		ti.EchoMode = textinput.EchoPassword
		ti.EchoCharacter = '*'
	}
	ti.SetValue(initialValue)
	ti.CursorEnd()
	ti.CharLimit = 128
	ti.Focus()

	ti.SetStyles(sinputStyles(false))

	inp := sinput.New(ti)
	inpPtr := &inp

	m := NewMenuModel(id, title, "", nil)
	m.textInput = true
	m.SetSubMenuMode(true)
	m.SetVariableHeight(false)
	m.SetIsDialog(false)
	m.SetButtons([]ButtonDef{})
	m.SetMaximized(true)
	m.SetShowLockGutter(false)
	m.SetNoLeftMargin(true)

	m.ContentRenderer = func(contentWidth int) string {
		// Resolved on each render, so the input follows the active theme and
		// the section's disabled state.
		(*inpPtr).SetStyles(sinputStyles(m.disabled))
		field := inputFieldStyle(m.disabled)
		if m.inputPrompt != "" {
			(*inpPtr).Prompt = RenderThemeText("{{|Prompt|}}"+m.inputPrompt+"{{[-]}}", field)
		}
		dialog := GetStyles().Dialog
		// The cell under the terminal cursor has no style of its own.
		view := MaintainBackground((*inpPtr).View(), field)
		_, pieces, controlsWidth := m.inputControlPieces()
		inputWidth := max(contentWidth-2, 1)
		if len(pieces) == 0 {
			return dialog.Width(contentWidth).Padding(0, 1).Render(field.Width(inputWidth).Render(view))
		}
		// The input scrolls within what the controls leave; +1 for the cursor.
		inputWidth = max(inputWidth-controlsWidth, 1)
		(*inpPtr).SetWidth(max(inputWidth-(*inpPtr).PromptWidth()-1, 1))
		space := dialog.Render(" ")
		return dialog.Width(contentWidth).Padding(0, 1).Render(
			field.Width(inputWidth).MaxWidth(inputWidth).Render(view) + space + strings.Join(pieces, space))
	}

	// Register a hit region covering the input text line so click-to-position and
	// drag-to-select work. Y=1 is the content row inside the section's top border.
	// Also sets screenTextX each frame so sinput can translate absolute X → char offset.
	m.ExtraHitRegions = func(offsetX, offsetY, baseZ int) []HitRegion {
		layout := GetLayout()
		// section left border (1) + padding (1) = text starts at col 2 within section
		textX := offsetX + layout.SingleBorder() + 1 + (*inpPtr).PromptWidth()
		(*inpPtr).SetScreenTextX(textX)
		controls, pieces, controlsWidth := m.inputControlPieces()
		regions := []HitRegion{{
			ID:     id + ".sinput",
			X:      offsetX + layout.SingleBorder(),
			Y:      offsetY + layout.SingleBorder(), // content row inside top border
			Width:  m.width - layout.BorderWidth() - controlsWidth,
			Height: 1,
			ZOrder: baseZ + 15,
			Label:  title,
		}}
		// The controls end one column (the padding) before the right border.
		x := offsetX + m.width - layout.SingleBorder() - 1 - controlsWidth
		for i, p := range pieces {
			x++
			w := WidthWithoutZones(p)
			regions = append(regions, HitRegion{ID: m.InputControlID(i), X: x, Y: offsetY + layout.SingleBorder(),
				Width: w, Height: 1, ZOrder: baseZ + 20, Label: controls[i].Help})
			x += w
		}
		return regions
	}

	m.SetUpdateInterceptor(func(msg tea.Msg, menu *MenuModel) (tea.Cmd, bool) {
		switch msg := msg.(type) {
		case tea.KeyPressMsg:
			if (key.Matches(msg, Keys.CycleTab) || key.Matches(msg, Keys.CycleShiftTab)) && !IsTypedText(msg) || key.Matches(msg, Keys.Enter) {
				return nil, false
			}
			newInp, cmd := (*inpPtr).Update(msg)
			*inpPtr = newInp
			menu.InvalidateCache()
			return cmd, true
		case sinput.PasteMsg, sinput.CutMsg, sinput.SelectAllMsg:
			newInp, cmd := (*inpPtr).Update(msg)
			*inpPtr = newInp
			menu.InvalidateCache()
			return cmd, true
		case LayerHitMsg:
			switch msg.Button {
			case tea.MouseLeft:
				(*inpPtr).HandleClick(msg.X)
				menu.InvalidateCache()
			case tea.MouseRight:
				return ShowInputContextMenuWithTitle(*inpPtr, title, msg.X, msg.Y, 9999, 9999), true
			}
			return nil, true
		case tea.MouseClickMsg:
			if msg.Button == tea.MouseLeft {
				(*inpPtr).HandleClick(msg.X)
				menu.InvalidateCache()
			}
			return nil, true
		case tea.MouseReleaseMsg:
			newInp, cmd := (*inpPtr).Update(msg)
			*inpPtr = newInp
			menu.InvalidateCache()
			return cmd, true
		case tea.MouseMotionMsg:
			if (*inpPtr).IsSelecting() {
				newInp, cmd := (*inpPtr).Update(msg)
				*inpPtr = newInp
				menu.InvalidateCache()
				return cmd, true
			}
		}
		return nil, false
	})

	return m, inpPtr
}

// NewNumberSinputSection is like NewSinputSection but restricts input to digits only.
// Up/Down arrow support (increment/decrement) can be added later.
func NewNumberSinputSection(id, title, initialValue string) (*MenuModel, *sinput.Model) {
	m, inp := NewSinputSection(id, title, initialValue)
	prev := m.Interceptor
	m.SetUpdateInterceptor(func(msg tea.Msg, menu *MenuModel) (tea.Cmd, bool) {
		if kp, ok := msg.(tea.KeyPressMsg); ok {
			if kp.Text != "" {
				for _, r := range kp.Text {
					if r < '0' || r > '9' {
						return nil, true // swallow non-digit printable input
					}
				}
			}
		}
		return prev(msg, menu)
	})
	return m, inp
}

// SinputSectionInit returns the Init cmd for a sinput section (blink cursor).
func SinputSectionInit() tea.Cmd {
	return sinput.Blink
}

// sinputStyles returns an input section's text styles for the active
// theme (see inputFieldStyle).
func sinputStyles(disabled bool) textinput.Styles {
	field := inputFieldStyle(disabled)
	ts := textinput.DefaultStyles(true)
	ts.Focused.Prompt, ts.Focused.Text = field, field
	ts.Blurred.Prompt, ts.Blurred.Text = field, field
	ts.Cursor.Color = TextCursorColor()
	return ts
}

// inputFieldStyle returns an input's field style for the active theme:
// InputField, filled in from the dialog style, or where the theme leaves it
// unset Item's text on the dialog background; its disabled form (see
// ResolveDisabledStyle) when disabled.
func inputFieldStyle(disabled bool) lipgloss.Style {
	dialog := GetStyles().Dialog
	if semstyle.GetRawTagCode("InputField") == "" {
		item := GetStyles().ItemNormal
		if disabled {
			item, _ = ResolveDisabledStyle("Item")
		}
		return item.Background(dialog.GetBackground())
	}
	if !disabled {
		return styleWithFallback("InputField", dialog)
	}
	field, _ := ResolveDisabledStyle("InputField")
	if _, noBG := field.GetBackground().(lipgloss.NoColor); noBG {
		field = field.Background(dialog.GetBackground())
	}
	if _, noFG := field.GetForeground().(lipgloss.NoColor); noFG {
		field = field.Foreground(dialog.GetForeground())
	}
	return field
}
