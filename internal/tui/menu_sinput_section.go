package tui

import (
	"DockSTARTer2/internal/tui/components/sinput"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
)

// NewSinputSection creates a MenuModel content section that renders a sinput
// (text input field) inside a titled bordered box, matching the style used by
// the set-value dialog's "Current Value" section.
// The returned *sinput.Model pointer is kept in sync by the section's interceptor;
// read inp.Value() to get the current text.
func NewSinputSection(id, title, initialValue string) (*MenuModel, *sinput.Model) {
	ti := textinput.New()
	ti.SetValue(initialValue)
	ti.CursorEnd()
	ti.CharLimit = 128
	ti.Focus()

	styles := GetStyles()
	bg := styles.Dialog.GetBackground()
	tiStyles := textinput.DefaultStyles(true)
	tiStyles.Focused.Prompt = styles.ItemNormal.Background(bg)
	tiStyles.Focused.Text = styles.ItemNormal.Background(bg)
	tiStyles.Blurred.Prompt = styles.ItemNormal.Background(bg)
	tiStyles.Blurred.Text = styles.ItemNormal.Background(bg)
	tiStyles.Cursor.Color = TextCursorColor()
	ti.SetStyles(tiStyles)

	inp := sinput.New(ti)
	inpPtr := &inp

	m := NewMenuModel(id, title, "", nil)
	m.SetSubMenuMode(true)
	m.SetVariableHeight(false)
	m.SetIsDialog(false)
	m.SetButtons([]ButtonDef{})
	m.SetMaximized(true)
	m.SetShowLockGutter(false)
	m.SetNoLeftMargin(true)

	m.contentRenderer = func(contentWidth int) string {
		return styles.Dialog.
			Width(contentWidth).
			Padding(0, 1).
			Render((*inpPtr).View())
	}

	// Register a hit region covering the input text line so click-to-position and
	// drag-to-select work. Y=1 is the content row inside the section's top border.
	// Also sets screenTextX each frame so sinput can translate absolute X → char offset.
	m.extraHitRegions = func(offsetX, offsetY, baseZ int) []HitRegion {
		layout := GetLayout()
		// section left border (1) + padding (1) = text starts at col 2 within section
		textX := offsetX + layout.SingleBorder() + 1 + (*inpPtr).PromptWidth()
		(*inpPtr).SetScreenTextX(textX)
		return []HitRegion{{
			ID:     id + ".sinput",
			X:      offsetX + layout.SingleBorder(),
			Y:      offsetY + layout.SingleBorder(), // content row inside top border
			Width:  m.width - layout.BorderWidth(),
			Height: 1,
			ZOrder: baseZ + 15,
			Label:  title,
		}}
	}

	m.SetUpdateInterceptor(func(msg tea.Msg, menu *MenuModel) (tea.Cmd, bool) {
		switch msg := msg.(type) {
		case tea.KeyPressMsg:
			if key.Matches(msg, Keys.CycleTab) || key.Matches(msg, Keys.CycleShiftTab) {
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
	prev := m.interceptor
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
