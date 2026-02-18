package tui

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/logger"
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
	// Title is parsed by RenderDialog when View() is called.
	// Subtitle/Command is parsed in View().

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

	// Initialize viewport style to match dialog background (fixes black scrollbar)
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
			// Create a pipe for streaming output
			reader, writer := io.Pipe()

			// Channel to signal task is done
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

		case key.Matches(msg, Keys.Enter):
			if m.done {
				return m, closeDialog
			}
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
	}

	// Update viewport for scrolling
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

// ViewString returns the dialog content as a string for compositing
func (m *programBoxModel) ViewString() string {
	if m.width == 0 {
		return ""
	}

	// If sub-dialog is active, show it instead of logs
	if m.subDialog != nil {
		var viewStr string
		if vs, ok := m.subDialog.(interface{ ViewString() string }); ok {
			viewStr = vs.ViewString()
		} else {
			viewStr = fmt.Sprintf("%v", m.subDialog.View())
		}

		return lipgloss.Place(
			m.width, m.height,
			lipgloss.Center, lipgloss.Center,
			viewStr,
			lipgloss.WithWhitespaceChars(" "),
		)
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

	borderedViewport := viewportStyle.Render(viewportContent)

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
	leftPart := borderStyle.Render(border.BottomLeft + strings.Repeat(border.Bottom, leftPadCnt))

	// Connectors
	leftConnector := borderStyle.Render(leftT)
	rightConnector := borderStyle.Render(rightT)

	// Right part: ...HorizontalLine + BottomRightCorner
	rightPart := borderStyle.Render(strings.Repeat(border.Bottom, rightPadCnt) + border.BottomRight)

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
		ButtonSpec{Text: " OK ", Active: m.done},
	)

	// Build dialog content
	var contentParts []string
	if commandDisplay != "" {
		contentParts = append(contentParts, commandDisplay)
	}
	contentParts = append(contentParts, borderedViewport)
	// Trim newlines from each part to ensure tight vertical stacking
	// and remove horizontal space above/below.
	var contentPartsCleaned []string
	if commandDisplay != "" {
		contentPartsCleaned = append(contentPartsCleaned, strings.Trim(commandDisplay, "\n"))
	}
	contentPartsCleaned = append(contentPartsCleaned, strings.Trim(borderedViewport, "\n"))
	contentPartsCleaned = append(contentPartsCleaned, strings.Trim(buttonRow, "\n"))

	content := strings.Join(contentPartsCleaned, "\n")

	// Remove padding to content (border will be added by RenderDialogWithTitle)
	// We want the inner border to be flush against the outer border
	paddedContent := styles.Dialog.
		Padding(0, 0).
		Render(content)

	// Wrap in border with title embedded (matching menu style)
	dialogWithTitle := RenderDialog(m.title, paddedContent, true)

	// Add shadow (matching menu style)
	dialogWithTitle = AddShadow(dialogWithTitle)

	// Just return the dialog content - backdrop will be handled by overlay
	return dialogWithTitle
}

func (m *programBoxModel) View() tea.View {
	return tea.NewView(m.ViewString())
}

// SetSize updates the dialog dimensions (called by AppModel on window resize).
func (m *programBoxModel) SetSize(w, h int) {
	m.width = w
	m.height = h

	cfg := config.LoadAppConfig()
	shadowWidth := 0
	shadowHeight := 0
	if cfg.UI.Shadow {
		shadowWidth = 2
		shadowHeight = 1
	}

	commandHeight := 1
	if m.command == "" {
		commandHeight = 0
	}

	// Width calculation:
	// If maximized, fill available width (only subtract shadow/borders)
	// If not maximized, use global margins (4)
	marginW := 4
	if m.maximized {
		marginW = 0
	}
	vpWidth := m.width - marginW - shadowWidth - 4 // -4 for borders (2 outer + 2 inner)
	if vpWidth < 20 {
		vpWidth = 20
	}
	m.viewport.SetWidth(vpWidth)

	// Height calculation:
	// If maximized, fill available height (only subtract shadow/borders/chrome)
	// If not maximized, use global margins (4)
	marginH := 4
	if m.maximized {
		marginH = 0
	}
	vpHeight := m.height - marginH - shadowHeight - 4 - commandHeight - 3 // -4 for borders, -3 for buttons
	if vpHeight < 5 {
		vpHeight = 5
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
	logger.Debug(ctx, "RunProgramBox config: Shadow=%v, ShadowLevel=%d, LineCharacters=%v", cfg.UI.Shadow, cfg.UI.ShadowLevel, cfg.UI.LineCharacters)
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
			return fmt.Errorf("application force quit")
		}
		if box, ok := app.dialog.(*ProgramBoxModel); ok {
			return box.err
		}
	}

	return nil
}
