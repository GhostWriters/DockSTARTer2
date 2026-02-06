package tui

import (
	"bufio"
	"context"
	"io"
	"os"
	"strings"

	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/theme"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// programBoxModel represents a dialog that displays streaming program output
type programBoxModel struct {
	title    string
	subtitle string
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
func newProgramBox(title, subtitle string) programBoxModel {
	vp := viewport.New(0, 0)
	// Use white-on-black background to properly display ANSI colors from command output
	vp.Style = lipgloss.NewStyle().
		Foreground(lipgloss.Color("255")). // White
		Background(lipgloss.Color("0"))    // Black

	return programBoxModel{
		title:    title,
		subtitle: subtitle,
		viewport: vp,
		lines:    []string{},
	}
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
func (m programBoxModel) streamReader(reader io.Reader) tea.Cmd {
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

func (m programBoxModel) Init() tea.Cmd {
	return nil
}

func (m programBoxModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		// Calculate viewport size
		// Account for borders, title, padding
		m.viewport.Width = m.width - 8
		m.viewport.Height = m.height - 10

		if m.viewport.Width < 0 {
			m.viewport.Width = 0
		}
		if m.viewport.Height < 0 {
			m.viewport.Height = 0
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
		case key.Matches(msg, key.NewBinding(key.WithKeys("q", "esc"))):
			if m.done {
				return m, tea.Quit
			}

		case key.Matches(msg, key.NewBinding(key.WithKeys("ctrl+c"))):
			return m, tea.Quit

		case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
			if m.done {
				return m, tea.Quit
			}
		}
	}

	// Update viewport for scrolling
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

func (m programBoxModel) View() string {
	if m.width == 0 {
		return "Loading..."
	}

	styles := GetStyles()

	// Build content
	content := m.viewport.View()

	// Status line
	statusStyle := lipgloss.NewStyle().
		Foreground(styles.HelpLine.GetForeground()).
		Italic(true)

	var status string
	if m.done {
		if m.err != nil {
			status = statusStyle.Render("Error: " + m.err.Error() + " | Press Enter or Esc to close")
		} else {
			status = statusStyle.Render("Complete | Press Enter or Esc to close")
		}
	} else {
		status = statusStyle.Render("Running... | Press Ctrl+C to cancel")
	}

	// Combine content and status
	fullContent := lipgloss.JoinVertical(lipgloss.Left,
		content,
		"",
		status,
	)

	// Wrap in dialog box
	dialogStyle := styles.Dialog.
		Padding(1, 2)
	dialogStyle = ApplyStraightBorder(dialogStyle, styles.LineCharacters)

	dialog := dialogStyle.Render(fullContent)

	// Add title
	titleText := m.title
	if m.subtitle != "" {
		titleText += " - " + m.subtitle
	}

	dialogWithTitle := renderBorderWithTitleStatic(titleText, dialog)

	// Center on screen
	return lipgloss.Place(
		m.width,
		m.height,
		lipgloss.Center,
		lipgloss.Center,
		dialogWithTitle,
	)
}

// RunProgramBox displays a program box dialog that shows command output
func RunProgramBox(ctx context.Context, title, subtitle string, task func(context.Context, io.Writer) error) error {
	// Initialize TUI if not already done
	cfg := config.LoadAppConfig()
	if err := theme.Load(cfg.Theme); err == nil {
		InitStyles(cfg)
	}

	model := newProgramBox(title, subtitle)

	// Create a pipe for output
	reader, writer := io.Pipe()

	// Create Bubble Tea program FIRST (before redirecting stdout/stderr)
	// so it can use the real terminal
	p := tea.NewProgram(model)

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
	return err
}
