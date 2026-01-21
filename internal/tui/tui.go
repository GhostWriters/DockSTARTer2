package tui

import (
	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/logger"
	"DockSTARTer2/internal/paths"
	"DockSTARTer2/internal/theme"
	"DockSTARTer2/internal/update"
	"DockSTARTer2/internal/version"
	"context"
	"fmt"
	"os"
	"strings"
	"time"
	"unicode"

	"codeberg.org/tslocum/cview"
	"github.com/gdamore/tcell/v3"
	"github.com/spf13/pflag"
)

// MenuItem defines an item in the menu
type MenuItem struct {
	Tag      string
	Desc     string
	Help     string
	Shortcut rune
	Action   func()
}

var (
	app                 *cview.Application
	currentConfig       config.GUIConfig
	helpline            *cview.TextView
	rootGrid            *cview.Grid
	panels              *cview.Panels
	menuSelectedIndices = make(map[string]int)
	rightView           *cview.TextView
	appUpdateAvailable  bool
	tmplUpdateAvailable bool
)

// Helper to set title with auto-translation
func setTitle(p *cview.Box, title string) {
	p.SetTitle(console.Translate(title))
}

// Helper to set text with auto-translation
func setText(p *cview.TextView, text string) {
	p.SetText(console.Translate(text))
}

// WrapInDialogFrame wraps any primitive into the standard centered dialog layout with shadow and buttons
func WrapInDialogFrame(title string, text string, content cview.Primitive, contentWidth, contentHeight int, backAction func(), buttonsFlex *cview.Flex) cview.Primitive {
	dialogBgColor := theme.Current.DialogBG

	// Helper to disable focus on non-interactive elements
	disableFocus := func(p *cview.Box) {
		p.SetMouseCapture(func(action cview.MouseAction, event *tcell.EventMouse) (cview.MouseAction, *tcell.EventMouse) {
			return 0, nil
		})
	}

	// Helper to create colored spacers
	makeSpacer := func() *cview.Box {
		sp := cview.NewBox()
		sp.SetBackgroundColor(dialogBgColor)
		disableFocus(sp)
		return sp
	}

	// 1. Body Text
	textView := cview.NewTextView()
	setText(textView, text)
	textView.SetBackgroundColor(dialogBgColor)
	textView.SetTextColor(theme.Current.DialogFG)
	disableFocus(textView.Box)

	// Layout Structure
	makePaddedFlex := func(p cview.Primitive, expand bool) *cview.Flex {
		f := cview.NewFlex()
		f.SetBackgroundColor(dialogBgColor)
		f.AddItem(makeSpacer(), 1, 0, false)
		f.AddItem(p, 0, 1, expand)
		f.AddItem(makeSpacer(), 1, 0, false)
		return f
	}

	innerFlex := cview.NewFlex()
	innerFlex.SetDirection(cview.FlexRow)
	innerFlex.SetBackgroundColor(dialogBgColor)
	innerFlex.SetBorder(false)
	innerFlex.SetTitle("")

	// Spacers for ThreeDBox border rows
	innerFlex.AddItem(makeSpacer(), 1, 0, false) // Row 0 (Border)
	innerFlex.AddItem(makePaddedFlex(textView, false), 1, 0, false)
	innerFlex.AddItem(makePaddedFlex(content, true), 0, 1, true)
	innerFlex.AddItem(makeSpacer(), 1, 0, false) // Row for Separator Line
	innerFlex.AddItem(buttonsFlex, 1, 0, false)
	innerFlex.AddItem(makeSpacer(), 1, 0, false) // Row for Bottom Border

	// Wrap in ThreeDBox
	titleStyle := tcell.StyleDefault.Foreground(theme.Current.TitleFG).Background(theme.Current.TitleBG)
	if strings.Contains(console.Translate("[_ThemeTitle_]"), ":u") {
		titleStyle = titleStyle.Underline(true)
	}

	threeDBox := &ThreeDBox{
		Flex:        innerFlex,
		Border2FG:   theme.Current.Border2FG,
		DialogBG:    dialogBgColor,
		DrawBorders: currentConfig.Borders,
		SeparatorDy: 3,
		TitlePlain:  fmt.Sprintf(" %s ", title),
		TitleStyle:  titleStyle,
	}

	// Dialog Dimensions
	dialogWidth := contentWidth + 4
	dialogHeight := contentHeight + 7

	// Centering Grid
	grid := cview.NewGrid()
	grid.SetColumns(0, 2, dialogWidth-2, 2, 0)
	grid.SetRows(0, 1, dialogHeight-1, 1, 0)

	// Full background
	bg := cview.NewBox()
	bg.SetBackgroundColor(theme.Current.ScreenBG)
	disableFocus(bg)
	grid.AddItem(bg, 0, 0, 5, 5, 0, 0, false)

	if currentConfig.Shadow {
		shadow := cview.NewBox()
		shadow.SetBackgroundColor(theme.Current.ShadowColor)
		disableFocus(shadow)
		grid.AddItem(shadow, 2, 2, 2, 2, 0, 0, false)
	}

	solidBg := cview.NewBox()
	solidBg.SetBackgroundColor(dialogBgColor)
	disableFocus(solidBg)
	grid.AddItem(solidBg, 1, 1, 2, 2, 0, 0, false)

	grid.AddItem(threeDBox, 1, 1, 2, 2, 0, 0, true)

	return grid
}

// NewMenuDialog creates a centered dialog with a menu list
func NewMenuDialog(title string, text string, items []MenuItem, backAction func()) (cview.Primitive, *cview.List) {
	// Calculate Dimensions
	maxTagLen := 0
	maxDescLen := 0
	for _, item := range items {
		if len(item.Tag) > maxTagLen {
			maxTagLen = len(item.Tag)
		}
		if len(item.Desc) > maxDescLen {
			maxDescLen = len(item.Desc)
		}
	}

	colPadding := 2
	contentWidth := maxTagLen + colPadding + maxDescLen + 4 // +4 for internal list padding/border
	if len(text) > contentWidth {
		contentWidth = len(text)
	}
	titleWidth := len(title) + 4
	if titleWidth > contentWidth {
		contentWidth = titleWidth
	}

	buttonsWidth := 24
	if backAction != nil {
		buttonsWidth += 10
	}
	if buttonsWidth > contentWidth {
		contentWidth = buttonsWidth
	}

	dialogBgColor := theme.Current.DialogBG

	// 1. Buttons
	btnSelect := cview.NewButton("<Select>")
	btnSelect.SetBackgroundColor(theme.Current.ButtonInactiveBG)
	btnSelect.SetLabelColor(theme.Current.ButtonInactiveFG)
	btnSelect.SetBackgroundColorFocused(theme.Current.ButtonActiveBG)
	btnSelect.SetLabelColorFocused(theme.Current.ButtonActiveFG)

	btnExit := cview.NewButton("<Exit>")
	btnExit.SetLabelColor(theme.Current.ButtonInactiveFG)
	btnExit.SetBackgroundColor(theme.Current.ButtonInactiveBG)
	btnExit.SetBackgroundColorFocused(theme.Current.ButtonActiveBG)
	btnExit.SetLabelColorFocused(theme.Current.ButtonActiveFG)

	var btnBack *cview.Button
	if backAction != nil {
		btnBack = cview.NewButton("<Back>")
		btnBack.SetLabelColor(theme.Current.ButtonInactiveFG)
		btnBack.SetBackgroundColor(theme.Current.ButtonInactiveBG)
		btnBack.SetBackgroundColorFocused(theme.Current.ButtonActiveBG)
		btnBack.SetLabelColorFocused(theme.Current.ButtonActiveFG)
		btnBack.SetSelectedFunc(func() {
			app.QueueUpdateDraw(backAction)
		})
	}

	// 2. Menu List
	baseList := cview.NewList()
	baseList.ShowSecondaryText(false)
	baseList.SetHighlightFullLine(true)
	baseList.SetBackgroundColor(dialogBgColor)
	baseList.SetMainTextColor(theme.Current.DialogFG)
	baseList.SetSelectedBackgroundColor(theme.Current.ItemSelectedBG)
	baseList.SetSelectedTextColor(theme.Current.ItemSelectedFG)
	baseList.SetBorder(true)
	baseList.SetBorderColor(theme.Current.BorderFG)
	baseList.SetPadding(0, 0, 1, 1)

	list := &ThreeDList{
		List:      baseList,
		Border2FG: theme.Current.Border2FG,
		DialogBG:  dialogBgColor,
	}

	refreshHelpline := func(index int) {
		if helpline != nil && index >= 0 && index < len(items) {
			helpline.SetText(" " + console.Translate(items[index].Help) + " ")
			app.Draw()
		}
	}

	for i, item := range items {
		firstLetter := string([]rune(item.Tag)[0])
		rest := string([]rune(item.Tag)[1:])

		// Use explicit theme colors to avoid Translation bugs in List rendering
		tagKeyColor := theme.GetColorStr(theme.Current.TagKeyFG)
		tagColor := theme.GetColorStr(theme.Current.TagFG)

		tagName := fmt.Sprintf("[%s]%s[%s]%s", tagKeyColor, firstLetter, tagColor, rest)
		padding := strings.Repeat(" ", maxTagLen-len(item.Tag)+colPadding)
		mainText := fmt.Sprintf("%s%s%s", tagName, padding, console.Translate(item.Desc))

		listItem := cview.NewListItem(mainText)
		action := item.Action
		idx := i
		listItem.SetSelectedFunc(func() {
			list.SetCurrentItem(idx)
			app.SetFocus(list)
			refreshHelpline(idx)
			if action != nil {
				app.QueueUpdateDraw(action)
			}
		})
		list.AddItem(listItem)
	}

	// Navigation & Shortcuts
	list.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyTab, tcell.KeyRight:
			app.SetFocus(btnSelect)
			return nil
		case tcell.KeyBacktab, tcell.KeyLeft:
			app.SetFocus(btnExit)
			return nil
		case tcell.KeyEsc:
			if backAction != nil {
				app.QueueUpdateDraw(backAction)
			} else {
				app.Stop()
			}
			return nil
		}
		if event.Key() == tcell.KeyRune {
			s := event.Str()
			if len(s) > 0 {
				r := []rune(s)[0]
				for i, item := range items {
					if unicode.ToLower(item.Shortcut) == unicode.ToLower(r) {
						app.SetFocus(list)
						list.SetCurrentItem(i)
						if item.Action != nil {
							app.QueueUpdateDraw(item.Action)
						}
						return nil
					}
				}
			}
		}
		return event
	})

	list.SetChangedFunc(func(index int, item *cview.ListItem) {
		refreshHelpline(index)
		menuSelectedIndices[title] = index
	})

	if idx, ok := menuSelectedIndices[title]; ok && idx >= 0 && idx < len(items) {
		list.SetCurrentItem(idx)
	}

	// Initial helpline
	currIdx := list.GetCurrentItemIndex()
	refreshHelpline(currIdx)

	btnSelect.SetSelectedFunc(func() {
		idx := list.GetCurrentItemIndex()
		if idx >= 0 && idx < len(items) {
			app.SetFocus(list)
			if items[idx].Action != nil {
				app.QueueUpdateDraw(items[idx].Action)
			}
		}
	})

	btnExit.SetSelectedFunc(func() {
		app.Stop()
	})

	// Button Focus Navigation
	btnSelect.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyTab, tcell.KeyRight:
			if btnBack != nil {
				app.SetFocus(btnBack)
			} else {
				app.SetFocus(btnExit)
			}
			return nil
		case tcell.KeyBacktab, tcell.KeyUp, tcell.KeyLeft:
			app.SetFocus(list)
			return nil
		case tcell.KeyEsc:
			if backAction != nil {
				app.QueueUpdateDraw(backAction)
			} else {
				app.Stop()
			}
			return nil
		}
		return event
	})

	if btnBack != nil {
		btnBack.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
			switch event.Key() {
			case tcell.KeyTab, tcell.KeyRight:
				app.SetFocus(btnExit)
				return nil
			case tcell.KeyBacktab, tcell.KeyLeft:
				app.SetFocus(btnSelect)
				return nil
			case tcell.KeyUp:
				app.SetFocus(list)
				return nil
			case tcell.KeyEsc:
				app.QueueUpdateDraw(backAction)
				return nil
			}
			return event
		})
	}

	btnExit.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyTab, tcell.KeyRight:
			app.SetFocus(list)
			return nil
		case tcell.KeyBacktab, tcell.KeyLeft:
			if btnBack != nil {
				app.SetFocus(btnBack)
			} else {
				app.SetFocus(btnSelect)
			}
			return nil
		case tcell.KeyUp:
			app.SetFocus(list)
			return nil
		case tcell.KeyEsc:
			if backAction != nil {
				app.QueueUpdateDraw(backAction)
			} else {
				app.Stop()
			}
			return nil
		}
		return event
	})

	// Buttons Layout
	buttonsFlex := cview.NewFlex()
	buttonsFlex.SetBackgroundColor(dialogBgColor)

	sp1 := cview.NewBox()
	sp1.SetBackgroundColor(dialogBgColor)
	buttonsFlex.AddItem(sp1, 0, 1, false) // Left spacer

	buttonsFlex.AddItem(btnSelect, 10, 0, false) // <Select>
	if btnBack != nil {
		spGap1 := cview.NewBox()
		spGap1.SetBackgroundColor(dialogBgColor)
		buttonsFlex.AddItem(spGap1, 2, 0, false)  // Gap
		buttonsFlex.AddItem(btnBack, 8, 0, false) // <Back>
	}

	spGap2 := cview.NewBox()
	spGap2.SetBackgroundColor(dialogBgColor)
	buttonsFlex.AddItem(spGap2, 2, 0, false) // Gap

	buttonsFlex.AddItem(btnExit, 8, 0, false) // <Exit>

	sp2 := cview.NewBox()
	sp2.SetBackgroundColor(dialogBgColor)
	buttonsFlex.AddItem(sp2, 0, 1, false) // Right spacer

	return WrapInDialogFrame(title, text, list, contentWidth, len(items), backAction, buttonsFlex), list.List
}

// Helper to draw the 3D border effect
func draw3DBorder(screen tcell.Screen, x, y, width, height int, fg, bg tcell.Color) {
	style := tcell.StyleDefault.Foreground(fg).Background(bg)
	vert := cview.Borders.Vertical
	horiz := cview.Borders.Horizontal
	bottomRight := cview.Borders.BottomRight

	for dy := 1; dy < height-1; dy++ {
		screen.SetContent(x+width-1, y+dy, vert, nil, style)
	}
	for dx := 1; dx < width-1; dx++ {
		screen.SetContent(x+dx, y+height-1, horiz, nil, style)
	}
	screen.SetContent(x+width-1, y+height-1, bottomRight, nil, style)
}

// ThreeDList wraps cview.List to provide 3D borders
type ThreeDList struct {
	*cview.List
	Border2FG tcell.Color
	DialogBG  tcell.Color
}

func (d *ThreeDList) Draw(screen tcell.Screen) {
	d.List.Draw(screen)
	if d.GetBorder() {
		x, y, width, height := d.GetRect()
		style := tcell.StyleDefault.Foreground(theme.Current.BorderFG).Background(d.DialogBG)
		horiz := cview.Borders.Horizontal
		vert := cview.Borders.Vertical
		topLeft := cview.Borders.TopLeft
		topRight := cview.Borders.TopRight
		bottomLeft := cview.Borders.BottomLeft

		// Draw Top and Left (replace cview's potentially double-line defaults)
		for dx := 1; dx < width-1; dx++ {
			screen.SetContent(x+dx, y, horiz, nil, style)
		}
		for dy := 1; dy < height-1; dy++ {
			screen.SetContent(x, y+dy, vert, nil, style)
		}
		screen.SetContent(x, y, topLeft, nil, style)
		screen.SetContent(x+width-1, y, topRight, nil, style)
		screen.SetContent(x, y+height-1, bottomLeft, nil, style)

		// Draw 3D Right and Bottom (Shadow Colors)
		draw3DBorder(screen, x, y, width, height, d.Border2FG, d.DialogBG)
	}
}

// ThreeDBox is a wrapper around cview.Flex to implement custom 3D borders and separator
type ThreeDBox struct {
	*cview.Flex
	Border2FG   tcell.Color
	DialogBG    tcell.Color
	DrawBorders bool
	SeparatorDy int
	TitlePlain  string
	TitleStyle  tcell.Style
}

func (d *ThreeDBox) Draw(screen tcell.Screen) {
	d.Flex.Draw(screen)
	if !d.DrawBorders {
		return
	}
	x, y, width, height := d.GetRect()

	style := tcell.StyleDefault.Foreground(theme.Current.BorderFG).Background(d.DialogBG)
	horiz := cview.Borders.Horizontal
	vert := cview.Borders.Vertical
	topLeft := cview.Borders.TopLeft
	topRight := cview.Borders.TopRight
	bottomLeft := cview.Borders.BottomLeft

	// Draw Top and Left (Standard Colors)
	for dx := 1; dx < width-1; dx++ {
		screen.SetContent(x+dx, y, horiz, nil, style)
	}
	for dy := 1; dy < height-1; dy++ {
		screen.SetContent(x, y+dy, vert, nil, style)
	}
	screen.SetContent(x, y, topLeft, nil, style)
	screen.SetContent(x+width-1, y, topRight, nil, style)
	screen.SetContent(x, y+height-1, bottomLeft, nil, style)

	// Draw 3D Right and Bottom (Shadow Colors)
	draw3DBorder(screen, x, y, width, height, d.Border2FG, d.DialogBG)

	// Title
	if d.TitlePlain != "" {
		titleLen := len(d.TitlePlain)
		titleX := x + (width-titleLen)/2
		for i, r := range d.TitlePlain {
			screen.SetContent(titleX+i, y, r, nil, d.TitleStyle)
		}
	}

	// Separator
	if d.SeparatorDy > 0 {
		sepY := y + height - d.SeparatorDy
		if sepY > y && sepY < y+height-1 {
			sepStyle := tcell.StyleDefault.Foreground(theme.Current.BorderFG).Background(d.DialogBG)
			leftT := cview.Borders.LeftT
			rightT := cview.Borders.RightT

			for dx := 1; dx < width-1; dx++ {
				screen.SetContent(x+dx, sepY, horiz, nil, sepStyle)
			}
			screen.SetContent(x, sepY, leftT, nil, sepStyle)
			styleShadow := tcell.StyleDefault.Foreground(d.Border2FG).Background(d.DialogBG)
			screen.SetContent(x+width-1, sepY, rightT, nil, styleShadow)
		}
	}
}

// Start starts the TUI application.
func Start(ctx context.Context) error {
	logger.Info(ctx, "TUI Starting...")
	currentConfig = config.LoadGUIConfig()
	func() {
		if r := recover(); r != nil {
			if app != nil {
				app.Stop()
			}
			logger.Error(ctx, "TUI Panic: %v", r)
		}
	}()

	_ = theme.Load(currentConfig.Theme)

	screen, err := tcell.NewScreen()
	if err != nil {
		return fmt.Errorf("failed to create screen: %w", err)
	}
	if err := screen.Init(); err != nil {
		return fmt.Errorf("failed to initialize screen: %w", err)
	}
	defer screen.Fini()

	app = cview.NewApplication()
	app.SetScreen(screen)
	app.EnableMouse(true)

	// Background update check
	// Background update check
	go func() {
		// Initial check
		appUpdate, tmplUpdate := update.GetUpdateStatus(ctx)
		appUpdateAvailable = appUpdate
		tmplUpdateAvailable = tmplUpdate
		app.QueueUpdateDraw(func() {
			refreshHeader()
		})

		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				appUpdate, tmplUpdate := update.GetUpdateStatus(ctx)
				if appUpdate != appUpdateAvailable || tmplUpdate != tmplUpdateAvailable {
					appUpdateAvailable = appUpdate
					tmplUpdateAvailable = tmplUpdate
					app.QueueUpdateDraw(func() {
						refreshHeader()
					})
				}
			}
		}
	}()

	headerBG := theme.Current.ScreenBG
	headerFG := tcell.ColorBlack

	if currentConfig.LineCharacters {
		cview.Borders.Horizontal = '─'
		cview.Borders.Vertical = '│'
		cview.Borders.TopLeft = '┌'
		cview.Borders.TopRight = '┐'
		cview.Borders.BottomLeft = '└'
		cview.Borders.BottomRight = '┘'
		cview.Borders.LeftT = '├'
		cview.Borders.RightT = '┤'
	} else {
		cview.Borders.Horizontal = '-'
		cview.Borders.Vertical = '|'
		cview.Borders.TopLeft = '+'
		cview.Borders.TopRight = '+'
		cview.Borders.BottomLeft = '+'
		cview.Borders.BottomRight = '+'
		cview.Borders.LeftT = '+'
		cview.Borders.RightT = '+'
	}

	leftView := cview.NewTextView()
	leftView.SetDynamicColors(true)
	leftView.SetBackgroundColor(headerBG)
	leftView.SetTextColor(headerFG)
	leftView.SetTextAlign(cview.AlignLeft)

	centerView := cview.NewTextView()
	centerView.SetDynamicColors(true)
	centerView.SetBackgroundColor(headerBG)
	centerView.SetTextColor(headerFG)
	centerView.SetTextAlign(cview.AlignCenter)

	hostname, _ := os.Hostname()
	leftText := "[_ThemeHostname_]" + hostname + " [-]"
	var flags []string
	if v, _ := pflag.CommandLine.GetBool("verbose"); v {
		flags = append(flags, "VERBOSE")
	}
	if d, _ := pflag.CommandLine.GetBool("debug"); d {
		flags = append(flags, "DEBUG")
	}
	if f, _ := pflag.CommandLine.GetBool("force"); f {
		flags = append(flags, "FORCE")
	}
	if y, _ := pflag.CommandLine.GetBool("yes"); y {
		flags = append(flags, "YES")
	}
	if len(flags) > 0 {
		leftText += " [_ThemeApplicationFlagsBrackets_]|[-]"
		for i, f := range flags {
			if i > 0 {
				leftText += "[_ThemeApplicationFlagsSpace_]|[-]"
			}
			leftText += "[_ThemeApplicationFlags_]" + f + "[-]"
		}
		leftText += "[_ThemeApplicationFlagsBrackets_]|[-]"
	}
	leftView.SetText(console.Translate(leftText))

	centerView.SetText(console.Translate("[_ThemeApplicationName_]" + version.ApplicationName + "[-]"))

	rightView = cview.NewTextView()
	rightView.SetDynamicColors(true)
	rightView.SetBackgroundColor(headerBG)
	rightView.SetTextColor(headerFG)
	rightView.SetTextAlign(cview.AlignRight)
	refreshHeader()

	headerFlex := cview.NewFlex()
	headerFlex.SetBackgroundColor(headerBG)

	hPadL := cview.NewBox()
	hPadL.SetBackgroundColor(headerBG)
	headerFlex.AddItem(hPadL, 1, 0, false)

	headerFlex.AddItem(leftView, 0, 1, false)
	headerFlex.AddItem(centerView, 0, 1, false)
	headerFlex.AddItem(rightView, 0, 1, false)

	hPadR := cview.NewBox()
	hPadR.SetBackgroundColor(headerBG)
	headerFlex.AddItem(hPadR, 1, 0, false)

	headerSep := cview.NewTextView()
	headerSep.SetBackgroundColor(headerBG)
	headerSep.SetTextColor(headerFG)
	sepChar := "─"
	if !currentConfig.LineCharacters {
		sepChar = "-"
	}
	headerSep.SetDrawFunc(func(screen tcell.Screen, x, y, width, height int) (int, int, int, int) {
		line := strings.Repeat(sepChar, width-2)
		cview.Print(screen, []byte(" "+line+" "), x, y, width, cview.AlignCenter, headerFG)
		return x, y, width, height
	})

	helpline = cview.NewTextView()
	helpline.SetBackgroundColor(headerBG)
	helpline.SetTextColor(headerFG)
	helpline.SetTextAlign(cview.AlignCenter)
	helpline.SetDynamicColors(true)

	panels = cview.NewPanels()

	rootGrid = cview.NewGrid()
	rootGrid.SetRows(1, 1, 0, 1)
	rootGrid.SetColumns(0)
	rootGrid.AddItem(headerFlex, 0, 0, 1, 1, 0, 0, false)
	rootGrid.AddItem(headerSep, 1, 0, 1, 1, 0, 0, false)
	rootGrid.AddItem(panels, 2, 0, 1, 1, 0, 0, true)
	rootGrid.AddItem(helpline, 3, 0, 1, 1, 0, 0, false)

	showMainMenu()

	app.SetRoot(rootGrid, true)
	err = app.Run()
	return err
}

func refreshHeader() {
	if rightView == nil {
		return
	}

	appVer := version.Version
	tmplVer := paths.GetTemplatesVersion()

	// App Update Flag
	appUpdateFlag := " "
	appVerTag := "[_ThemeApplicationVersion_]"
	if appUpdateAvailable {
		appUpdateFlag = "[_ThemeApplicationUpdate_]*[_ThemeReset_]"
		appVerTag = "[_ThemeApplicationUpdate_]"
	}

	// Templates Update Flag
	tmplUpdateFlag := " "
	tmplVerTag := "[_ThemeApplicationVersion_]"
	if tmplUpdateAvailable {
		tmplUpdateFlag = "[_ThemeApplicationUpdate_]*[_ThemeReset_]"
		tmplVerTag = "[_ThemeApplicationUpdate_]"
	}

	// Structure: [Flag][Brackets]A:[Version[Brackets]]
	// We use the colors from the tags and let Translate + cview handle the single brackets
	appText := fmt.Sprintf("%s[_ThemeApplicationVersionBrackets_]A:[%s%s[_ThemeApplicationVersionBrackets_]]", appUpdateFlag, appVerTag, appVer)
	tmplText := fmt.Sprintf("%s[_ThemeApplicationVersionBrackets_]T:[%s%s[_ThemeApplicationVersionBrackets_]]", tmplUpdateFlag, tmplVerTag, tmplVer)

	fullText := appText + "[_ThemeApplicationVersionSpace_]" + tmplText

	rightView.SetText(console.Translate(fullText))
}
