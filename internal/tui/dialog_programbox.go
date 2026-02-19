package tui

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/logger"
	"DockSTARTer2/internal/strutil"
	"DockSTARTer2/internal/theme"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/progress"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	zone "github.com/lrstanley/bubblezone/v2"
)

// TaskStatus defines the state of a background task
type TaskStatus int

const (
	StatusWaiting TaskStatus = iota
	StatusInProgress
	StatusCompleted
)

// Task represents a structured progress item
type Task struct {
	Label     string
	Command   string
	Apps      []string
	Status    TaskStatus
	ActiveApp string
}

// ProgramBoxModel represents a dialog that displays streaming program output
type ProgramBoxModel struct {
	title    string
	subtitle string
	command  string // Command being executed (displayed above output)
	viewport viewport.Model
	lines    []string
	done     bool
	err      error
	width    int
	height   int

	// Dialog behavior and auto-close
	isDialog       bool
	maximized      bool
	autoClose      bool
	autoCloseDelay time.Duration
	// AutoExit determines if the dialog should automatically close (and exit app) on success
	AutoExit bool
	task     func(context.Context, io.Writer) error
	focused  bool
	ctx      context.Context

	// Overlay prompts (for blocking prompts during task)
	subDialog     tea.Model
	subDialogChan chan bool

	// Progress tracking
	Tasks    []Task
	Percent  float64
	progress progress.Model
}

// SubDialogMsg signals a request to show a sub-dialog and blocks the task
type SubDialogMsg struct {
	Model tea.Model
	Chan  chan bool
}

// SubDialogResultMsg signals the completion of a sub-dialog
type SubDialogResultMsg struct {
	Result bool
}

// programBoxModel is an alias for backward compatibility
type programBoxModel = ProgramBoxModel

// autoCloseMsg signals that the auto-close delay is over
type autoCloseMsg struct{}

// outputLineMsg carries a new line of output
type outputLineMsg struct {
	line string
}

// outputDoneMsg signals that output is complete
type outputDoneMsg struct {
	err error
}

// UpdateTaskMsg updates a task's status or active app
type UpdateTaskMsg struct {
	Label     string
	Status    TaskStatus
	ActiveApp string
}

// UpdatePercentMsg updates the progress bar percentage
type UpdatePercentMsg struct {
	Percent float64
}

// newProgramBox creates a new program box dialog (internal use)
func newProgramBox(title, subtitle, command string) *ProgramBoxModel {

	m := &ProgramBoxModel{
		title:    title,
		subtitle: subtitle,
		command:  command,
		viewport: viewport.New(),
		lines:    []string{},
		Tasks:    []Task{},
		focused:  true,
		ctx:      context.Background(),
	}

	// Initialize viewport style to match dialog background
	styles := GetStyles()
	m.viewport.Style = styles.Dialog.Padding(0, 0)
	// Use theme-defined console colors to properly display ANSI colors from command output
	m.viewport.Style = m.viewport.Style.
		Background(styles.Console.GetBackground()).
		Foreground(styles.Console.GetForeground())

	// Initialize progress bar with default options
	m.progress = progress.New()

	return m
}

// AddTask adds a task category to the progress header
func (m *ProgramBoxModel) AddTask(label, command string, apps []string) {
	m.Tasks = append(m.Tasks, Task{
		Label:   label,
		Command: command,
		Apps:    apps,
		Status:  StatusWaiting,
	})
}

// UpdateTaskStatus updates a task's state and active app
func (m *ProgramBoxModel) UpdateTaskStatus(label string, status TaskStatus, activeApp string) {
	for i, t := range m.Tasks {
		if t.Label == label {
			m.Tasks[i].Status = status
			m.Tasks[i].ActiveApp = activeApp
			break
		}
	}
}

// SetPercent updates the progress bar percentage (0.0 to 1.0)
func (m *ProgramBoxModel) SetPercent(percent float64) {
	m.Percent = percent
}

// SetMaximized sets whether the dialog should be maximized
func (m *ProgramBoxModel) SetMaximized(maximized bool) {
	m.maximized = maximized
}

// IsMaximized returns whether the dialog is maximized
func (m *ProgramBoxModel) IsMaximized() bool {
	return m.maximized
}

// SetAutoClose sets whether the dialog should auto-close after completion
func (m *ProgramBoxModel) SetAutoClose(autoClose bool, delay time.Duration) {
	m.autoClose = autoClose
	m.autoCloseDelay = delay
}

// NewProgramBoxModel creates a new program box dialog (exported)
func NewProgramBoxModel(title, subtitle, command string) *ProgramBoxModel {
	return newProgramBox(title, subtitle, command)
}

// SetTask sets the task function to execute
func (m *ProgramBoxModel) SetTask(task func(context.Context, io.Writer) error) {
	m.task = task
}

// SetIsDialog sets whether this is a modal dialog overlay
func (m *ProgramBoxModel) SetIsDialog(isDialog bool) {
	m.isDialog = isDialog
}

// SetFocused sets the focus state
func (m *ProgramBoxModel) SetFocused(focused bool) {
	m.focused = focused
}

// startStreamingOutput reads from the provided reader and sends output lines
func startStreamingOutput(reader io.Reader) tea.Cmd {
	return func() tea.Msg {
		scanner := bufio.NewScanner(reader)
		for scanner.Scan() {
			line := scanner.Text()
			// Send line message immediately
			// Note: In Bubble Tea, we can't send multiple messages from one Cmd
			// So we'll batch them or use a different approach
			return outputLineMsg{line: line}
		}

		if err := scanner.Err(); err != nil {
			return outputDoneMsg{err: err}
		}

		return outputDoneMsg{}
	}
}

// streamReader creates a command that continuously reads from the reader
func (m *programBoxModel) streamReader(reader io.Reader) tea.Cmd {
	return func() tea.Msg {
		scanner := bufio.NewScanner(reader)
		if scanner.Scan() {
			return outputLineMsg{line: scanner.Text()}
		}

		if err := scanner.Err(); err != nil {
			return outputDoneMsg{err: err}
		}

		return outputDoneMsg{}
	}
}

func (m *programBoxModel) Init() tea.Cmd {
	// If a task function was set (dialog mode), start it now
	if m.task != nil {
		task := m.task
		m.task = nil // Prevent double-start

		return func() tea.Msg {
			reader, writer := io.Pipe()

			errChan := make(chan error, 1)

			// Start reading output in a goroutine — sends lines to the viewport
			go func() {
				scanner := bufio.NewScanner(reader)
				for scanner.Scan() {
					if program != nil {
						program.Send(outputLineMsg{line: scanner.Text()})
					}
				}
			}()

			// Run the task in a goroutine
			go func() {
				defer writer.Close()
				ctx := m.ctx
				// task is already wrapped with WithTUIWriter if coming from RunCommand
				errChan <- task(ctx, writer)
			}()

			// Log completion happens via errChan if we were using it for sync,
			// but here we just need to send the Done msg.
			// Wait, we need to wait for task to finish to send Done.
			// The goroutine above sends to errChan. We should wait for it.
			go func() {
				err := <-errChan
				if program != nil {
					program.Send(outputDoneMsg{err: err})
				}
			}()

			return nil
		}
	}
	return nil
}

func (m *programBoxModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Handle sub-dialog result specifically (it signals closing of the sub-dialog)
	if resultMsg, ok := msg.(SubDialogResultMsg); ok {
		if m.subDialogChan != nil {
			m.subDialogChan <- resultMsg.Result
			close(m.subDialogChan)
			m.subDialogChan = nil
		}
		m.subDialog = nil
		return m, nil
	}

	// Handle sub-dialog updates if active
	if m.subDialog != nil {
		var cmd tea.Cmd

		// Special case: WindowSizeMsg goes to both to ensure ProgramBox stays sized correctly
		if wsm, ok := msg.(tea.WindowSizeMsg); ok {
			m.width = wsm.Width
			m.height = wsm.Height
			m.subDialog, cmd = m.subDialog.Update(msg)
			// We fall through to let ProgramBox also handle the resize (viewport mainly)
		} else {
			// Exclusive delegations for interaction
			m.subDialog, cmd = m.subDialog.Update(msg)
			return m, cmd
		}
	}

	var cmd tea.Cmd

	switch msg := msg.(type) {
	case SubDialogMsg:
		m.subDialog = msg.Model
		m.subDialogChan = msg.Chan
		// Set size immediately so it can render (it might have missed the first WindowSizeMsg)
		if s, ok := m.subDialog.(interface{ SetSize(int, int) }); ok {
			s.SetSize(m.width, m.height)
		}
		return m, nil

	case CloseDialogMsg:
		if m.subDialog != nil {
			if m.subDialogChan != nil {
				// Handle result if it's a bool (confirmations)
				if r, ok := msg.Result.(bool); ok {
					m.subDialogChan <- r
				} else {
					m.subDialogChan <- false // Default/cancel
				}
			}
			m.subDialog = nil
			m.subDialogChan = nil
			return m, nil
		}

	case tea.WindowSizeMsg:
		// Delegate to SetSize (single source of truth)
		m.SetSize(msg.Width, msg.Height)
		return m, nil

	case outputLineMsg:
		// Convert semantic theme tags to ANSI colors before displaying
		styles := GetStyles()
		rendered := RenderThemeText(msg.line, styles.Console)
		// Truncate to viewport width to prevent overflow past borders
		if m.viewport.Width() > 0 {
			rendered = lipgloss.NewStyle().
				MaxWidth(m.viewport.Width()).
				Render(rendered)
		}
		m.lines = append(m.lines, rendered)
		m.viewport.SetContent(strings.Join(m.lines, "\n"))
		m.viewport.GotoBottom()

		// Continue reading if not done
		if !m.done {
			return m, nil
		}

	case outputDoneMsg:
		m.done = true
		m.err = msg.err
		m.viewport.SetContent(strings.Join(m.lines, "\n"))
		m.viewport.GotoBottom()

		// If AutoExit is enabled and no error occurred, close immediately
		if m.AutoExit && m.err == nil {
			return m, func() tea.Msg { return CloseDialogMsg{} }
		}

		return m, nil

	case tea.KeyPressMsg:
		closeDialog := func() tea.Msg { return CloseDialogMsg{} }
		switch {
		case key.Matches(msg, Keys.Esc):
			if m.done {
				return m, closeDialog
			}

		case key.Matches(msg, Keys.ForceQuit):
			return m, closeDialog

		case key.Matches(msg, Keys.Enter), msg.String() == "o", msg.String() == "O", key.Matches(msg, Keys.Space):
			if m.done {
				return m, closeDialog
			}
			// Important: consume these keys even if not done to prevent them from bubbling up
			// or triggering background elements (like the header)
			return m, nil

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

	case tea.MouseClickMsg:
		// Check if OK button was clicked (auto-generated zone ID: "Button.OK")
		if m.done {
			if zoneInfo := zone.Get("Button.OK"); zoneInfo != nil {
				if zoneInfo.InBounds(msg) {
					return m, func() tea.Msg { return CloseDialogMsg{} }
				}
			}
		}

	case UpdateTaskMsg:
		m.UpdateTaskStatus(msg.Label, msg.Status, msg.ActiveApp)
		return m, nil

	case UpdatePercentMsg:
		m.SetPercent(msg.Percent)
		return m, nil
	}

	// Update viewport for scrolling
	m.viewport, cmd = m.viewport.Update(msg)

	// Handle mouse wheel scrolling for the program box viewport
	if mwMsg, ok := msg.(tea.MouseWheelMsg); ok {
		if mwMsg.Button == tea.MouseWheelUp {
			m.viewport.ScrollUp(3)
			return m, nil
		}
		if mwMsg.Button == tea.MouseWheelDown {
			m.viewport.ScrollDown(3)
			return m, nil
		}
	}

	return m, cmd
}

// ViewString returns the dialog content as a string for compositing
func (m *programBoxModel) ViewString() string {
	if m.width == 0 {
		return ""
	}

	styles := GetStyles()

	// Calculate scroll percentage
	scrollPercent := m.viewport.ScrollPercent()

	// Add scroll indicator at bottom of viewport content
	scrollIndicator := styles.TagKey.
		Bold(true).
		Render(fmt.Sprintf("%d%%", int(scrollPercent*100)))

	// Use console background for the spacer row
	// Apply background maintenance to captured output to prevent resets from bleeding
	viewportContent := MaintainBackground(m.viewport.View(), styles.Console)
	// viewportWithScroll := viewportContent + "\n" +
	// 	lipgloss.NewStyle().
	// 		Width(m.viewport.Width).
	// 		Align(lipgloss.Center).
	// 		Background(styles.Console.GetBackground()).
	// 		Render(scrollIndicator)

	// Wrap viewport in rounded inner border with console background
	viewportStyle := styles.Console.
		Padding(0, 0) // Remove side padding inside inner box for a tighter look
	viewportStyle = ApplyRoundedBorder(viewportStyle, styles.LineCharacters)

	// Apply scroll indicator manually to bottom border
	// We disable the bottom border initially to let us construct it ourselves
	viewportStyle = viewportStyle.BorderBottom(false)

	borderedViewport := viewportStyle.
		Height(m.viewport.Height()).
		Render(viewportContent)

	// Construct custom bottom border with label
	border := styles.Border
	width := m.viewport.Width() + 2 // Add 2 for left/right padding of viewportStyle
	labelWidth := lipgloss.Width(scrollIndicator)

	// Determine T-connectors based on line style
	var leftT, rightT string
	if styles.LineCharacters {
		// Use inverse T connectors for bottom border
		leftT = "┤"
		rightT = "├"
	} else {
		leftT = "|"
		rightT = "|"
	}

	// Calculate padding for label to place it on the right
	// We want it close to the right corner, e.g., 2 chars padding
	rightPadCnt := 2

	// Ensure we have enough space
	totalLabelWidth := 1 + labelWidth + 1 // connector + label + connector
	if width < totalLabelWidth+rightPadCnt+2 {
		// Fallback to center if too narrow
		rightPadCnt = (width - totalLabelWidth) / 2
	}

	// Correct math for bottom line length:
	// Corner(1) + LeftPad + Connector(1) + Label + Connector(1) + RightPad + Corner(1) = width
	// LeftPad + RightPad + Label + 4 = width
	leftPadCnt := width - labelWidth - 4 - rightPadCnt
	if leftPadCnt < 0 {
		leftPadCnt = 0
		rightPadCnt = width - labelWidth - 4
		if rightPadCnt < 0 {
			rightPadCnt = 0
		}
	}

	// Style for border segments (match ApplyRoundedBorder logic)
	borderStyle := lipgloss.NewStyle().
		Foreground(styles.Border2Color).
		Background(styles.Dialog.GetBackground())

	// Build bottom line parts
	// Left part: BottomLeftCorner + HorizontalLine...
	leftPart := borderStyle.Render(border.BottomLeft + strutil.Repeat(border.Bottom, leftPadCnt))

	// Connectors
	leftConnector := borderStyle.Render(leftT)
	rightConnector := borderStyle.Render(rightT)

	// Right part: ...HorizontalLine + BottomRightCorner
	rightPart := borderStyle.Render(strutil.Repeat(border.Bottom, rightPadCnt) + border.BottomRight)

	// Combine parts: Left-----┤100%├--Right
	bottomLine := lipgloss.JoinHorizontal(lipgloss.Bottom, leftPart, leftConnector, scrollIndicator, rightConnector, rightPart)

	// Append custom bottom line to viewport
	// Use strings.Join to avoid extra newlines often added by lipgloss.JoinVertical
	borderedViewport = strings.TrimSuffix(borderedViewport, "\n")
	borderedViewport = borderedViewport + "\n" + bottomLine

	// Calculate content width based on viewport (matches borderedViewport width)
	// viewport.Width() + border (2) = viewport.Width() + 2
	contentWidth := m.viewport.Width() + 2

	// Build command display using theme semantic tags
	var commandDisplay string
	if m.command != "" {
		// Use RenderThemeText for robust parsing of embedded tags/colors
		// We use the console style as base, but DO NOT force the background color onto the whole bar
		// This allows the user to have unstyled spaces or mixed colors.
		// Use styles.Dialog as base so unstyled text matches the dialog background
		base := styles.Dialog
		renderedCmd := RenderThemeText(m.command, base)

		// Use lipgloss to render the row so width and background are handled correctly
		// even with ANSI codes in renderedCmd.
		// Use lipgloss to render the row so width and background are handled correctly
		// even with ANSI codes in renderedCmd.
		commandDisplay = lipgloss.NewStyle().
			Width(contentWidth).
			Background(styles.Dialog.GetBackground()).
			Render(renderedCmd)
	}

	// Render OK button using the standard button helper (ensures consistency)
	// Zone marking is handled automatically by RenderCenteredButtons (zone ID: "Button.OK")
	buttonRow := RenderCenteredButtons(
		contentWidth,
		ButtonSpec{Text: "OK", Active: m.done},
	)

	// Build dialog content
	var contentParts []string

	// Header UI: Title + Tasks + Progress Bar
	headerUI := m.renderHeaderUI(contentWidth)
	if headerUI != "" {
		contentParts = append(contentParts, headerUI)
	}

	if commandDisplay != "" {
		contentParts = append(contentParts, commandDisplay)
	}
	contentParts = append(contentParts, borderedViewport)
	contentParts = append(contentParts, buttonRow)

	// Use JoinVertical to ensure all parts are correctly combined with their heights
	content := lipgloss.JoinVertical(lipgloss.Left, contentParts...)

	// If maximized, force total content height to match the budget (minus outer borders)
	// This ensures the dialog box itself is the correct size as requested.
	if m.maximized {
		content = lipgloss.NewStyle().
			Height(m.height - 2).
			Background(styles.Dialog.GetBackground()).
			Render(content)
	}

	// Wrap in border with title embedded (matching menu style)
	dialogWithTitle := RenderDialog(m.title, content, true)

	// Add shadow (matching menu style)
	dialogWithTitle = AddShadow(dialogWithTitle)

	// If error occurred, show it (suppressing "user aborted" which is just a cancellation)
	if m.err != nil && m.err != ErrUserAborted && !errors.Is(m.err, console.ErrUserAborted) {
		errStyle := SemanticStyle("{{|Theme_Error|}}")
		errView := RenderDialog("Error", errStyle.Render(m.err.Error()), true)
		errView = AddShadow(errView)
		dialogWithTitle = Overlay(errView, dialogWithTitle, OverlayCenter, OverlayCenter, 0, 0)
	}

	// If sub-dialog is active, overlay it
	if m.subDialog != nil {
		var subView string
		if vs, ok := m.subDialog.(interface{ ViewString() string }); ok {
			subView = vs.ViewString()
		} else {
			subView = fmt.Sprintf("%v", m.subDialog.View())
		}
		// Overlay sub-dialog on top of the program box content
		dialogWithTitle = Overlay(subView, dialogWithTitle, OverlayCenter, OverlayCenter, 0, 0)
	}

	return dialogWithTitle
}

// renderHeaderUI renders the tasks and progress bar
func (m *ProgramBoxModel) renderHeaderUI(width int) string {
	if len(m.Tasks) == 0 && m.Percent == 0 {
		return ""
	}

	var b strings.Builder
	styles := GetStyles()
	bgStyle := lipgloss.NewStyle().Background(styles.Dialog.GetBackground())

	// Tasks
	maxLabelLen := 0
	for _, t := range m.Tasks {
		if len(t.Label) > maxLabelLen {
			maxLabelLen = len(t.Label)
		}
	}

	for _, t := range m.Tasks {
		// Category status colors
		catStyle := SemanticStyle("{{|Theme_ProgressWaiting|}}")
		statusText := " Waiting "
		switch t.Status {
		case StatusInProgress:
			catStyle = SemanticStyle("{{|Theme_ProgressInProgress|}}")
			statusText = " In Progress "
		case StatusCompleted:
			catStyle = SemanticStyle("{{|Theme_ProgressCompleted|}}")
			statusText = " Completed "
		}

		// Calculate gap width: padding to maxLabelLen + 2 spaces
		gapWidth := maxLabelLen - len(t.Label) + 2
		// Explicitly use space characters to ensure the background color is visible
		gap := bgStyle.Render(strutil.Repeat(" ", gapWidth))

		headerLine := lipgloss.JoinHorizontal(lipgloss.Top,
			bgStyle.Render("  "), // 2-char margin
			catStyle.Render(t.Label),
			gap,
			catStyle.Render("[ "+statusText+" ]"),
		)
		// Ensure the line covers full width to avoid black bars on the right
		fullHeaderLine := bgStyle.Width(width).Render(headerLine)
		b.WriteString(fullHeaderLine + "\n")

		// Command line: Indent Command App1 App2 ...
		if t.Command != "" || len(t.Apps) > 0 {
			appLine := "     " // 2 spaces margin + 3 spaces indent
			if t.Command != "" {
				appLine += catStyle.Render(t.Command) + " "
			}

			foundActive := false
			for i, app := range t.Apps {
				appStyle := SemanticStyle("{{|Theme_ProgressWaiting|}}")
				if t.Status == StatusCompleted {
					appStyle = SemanticStyle("{{|Theme_ProgressCompleted|}}")
				} else if t.Status == StatusInProgress {
					if t.ActiveApp != "" {
						if app == t.ActiveApp {
							appStyle = SemanticStyle("{{|Theme_ProgressInProgress|}}")
							foundActive = true
						} else if !foundActive {
							appStyle = SemanticStyle("{{|Theme_ProgressCompleted|}}")
						} else {
							appStyle = SemanticStyle("{{|Theme_ProgressWaiting|}}")
						}
					} else {
						// Sub-apps inherit parent status if no specific active app
						appStyle = SemanticStyle("{{|Theme_ProgressInProgress|}}")
					}
				}

				if app == t.ActiveApp {
					appLine += SemanticStyle("{{|Theme_Highlight|}}").Render(app)
				} else {
					appLine += appStyle.Render(app)
				}

				if i < len(t.Apps)-1 {
					appLine += " "
				}
			}
			// Wrap app line if too long AND ensure background maintenance
			// Width is (width - 2) because we want a 2-char right margin too
			wrapped := lipgloss.NewStyle().
				Width(width).
				Background(bgStyle.GetBackground()).
				PaddingLeft(0). // Alignment handled by appLine string
				Render(appLine)
			b.WriteString(wrapped + "\n")
		}
	}

	// Progress Bar
	if m.Percent > 0 {
		barMargin := 4                    // 2 spaces each side
		barWidth := width - barMargin - 4 // [ ... ] padding
		if barWidth < 1 {                 // Ensure barWidth is at least 1
			barWidth = 1
		}
		m.progress.SetWidth(barWidth)

		barRow := bgStyle.Render("  ") + // Left margin
			styles.Dialog.Render("[ ") +
			m.progress.ViewAs(m.Percent) +
			styles.Dialog.Render(" ]")

		// Fill to full width
		fullBarRow := bgStyle.Width(width).Render(barRow)
		b.WriteString(fullBarRow + "\n\n")
	}

	return b.String()
}

// calculateHeaderHeight returns the estimated height of the header UI
func (m *ProgramBoxModel) calculateHeaderHeight() int {
	height := 0
	for _, t := range m.Tasks {
		height += 1 // Label/Status
		if t.Command != "" || len(t.Apps) > 0 {
			height += 1 // App list
		}
	}
	if m.Percent > 0 {
		height += 1 // Bar (no extra \n)
	}
	// Note: spacings are already added in the loop and per-section.
	// We don't add extra safety line here as it creates gaps.
	return height
}

func (m *programBoxModel) View() tea.View {
	v := tea.NewView(m.ViewString())
	v.MouseMode = tea.MouseModeAllMotion
	v.AltScreen = true
	return v
}

// SetSize updates the dialog dimensions (called by AppModel on window resize).
func (m *programBoxModel) SetSize(w, h int) {
	m.width = w
	m.height = h

	commandHeight := 1
	if m.command == "" {
		commandHeight = 0
	}

	headerHeight := m.calculateHeaderHeight()

	// Width calculation:
	// Account for internal overhead: outer dialog border (2) + inner viewport border (2) = 4
	vpWidth := m.width - 4
	if vpWidth < 20 {
		vpWidth = 20
	}
	m.viewport.SetWidth(vpWidth)

	// Height calculation:
	// Find number of top-level content parts for JoinVertical
	numParts := 2 // viewport box + buttons
	if commandHeight > 0 {
		numParts++
	}
	if headerHeight > 0 {
		numParts++
	}

	// Overhead:
	// - outer dialog border (2)
	// - inner viewport border (2)
	// - separators between joined parts (numParts - 1)
	// - header UI lines (headerHeight)
	// - command line (commandHeight)
	// - buttons at bottom (3)
	overhead := 4 + (numParts - 1) + headerHeight + commandHeight + 3
	vpHeight := m.height - overhead

	if vpHeight < 2 {
		vpHeight = 2
	}
	m.viewport.SetHeight(vpHeight)
}

// GetHelpText returns the dynamic help text based on the current state
// Implements DynamicHelpProvider interface for use with DialogWithBackdrop
func (m *programBoxModel) GetHelpText() string {
	scrollInfo := ""
	if m.viewport.TotalLineCount() > m.viewport.VisibleLineCount() {
		scrollPercent := m.viewport.ScrollPercent()
		scrollInfo = fmt.Sprintf(" | %d%%", int(scrollPercent*100))
	}

	if m.done {
		if m.err != nil {
			return "Error: " + m.err.Error() + scrollInfo + " | Press Enter or Esc to close"
		}
		return "Complete" + scrollInfo + " | Press Enter or Esc to close | PgUp/PgDn to scroll"
	}
	return "Running..." + scrollInfo + " | Press Ctrl+C to cancel | PgUp/PgDn to scroll"
}

// RunProgramBox displays a program box dialog that shows command output
func RunProgramBox(ctx context.Context, title, subtitle string, task func(context.Context, io.Writer) error) error {
	// Initialize global zone manager for mouse support (safe to call multiple times)
	zone.NewGlobal()

	// Enable TUI mode for console prompts
	console.SetTUIEnabled(true)
	defer console.SetTUIEnabled(false)

	logger.TUIMode = true
	defer func() { logger.TUIMode = false }()

	// Initialize TUI if not already done
	cfg := config.LoadAppConfig()

	currentConfig = cfg // Set global config so styles like AddShadow work correctly
	if err := theme.Load(cfg.UI.Theme); err == nil {
		InitStyles(cfg)
	}

	// Create dialog model
	dialogModel := NewProgramBoxModel(title, subtitle, subtitle)
	dialogModel.ctx = ctx
	dialogModel.SetTask(task)
	dialogModel.SetMaximized(true)

	// If -y flag was passed, enable AutoExit
	if console.GlobalYes {
		dialogModel.AutoExit = true
	}

	// Create full app model with standalone dialog to include log panel and backdrop
	model := NewAppModelStandalone(ctx, currentConfig, dialogModel)

	// Create Bubble Tea program
	p := tea.NewProgram(model)

	// Set global program variable so ProgramBoxModel.Init can send messages
	program = p
	console.TUIConfirm = PromptConfirm
	defer func() {
		program = nil
		console.TUIConfirm = nil
	}()

	// Run the program (Init will start the task)
	finalModel, err := p.Run()

	// Reset terminal colors on exit to prevent "bleeding" into the shell prompt
	fmt.Print("\x1b[0m\n")

	if err != nil {
		return err
	}

	// Extract details from the model
	if app, ok := finalModel.(AppModel); ok {
		if app.Fatal {
			logger.TUIMode = false
			console.AbortHandler(ctx)
			return console.ErrUserAborted
		}
		if box, ok := app.dialog.(*ProgramBoxModel); ok {
			return box.err
		}
	}

	return nil
}
