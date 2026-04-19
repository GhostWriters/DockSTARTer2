package tui

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"DockSTARTer2/internal/commands"
	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/logger"
	"DockSTARTer2/internal/version"
	"DockSTARTer2/internal/tui/components/sinput"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

const (
	logPanelZoneID    = IDLogToggle
	logResizeZoneID   = IDLogResize
	logViewportZoneID = IDLogViewport
)

// ─── Message types ────────────────────────────────────────────────────────────

// logLineMsg carries a new log line from the subscription channel.
type logLineMsg string

// toggleLogPanelMsg requests the log panel to expand or collapse.
type toggleLogPanelMsg struct{}

// consoleLinesMsg carries a batch of lines from a running console command.
type consoleLinesMsg struct{ lines []string }

// consoleDoneMsg signals that a console command has finished.
type consoleDoneMsg struct {
	err           error
	configChanged bool // reload TUI styles (ConfigChangedMsg)
	appsChanged   bool // refresh app list (RefreshAppsListMsg)
}

// ─── Model ───────────────────────────────────────────────────────────────────

// LogPanelModel is the slide-up console panel that lives below the helpline.
// When collapsed it shows only a 1-line toggle strip (^).
// When expanded it shows a log/output viewport and a single-line input bar.
type LogPanelModel struct {
	expanded bool
	focused  bool
	viewport viewport.Model
	lines    []string
	width    int
	// totalHeight is the full height of the terminal (used for max constraint)
	totalHeight int
	// height is the current variable height of the panel (when expanded)
	height int

	// Resizing state
	resizeDrag ScrollbarDragState

	// maxHeight is the externally imposed height ceiling (set by AppModel).
	// Zero means "no override" — logPanelMaxHeight() is used as the fallback.
	maxHeight int

	// Console input bar
	inputFocused bool
	input        sinput.Model
	history      []string // in-session command history, oldest first
	historyIdx   int      // -1 = new command; >=0 = navigating history
	historyDraft string   // saved in-progress text when navigating up

	// Running console command state
	consoleScanner       *bufio.Scanner
	consoleCancel        context.CancelFunc
	consoleConfigChanged bool
	consoleAppsChanged   bool

	// sessionActive locks the input bar while a TUI screen is busy.
	sessionActive bool
}

// applyInputStyles updates the sinput colours from the current theme.
func (m *LogPanelModel) applyInputStyles() {
	styles := GetStyles()
	bg := styles.Dialog.GetBackground()
	tiStyles := textinput.DefaultStyles(true)
	tiStyles.Focused.Prompt = styles.ItemNormal.Background(bg)
	tiStyles.Focused.Text = styles.ItemNormal.Background(bg)
	tiStyles.Blurred.Prompt = styles.ItemNormal.Background(bg)
	tiStyles.Blurred.Text = styles.ItemNormal.Background(bg)
	m.input.SetStyles(tiStyles)
}

// NewLogPanelModel creates a new console panel in collapsed state.
func NewLogPanelModel() LogPanelModel {
	vp := viewport.New()

	ti := textinput.New()
	ti.Prompt = "> "
	inp := sinput.New(ti)

	m := LogPanelModel{
		viewport:   vp,
		input:      inp,
		historyIdx: -1,
	}
	m.applyInputStyles()
	return m
}

// CollapsedHeight returns the height the panel always occupies (the toggle strip).
func (m LogPanelModel) CollapsedHeight() int { return 1 }

// Height returns the current rendered height of the panel.
func (m LogPanelModel) Height() int {
	if m.expanded {
		if m.height > 2 {
			return m.height
		}
		if m.totalHeight > 2 {
			return m.totalHeight / 2
		}
	}
	return 1
}

// SetMaxHeight updates the externally imposed height ceiling. Pass 0 to revert to default.
func (m *LogPanelModel) SetMaxHeight(h int) { m.maxHeight = h }

// SetSessionActive locks or unlocks the input bar.
func (m *LogPanelModel) SetSessionActive(active bool) {
	if active && m.inputFocused {
		m.input.Blur()
		m.inputFocused = false
	}
	m.sessionActive = active
}

// applyDragY computes the new panel height from the current mouse Y.
func (m *LogPanelModel) applyDragY(mouseY int) {
	delta := m.resizeDrag.StartMouseY - mouseY
	newHeight := m.resizeDrag.StartThumbTop + delta
	maxH := m.effectiveMaxHeight()
	if newHeight > maxH {
		newHeight = maxH
	}
	if newHeight < 5 {
		newHeight = 5
	}
	m.height = newHeight
	if m.expanded {
		vpH := m.height - 4
		if vpH < 1 {
			vpH = 1
		}
		m.viewport.SetHeight(vpH)
		m.viewport.SetYOffset(m.viewport.YOffset())
	}
}

// effectiveMaxHeight returns the ceiling used for clamping.
func (m *LogPanelModel) effectiveMaxHeight() int {
	if m.maxHeight > 0 {
		return m.maxHeight
	}
	return logPanelMaxHeight(m.totalHeight)
}

// SetSize stores dimensions and adjusts the viewport and input bar.
func (m *LogPanelModel) SetSize(width, totalTermHeight int) {
	m.width = width
	m.totalHeight = totalTermHeight

	m.viewport.SetWidth(width - ScrollbarGutterWidth)
	// Input box inner width: outer panel width - 2 (outer box) - 2 (inner border)
	m.input.SetWidth(width - 4)

	if m.expanded {
		if m.height == 0 {
			m.height = totalTermHeight / 2
		}
		maxH := m.effectiveMaxHeight()
		if m.height > maxH {
			m.height = maxH
		}
		if m.height < 5 {
			m.height = 5
		}

		vpH := m.height - 4 // subtract toggle strip + 3-row input box
		if vpH < 1 {
			vpH = 1
		}
		prevH := m.viewport.Height()
		m.viewport.SetHeight(vpH)
		if prevH == 0 && vpH > 0 && len(m.lines) > 0 {
			content := strings.Join(m.lines, "\n")
			m.viewport.SetContent(content)
			m.viewport.GotoBottom()
		} else if vpH > 0 {
			m.viewport.SetYOffset(m.viewport.YOffset())
		}
	}
}

// Init preloads the log file and starts the live subscription.
func (m LogPanelModel) Init() tea.Cmd {
	return tea.Batch(
		func() tea.Msg { return preloadLogFile() },
		waitForLogLine(),
	)
}

// preloadLogFile reads the last 200 lines of the log file.
func preloadLogFile() tea.Msg {
	path := logger.GetLogFilePath()
	if path == "" {
		return nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	var all []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		all = append(all, sc.Text())
	}

	const maxLines = 200
	if len(all) > maxLines {
		all = all[len(all)-maxLines:]
	}
	if len(all) == 0 {
		return nil
	}
	return logLineMsg(strings.Join(all, "\n"))
}

// waitForLogLine blocks until the logger sends a line, then returns it as a message.
func waitForLogLine() tea.Cmd {
	return func() tea.Msg {
		line, ok := <-logger.SubscribeLogLines()
		if !ok {
			return nil
		}
		return logLineMsg(line)
	}
}

// ─── Command execution ────────────────────────────────────────────────────────

// runShellCmd runs cmdStr as a shell command, streaming output to w.
func runShellCmd(ctx context.Context, cmdStr string, w io.Writer) error {
	var shellCmd *exec.Cmd
	if runtime.GOOS == "windows" {
		shellCmd = exec.CommandContext(ctx, "cmd", "/c", cmdStr)
	} else {
		shellCmd = exec.CommandContext(ctx, "sh", "-c", cmdStr)
	}
	shellCmd.Stdout = w
	shellCmd.Stderr = w
	return shellCmd.Run()
}

// readConsoleBatch reads up to 50 lines from the scanner and returns them as a
// consoleLinesMsg, or consoleDoneMsg on EOF.
func readConsoleBatch(sc *bufio.Scanner, cancel context.CancelFunc) tea.Cmd {
	return readConsoleBatchWithFlag(sc, cancel, false, false)
}

// readConsoleBatchWithFlag is like readConsoleBatch but carries post-execution
// flags so AppModel can trigger config reload and/or app list refresh.
func readConsoleBatchWithFlag(sc *bufio.Scanner, cancel context.CancelFunc, configChanged, appsChanged bool) tea.Cmd {
	return func() tea.Msg {
		var batch []string
		for i := 0; i < 50 && sc.Scan(); i++ {
			batch = append(batch, sc.Text())
		}
		if len(batch) > 0 {
			return consoleLinesMsg{lines: batch}
		}
		cancel()
		return consoleDoneMsg{err: sc.Err(), configChanged: configChanged, appsChanged: appsChanged}
	}
}

// isDS2Prefix reports whether tok is a recognized ds2 command prefix —
// the detected binary name (e.g. "dockstarter2"), "ds2", or "ds".
func isDS2Prefix(tok string) bool {
	lower := strings.ToLower(tok)
	cmdName := strings.ToLower(version.CommandName)
	return lower == cmdName || lower == "ds2" || lower == "ds"
}


// submitConsoleCommand parses and runs cmdStr.
// ds2 commands (starting with - or prefixed with "ds2") are executed internally
// via commands.Parse + commands.Execute; output flows through the logger subscription.
// Everything else is run as a shell command and streamed via the pipe/scanner path.
func (m *LogPanelModel) submitConsoleCommand(cmdStr string) tea.Cmd {
	tokens := strings.Fields(cmdStr)
	if len(tokens) == 0 {
		return nil
	}

	logger.Notice(context.Background(), "Console command: '{{|UserCommand|}}%s{{[-]}}'", cmdStr)

	isDS2 := isDS2Prefix(tokens[0])
	args := tokens
	if isDS2 {
		args = tokens[1:]
	}

	// ds2 command: prefixed with ds/ds2/executable, or first token starts with -
	if isDS2 || (len(args) > 0 && strings.HasPrefix(args[0], "-")) {
		groups, err := commands.Parse(args)

		if err != nil {
			logger.Error(context.Background(), "%s", err.Error())
			return func() tea.Msg { return consoleDoneMsg{} }
		}

		configChanged := commands.GroupsNeedConfigReload(groups)
		appsChanged := commands.GroupsNeedAppsRefresh(groups)

		ctx, cancel := context.WithCancel(context.Background())
		m.consoleCancel = cancel
		pr, pw := io.Pipe()
		cmdCtx := console.WithTUIWriter(ctx, pw)

		go func() {
			commands.Execute(cmdCtx, groups)
			pw.Close()
		}()

		sc := bufio.NewScanner(pr)
		m.consoleScanner = sc
		m.consoleConfigChanged = configChanged
		m.consoleAppsChanged = appsChanged
		return readConsoleBatchWithFlag(sc, cancel, configChanged, appsChanged)
	}

	// Shell command
	ctx, cancel := context.WithCancel(context.Background())
	m.consoleCancel = cancel

	pr, pw := io.Pipe()
	go func() {
		err := runShellCmd(ctx, cmdStr, pw)
		pw.CloseWithError(err)
	}()

	sc := bufio.NewScanner(pr)
	m.consoleScanner = sc
	return readConsoleBatch(sc, cancel)
}

// appendConsoleLines adds rendered lines to the scrollback.
func (m *LogPanelModel) appendConsoleLines(lines []string) {
	styles := GetStyles()
	targetWidth := m.viewport.Width()
	if targetWidth <= 0 && m.width > 0 {
		targetWidth = m.width - ScrollbarGutterWidth
	}
	for _, line := range lines {
		rendered := RenderConsoleText(line, styles.Console)
		if targetWidth > 0 {
			rendered = lipgloss.NewStyle().MaxWidth(targetWidth).Render(rendered)
		}
		m.lines = append(m.lines, rendered)
	}
	content := strings.Join(m.lines, "\n")
	m.viewport.SetContent(content)
	if m.viewport.Height() > 0 {
		m.viewport.GotoBottom()
	}
}

// ─── Update ───────────────────────────────────────────────────────────────────

func (m LogPanelModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case sinput.PasteMsg, sinput.CutMsg, sinput.SelectAllMsg:
		if m.inputFocused {
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)
			return m, cmd
		}
		return m, nil

	case ConfigChangedMsg:
		m.applyInputStyles()
		return m, nil

	case logLineMsg:
		m.appendConsoleLines(strings.Split(string(msg), "\n"))
		return m, waitForLogLine()

	case consoleLinesMsg:
		m.appendConsoleLines(msg.lines)
		return m, readConsoleBatchWithFlag(m.consoleScanner, m.consoleCancel, m.consoleConfigChanged, m.consoleAppsChanged)

	case consoleDoneMsg:
		m.consoleScanner = nil
		m.consoleCancel = nil
		if !m.sessionActive {
			m.inputFocused = true
			cmd := m.input.Focus()
			return m, tea.Batch(cmd, sinput.Blink)
		}
		return m, nil

	case toggleLogPanelMsg:
		m.expanded = !m.expanded
		if m.expanded {
			m.SetSize(m.width, m.totalHeight)
			content := strings.Join(m.lines, "\n")
			m.viewport.SetContent(content)
			m.viewport.GotoBottom()
		}
		return m, nil

	case LayerHitMsg:
		if strings.HasSuffix(msg.ID, ".sb.up") {
			if m.expanded && msg.Button != HoverButton {
				m.viewport.ScrollUp(1)
			}
			return m, nil
		}
		if strings.HasSuffix(msg.ID, ".sb.down") {
			if m.expanded && msg.Button != HoverButton {
				m.viewport.ScrollDown(1)
			}
			return m, nil
		}
		if strings.HasSuffix(msg.ID, ".sb.above") {
			if m.expanded && msg.Button != HoverButton {
				m.viewport.HalfPageUp()
			}
			return m, nil
		}
		if strings.HasSuffix(msg.ID, ".sb.below") {
			if m.expanded && msg.Button != HoverButton {
				m.viewport.HalfPageDown()
			}
			return m, nil
		}
		if msg.ID == IDConsoleInput && msg.Button == tea.MouseRight {
			return m, ShowInputContextMenu(m.input, msg.X, msg.Y, m.width, m.totalHeight)
		}
		if msg.ID == logPanelZoneID {
			return m, func() tea.Msg { return toggleLogPanelMsg{} }
		}

	case DragDoneMsg:
		if msg.ID == logResizeZoneID {
			m.resizeDrag.DragPending = false
			if m.resizeDrag.PendingDragY != m.resizeDrag.LastDragY {
				m.resizeDrag.LastDragY = m.resizeDrag.PendingDragY
				m.applyDragY(m.resizeDrag.PendingDragY)
				m.resizeDrag.DragPending = true
				return m, DragDoneCmd(logResizeZoneID)
			}
		}
		return m, nil

	case tea.MouseClickMsg:
		if msg.Button == tea.MouseLeft {
			m.resizeDrag.StartDrag(msg.Y, m.height, ScrollbarInfo{})
			if !m.expanded {
				m.expanded = true
				m.height = 1
				m.SetSize(m.width, m.totalHeight)
				m.resizeDrag.StartThumbTop = 1
				content := strings.Join(m.lines, "\n")
				m.viewport.SetContent(content)
				m.viewport.GotoBottom()
			}
			return m, nil
		}

	case tea.MouseReleaseMsg:
		if m.resizeDrag.Dragging {
			m.resizeDrag.StopDrag()
			m.SetSize(m.width, m.totalHeight)
			return m, nil
		}

	case tea.MouseMotionMsg:
		if m.resizeDrag.Dragging {
			m.resizeDrag.PendingDragY = msg.Y
			if !m.resizeDrag.DragPending {
				m.resizeDrag.LastDragY = msg.Y
				m.applyDragY(msg.Y)
				m.resizeDrag.DragPending = true
				return m, DragDoneCmd(logResizeZoneID)
			}
			return m, nil
		}

	case tea.MouseWheelMsg:
		if m.expanded {
			if msg.Button == tea.MouseWheelUp {
				m.viewport.ScrollUp(3)
				return m, nil
			}
			if msg.Button == tea.MouseWheelDown {
				m.viewport.ScrollDown(3)
				return m, nil
			}
		}

	case tea.KeyPressMsg:
		if m.inputFocused {
			return m.updateInputFocused(msg)
		}
		if m.expanded {
			switch {
			case key.Matches(msg, Keys.Home):
				m.viewport.GotoTop()
				return m, nil
			case key.Matches(msg, Keys.End):
				m.viewport.GotoBottom()
				return m, nil
			case key.Matches(msg, Keys.HalfPageUp):
				m.viewport.HalfPageUp()
				return m, nil
			case key.Matches(msg, Keys.HalfPageDown):
				m.viewport.HalfPageDown()
				return m, nil
			}
		}
	}

	var cmd tea.Cmd
	if m.expanded {
		m.viewport, cmd = m.viewport.Update(msg)
	}
	return m, cmd
}

// updateInputFocused handles key events when the input bar has focus.
func (m LogPanelModel) updateInputFocused(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, Keys.Esc):
		m.input.Blur()
		m.inputFocused = false
		return m, nil

	case key.Matches(msg, Keys.Up):
		if len(m.history) == 0 {
			return m, nil
		}
		if m.historyIdx == -1 {
			m.historyDraft = m.input.Value()
			m.historyIdx = len(m.history) - 1
		} else if m.historyIdx > 0 {
			m.historyIdx--
		}
		m.input.SetValue(m.history[m.historyIdx])
		m.input.CursorEnd()
		return m, nil

	case key.Matches(msg, Keys.Down):
		if m.historyIdx == -1 {
			return m, nil
		}
		m.historyIdx++
		if m.historyIdx >= len(m.history) {
			m.historyIdx = -1
			m.input.SetValue(m.historyDraft)
		} else {
			m.input.SetValue(m.history[m.historyIdx])
		}
		m.input.CursorEnd()
		return m, nil

	case key.Matches(msg, Keys.Enter):
		cmdStr := strings.TrimSpace(m.input.Value())
		if cmdStr == "" {
			return m, nil
		}
		m.history = append(m.history, cmdStr)
		m.historyIdx = -1
		m.historyDraft = ""
		m.input.SetValue("")
		m.input.Blur()
		m.inputFocused = false

		// Show the submitted command in the scrollback.
		m.appendConsoleLines([]string{"> " + cmdStr})

		return m, m.submitConsoleCommand(cmdStr)
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

// FocusInput transitions the panel to input-focused state (called from AppModel).
// Returns the Blink command needed to activate the hardware cursor.
func (m *LogPanelModel) FocusInput() tea.Cmd {
	if m.sessionActive || !m.expanded {
		return nil
	}
	m.inputFocused = true
	cmd := m.input.Focus()
	return tea.Batch(cmd, sinput.Blink)
}

// ─── View ─────────────────────────────────────────────────────────────────────

func (m LogPanelModel) ViewString() string {
	ctx := GetActiveContext()

	marker := "^"
	if m.expanded {
		marker = "v"
	}
	title := marker + " Console " + marker

	rightTitle := ""
	if m.focused && m.expanded && !m.inputFocused {
		pct := int(m.viewport.ScrollPercent() * 100)
		rightTitle = fmt.Sprintf(" %d%% ", pct)
	}

	// Input box occupies 3 rows (top border + 1 content + bottom border).
	vpH := m.height - 4
	if vpH < 1 {
		if m.totalHeight > 0 {
			vpH = (m.totalHeight / 2) - 4
		}
		if vpH < 1 {
			vpH = 1
		}
	}
	if m.viewport.Height() != vpH {
		m.viewport.SetHeight(vpH)
	}
	if m.viewport.Width() != m.width-ScrollbarGutterWidth {
		m.viewport.SetWidth(m.width - ScrollbarGutterWidth)
	}

	vpStyle := lipgloss.NewStyle().
		Width(m.viewport.Width()).
		Height(vpH).
		Background(ctx.Console.GetBackground()).
		Foreground(ctx.Console.GetForeground())
	m.viewport.Style = vpStyle

	vpView := MaintainBackground(m.viewport.View(), ctx.Console)
	vpView = ApplyScrollbarColumn(vpView, len(m.lines), vpH, m.viewport.YOffset(), ctx.LineCharacters, ctx)

	// Input box — bordered with submenu styling.
	inputBoxWidth := m.width - 2 // inner content width (outer panel has no side borders)
	m.input.SetWidth(inputBoxWidth - 2)
	if m.sessionActive {
		m.input.Placeholder = "Session active — input locked"
	} else {
		m.input.Placeholder = ""
	}
	inputTitleTag := "TitleSubMenu"
	if m.inputFocused {
		inputTitleTag = "TitleSubMenuFocused"
	}
	inputContent := lipgloss.NewStyle().
		Width(inputBoxWidth - 2).
		Background(ctx.Dialog.GetBackground()).
		Render(m.input.View())
	inputBox := RenderBorderedBoxCtx(
		"{{|"+inputTitleTag+"|}}Command{{[-]}}",
		inputContent,
		inputBoxWidth,
		3,
		m.inputFocused,
		true,
		true,
		ctx.SubmenuTitleAlign,
		inputTitleTag,
		ctx,
	)

	combined := vpView + "\n" + inputBox

	consoleTitleStyle := SemanticRawStyle("ConsoleTitle")
	consoleBorderStyle := SemanticRawStyle("ConsoleBorder")

	return RenderTopBorderBoxCtx(title, rightTitle, combined, m.width, m.focused, consoleTitleStyle, consoleBorderStyle, ctx)
}

// Layers returns a single layer with the panel content for visual compositing.
func (m LogPanelModel) Layers() []*lipgloss.Layer {
	return []*lipgloss.Layer{
		lipgloss.NewLayer(m.ViewString()).Z(ZLogPanel).ID(IDLogPanel),
	}
}

// GetHitRegions implements HitRegionProvider for mouse hit testing.
func (m LogPanelModel) GetHitRegions(offsetX, offsetY int) []HitRegion {
	var regions []HitRegion

	panelHelp := &HelpContext{
		ScreenName: "Console Panel",
		PageTitle:  "Viewer",
		PageText:   "Displays live application logs and accepts ds2/shell commands.",
		ItemTitle:  "Console Panel",
		ItemText:   "Scroll with the mouse wheel or use Home/End/PgUp/PgDn when focused.",
	}
	inputHelp := &HelpContext{
		ScreenName: "Console Panel",
		PageTitle:  "Input",
		PageText:   "Type ds2 commands or shell commands and press Enter to run them.",
		ItemTitle:  "Console Input",
		ItemText:   "Enter: run | Up/Down: history | Esc: exit input",
	}

	ctx := GetActiveContext()
	marker := "^"
	if m.expanded {
		marker = "v"
	}
	title := marker + " Console " + marker

	titleWidth := WidthWithoutZones(RenderThemeText(title, ctx.Dialog))
	titleSectionLen := 1 + 1 + titleWidth + 1 + 1
	actualWidth := m.width - 2
	var leftPad int
	if ctx.LogTitleAlign == "left" {
		leftPad = 0
	} else {
		leftPad = (actualWidth - titleSectionLen) / 2
	}
	if leftPad < 0 {
		leftPad = 0
	}

	titleStart := 1 + leftPad
	titleEnd := titleStart + titleSectionLen

	regions = append(regions, HitRegion{
		ID:     IDLogResize,
		X:      offsetX,
		Y:      offsetY,
		Width:  titleStart,
		Height: 1,
		ZOrder: ZLogPanel + 1,
		Label:  "Console Panel",
		Help:   panelHelp,
	})
	regions = append(regions, HitRegion{
		ID:     IDLogToggle,
		X:      offsetX + titleStart,
		Y:      offsetY,
		Width:  titleSectionLen,
		Height: 1,
		ZOrder: ZLogPanel + 1,
		Label:  "Console Panel",
		Help:   panelHelp,
	})
	regions = append(regions, HitRegion{
		ID:     IDLogResize,
		X:      offsetX + titleEnd,
		Y:      offsetY,
		Width:  m.width - titleEnd,
		Height: 1,
		ZOrder: ZLogPanel + 1,
		Label:  "Console Panel",
		Help:   panelHelp,
	})

	if m.expanded {
		vpH := m.height - 4
		regions = append(regions, HitRegion{
			ID:     IDLogViewport,
			X:      offsetX,
			Y:      offsetY + 1,
			Width:  m.width,
			Height: vpH,
			ZOrder: ZLogPanel + 1,
			Label:  "Console Panel",
			Help:   panelHelp,
		})

		// Input bar region (3 rows: top border + content + bottom border)
		// Text X: 1 (input box left border) + promptWidth
		m.input.SetScreenTextX(offsetX + 1 + m.input.PromptWidth())
		regions = append(regions, HitRegion{
			ID:     IDConsoleInput,
			X:      offsetX,
			Y:      offsetY + 1 + vpH,
			Width:  m.width,
			Height: 3,
			ZOrder: ZLogPanel + 1,
			Label:  "Console Input",
			Help:   inputHelp,
		})

		// Scrollbar hit regions
		if currentConfig.UI.Scrollbar {
			sbInfo := ComputeScrollbarInfo(m.viewport.TotalLineCount(), m.viewport.Height(), m.viewport.YOffset(), vpH)
			if sbInfo.Needed {
				sbX := offsetX + m.viewport.Width()
				sbTopY := offsetY + 1

				regions = append(regions, HitRegion{
					ID: IDLogPanel + ".sb.up", X: sbX, Y: sbTopY,
					Width: 1, Height: 1, ZOrder: ZLogPanel + 2,
				})
				if aboveH := sbInfo.ThumbStart - 1; aboveH > 0 {
					regions = append(regions, HitRegion{
						ID: IDLogPanel + ".sb.above", X: sbX, Y: sbTopY + 1,
						Width: 1, Height: aboveH, ZOrder: ZLogPanel + 2,
					})
				}
				if thumbH := sbInfo.ThumbEnd - sbInfo.ThumbStart; thumbH > 0 {
					regions = append(regions, HitRegion{
						ID: IDLogPanel + ".sb.thumb", X: sbX, Y: sbTopY + sbInfo.ThumbStart,
						Width: 1, Height: thumbH, ZOrder: ZLogPanel + 3,
					})
				}
				if belowH := (sbInfo.Height - 1) - sbInfo.ThumbEnd; belowH > 0 {
					regions = append(regions, HitRegion{
						ID: IDLogPanel + ".sb.below", X: sbX, Y: sbTopY + sbInfo.ThumbEnd,
						Width: 1, Height: belowH, ZOrder: ZLogPanel + 2,
					})
				}
				regions = append(regions, HitRegion{
					ID: IDLogPanel + ".sb.down", X: sbX, Y: sbTopY + sbInfo.Height - 1,
					Width: 1, Height: 1, ZOrder: ZLogPanel + 2,
				})
			}
		}
	}

	return regions
}

// DragScrollbar scrolls the viewport to match the dragged thumb position.
func (m LogPanelModel) DragScrollbar(mouseY int, drag *ScrollbarDragState, sbAbsTopY int, info ScrollbarInfo) (LogPanelModel, bool) {
	total := m.viewport.TotalLineCount()
	visible := m.viewport.Height()
	if total <= visible {
		return m, false
	}
	maxOff := total - visible
	newOff, _ := drag.ScrollOffset(mouseY, sbAbsTopY, maxOff, info)
	if newOff == m.viewport.YOffset() {
		return m, false
	}
	m.viewport.GotoTop()
	m.viewport.ScrollDown(newOff)
	return m, true
}

// logPanelMaxHeight returns the maximum height the console panel may occupy.
func logPanelMaxHeight(totalTermHeight int) int {
	layout := GetLayout()
	shadowH := 0
	if currentConfig.UI.Shadow {
		shadowH = layout.ShadowHeight
	}
	usable := totalTermHeight - layout.ChromeHeight(1) - layout.BottomChrome(layout.HelplineHeight) - shadowH
	maxH := usable / 2
	if maxH < 3 {
		maxH = 3
	}
	return maxH
}

// View renders the panel at its current height.
func (m LogPanelModel) View() tea.View {
	return tea.NewView(m.ViewString())
}
