package tui

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/logger"
	"DockSTARTer2/internal/theme"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	zone "github.com/lrstanley/bubblezone"
	overlay "github.com/rmhubbert/bubbletea-overlay"
)

// programBoxModel represents a dialog that displays streaming program output
type programBoxModel struct {
	title    string
	subtitle string
	command  string // Command being executed (displayed above output)
	viewport viewport.Model
	lines    []string
	done     bool
	err      error
	width    int
	height   int
}

// outputLineMsg carries a new line of output
type outputLineMsg struct {
	line string
}

// outputDoneMsg signals that output is complete
type outputDoneMsg struct {
	err error
}

// newProgramBox creates a new program box dialog
func newProgramBox(title, subtitle, command string) *programBoxModel {
	// Title is parsed by RenderDialog when View() is called.
	// Subtitle/Command is parsed in View().

	m := &programBoxModel{
		title:    title,
		subtitle: subtitle,
		command:  command,
		viewport: viewport.New(0, 0),
		lines:    []string{},
	}

	// Initialize viewport style to match dialog background (fixes black scrollbar)
	styles := GetStyles()
	m.viewport.Style = styles.Dialog.Copy().Padding(0, 0)
	// Use theme-defined console colors to properly display ANSI colors from command output
	m.viewport.Style = m.viewport.Style.Copy().
		Background(styles.Console.GetBackground()).
		Foreground(styles.Console.GetForeground())

	return m
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
	return nil
}

func (m *programBoxModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

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

		// Width calculation: screen - margins(4) - shadow(2) - borders(2 outer + 2 inner)
		m.viewport.Width = m.width - 4 - shadowWidth - 4
		if m.viewport.Width < 20 {
			m.viewport.Width = 20
		}

		// Height calculation: screen - margins(4) - shadow(1) - borders(2 outer + 2 inner) - other components(cmd line + button row)
		m.viewport.Height = m.height - 4 - shadowHeight - 4 - commandHeight - 3
		if m.viewport.Height < 5 {
			m.viewport.Height = 5
		}

		return m, nil

	case outputLineMsg:
		m.lines = append(m.lines, msg.line)
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
		return m, nil

	case tea.KeyMsg:
		switch {
		case key.Matches(msg, Keys.Esc):
			if m.done {
				return m, tea.Quit
			}

		case key.Matches(msg, Keys.ForceQuit):
			return m, tea.Quit

		case key.Matches(msg, Keys.Enter):
			if m.done {
				return m, tea.Quit
			}
		}

	case tea.MouseMsg:
		if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
			// Check if OK button was clicked (auto-generated zone ID: "Button.OK")
			if m.done {
				if zoneInfo := zone.Get("Button.OK"); zoneInfo != nil {
					if zoneInfo.InBounds(msg) {
						return m, tea.Quit
					}
				}
			}
		}
	}

	// Update viewport for scrolling
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

func (m *programBoxModel) View() string {
	if m.width == 0 {
		return ""
	}

	styles := GetStyles()

	// Calculate scroll percentage
	scrollPercent := m.viewport.ScrollPercent()

	// Add scroll indicator at bottom of viewport content
	scrollIndicator := styles.TagKey.Copy().
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
	viewportStyle := styles.Console.Copy().
		Padding(0, 0) // Remove side padding inside inner box for a tighter look
	viewportStyle = ApplyRoundedBorder(viewportStyle, styles.LineCharacters)

	// Apply scroll indicator manually to bottom border
	// We disable the bottom border initially to let us construct it ourselves
	viewportStyle = viewportStyle.BorderBottom(false)

	borderedViewport := viewportStyle.Render(viewportContent)

	// Construct custom bottom border with label
	border := styles.Border
	width := m.viewport.Width + 2 // Add 2 for left/right padding of viewportStyle
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
	// viewport.Width + border (2) = viewport.Width + 2
	contentWidth := m.viewport.Width + 2

	// Build command display using theme semantic tags
	var commandDisplay string
	if m.command != "" {
		// Use RenderThemeText for robust parsing of embedded tags/colors
		// We use the console style as base, but DO NOT force the background color onto the whole bar
		// This allows the user to have unstyled spaces or mixed colors.
		// Use styles.Dialog as base so unstyled text matches the dialog background
		base := styles.Dialog.Copy()
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

	// Width calculation: screen - margins(4) - shadow(2) - borders(2 outer + 2 inner)
	m.viewport.Width = m.width - 4 - shadowWidth - 4
	if m.viewport.Width < 20 {
		m.viewport.Width = 20
	}

	// Height calculation: screen - margins(4) - shadow(1) - borders(2 outer + 2 inner) - other components(cmd line + button row)
	m.viewport.Height = m.height - 4 - shadowHeight - 4 - commandHeight - 3
	if m.viewport.Height < 5 {
		m.viewport.Height = 5
	}
}

// getHelpText returns the dynamic help text based on the current state
func (m *programBoxModel) getHelpText() string {
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

// programBoxWithBackdrop wraps a program box dialog with backdrop using overlay
type programBoxWithBackdrop struct {
	backdrop BackdropModel
	dialog   *programBoxModel
}

func (m programBoxWithBackdrop) Init() tea.Cmd {
	return tea.Batch(m.backdrop.Init(), m.dialog.Init())
}

func (m programBoxWithBackdrop) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	// Update backdrop
	backdropModel, cmd := m.backdrop.Update(msg)
	m.backdrop = backdropModel.(BackdropModel)
	cmds = append(cmds, cmd)

	// Update dialog
	dialogModel, cmd := m.dialog.Update(msg)
	m.dialog = dialogModel.(*programBoxModel)
	cmds = append(cmds, cmd)

	// Update backdrop helpText based on dialog state
	m.backdrop.SetHelpText(m.dialog.getHelpText())

	return m, tea.Batch(cmds...)
}

func (m programBoxWithBackdrop) View() string {
	// Get backdrop and dialog views
	backdropView := m.backdrop.View()
	dialogView := m.dialog.View()

	// If dialog isn't ready yet, just show backdrop
	if dialogView == "" {
		return backdropView
	}

	// Position dialog using offsets from top-left corner
	// Offset using parameters (2, 2)
	output := overlay.Composite(
		dialogView,   // foreground (dialog content)
		backdropView, // background (backdrop base)
		0,            // xPos: top-left
		0,            // yPos: top-left
		2,            // xOffset: 2 chars from left
		2,            // yOffset: 2 lines down
	)

	// Scan zones at root level for mouse support
	return zone.Scan(output)
}

// RunProgramBox displays a program box dialog that shows command output
func RunProgramBox(ctx context.Context, title, subtitle string, task func(context.Context, io.Writer) error) error {
	// Automatically append reset tags to title/subtitle if missing
	if title != "" && !strings.HasSuffix(title, "{{|-|}}") {
		title += "{{|-|}}"
	}
	if subtitle != "" && !strings.HasSuffix(subtitle, "{{|-|}}") {
		subtitle += "{{|-|}}"
	}

	// Initialize global zone manager for mouse support (safe to call multiple times)
	zone.NewGlobal()

	// Initialize TUI if not already done
	cfg := config.LoadAppConfig()
	logger.Debug(ctx, "RunProgramBox config: Shadow=%v, ShadowLevel=%d, LineCharacters=%v", cfg.UI.Shadow, cfg.UI.ShadowLevel, cfg.UI.LineCharacters)
	currentConfig = cfg // Set global config so styles like AddShadow work correctly
	if err := theme.Load(cfg.UI.Theme); err == nil {
		InitStyles(cfg)
	}

	// Use subtitle as command display (can be empty)
	dialogModel := newProgramBox(title, subtitle, subtitle)

	// Create wrapper with backdrop
	initialHelpText := "Running... | Press Ctrl+C to cancel | PgUp/PgDn to scroll"
	model := programBoxWithBackdrop{
		backdrop: NewBackdropModel(initialHelpText),
		dialog:   dialogModel,
	}

	// Create a pipe for output
	reader, writer := io.Pipe()

	// Create Bubble Tea program FIRST (before redirecting stdout/stderr)
	// so it can use the real terminal
	p := tea.NewProgram(model, tea.WithMouseAllMotion())

	// Run the task in a goroutine
	errChan := make(chan error, 1)
	go func() {
		defer writer.Close()

		// Check if stdout/stderr are already redirected (not terminals)
		// If they are, don't redirect again - they're likely already going to a dialog
		stdoutStat, _ := os.Stdout.Stat()
		stderrStat, _ := os.Stderr.Stat()
		stdoutIsTerminal := (stdoutStat.Mode() & os.ModeCharDevice) != 0
		stderrIsTerminal := (stderrStat.Mode() & os.ModeCharDevice) != 0

		// Only redirect if stdout/stderr are actual terminals
		if stdoutIsTerminal && stderrIsTerminal {
			// Save original stdout/stderr
			oldStdout := os.Stdout
			oldStderr := os.Stderr

			// Create pipes for stdout/stderr redirection
			r, w, _ := os.Pipe()

			// Redirect stdout/stderr to our pipe
			os.Stdout = w
			os.Stderr = w

			// Copy from the pipe to our writer in a goroutine
			copyDone := make(chan struct{})
			go func() {
				io.Copy(writer, r)
				close(copyDone)
			}()

			// Run the task
			err := task(ctx, writer)

			// Restore original stdout/stderr
			w.Close()
			os.Stdout = oldStdout
			os.Stderr = oldStderr

			// Wait for copy to finish
			<-copyDone

			errChan <- err
		} else {
			// stdout/stderr already redirected, just run the task
			errChan <- task(ctx, writer)
		}
	}()

	// Start streaming output
	go func() {
		scanner := bufio.NewScanner(reader)
		for scanner.Scan() {
			line := scanner.Text()
			p.Send(outputLineMsg{line: line})
		}

		// Signal completion
		err := <-errChan
		p.Send(outputDoneMsg{err: err})
	}()

	_, err := p.Run()
	// Reset terminal colors on exit to prevent "bleeding" into the shell prompt
	fmt.Print("\x1b[0m\n")
	return err
}
