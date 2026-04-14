package tui

import (
	"bufio"
	"context"
	"io"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/progress"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
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
	lines    []string // Currently rendered/wrapped lines
	rawLines []string // Themed but unwrapped lines for re-wrapping on resize
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
	// SuccessMsg is forwarded as CloseDialogMsg.Result when the task completes without error
	// and the user dismisses the dialog.  shouldForwardResult routes it to the active screen.
	SuccessMsg tea.Msg
	task       func(context.Context, io.Writer) error
	focused  bool
	ctx      context.Context

	// Overlay prompts (for blocking prompts during task)
	subDialog     tea.Model
	subDialogChan any

	// Progress tracking
	Tasks    []Task
	Percent  float64
	progress progress.Model

	// Unified layout (deterministic sizing)
	layout DialogLayout
	id     string

	// Scrollbar component
	Scroll Scrollbar
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
		id:       "programbox_dialog",
		title:    title,
		subtitle: subtitle,
		command:  command,
		viewport: viewport.New(),
		lines:    []string{},
		Tasks:    []Task{},
		focused:  true,
		ctx:      context.Background(),
		Scroll:   Scrollbar{ID: "programbox_dialog"},
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

// SetContext sets a cancelable context to be used for the task
func (m *ProgramBoxModel) SetContext(ctx context.Context) {
	if ctx != nil {
		m.ctx = ctx
	}
}

// SetFocused sets the focus state
func (m *ProgramBoxModel) SetFocused(focused bool) {
	m.focused = focused
}

// IsScrollbarDragging reports whether the viewport scrollbar thumb is being dragged.
func (m *ProgramBoxModel) IsScrollbarDragging() bool {
	return m.Scroll.Drag.Dragging
}



func (m *ProgramBoxModel) Init() tea.Cmd {
	// If a task function was set (dialog mode), start it now
	if m.task != nil {
		task := m.task
		m.task = nil // Prevent double-start

		return func() tea.Msg {
			reader, writer := io.Pipe()

			errChan := make(chan error, 1)

			// Start reading output in a goroutine — sends lines to the viewport
			go func() {
				ticker := time.NewTicker(100 * time.Millisecond)
				defer ticker.Stop()

				var lines []string
				lineChan := make(chan string, 100)

				// Separate goroutine to scan and feed the aggregator channel
				go func() {
					scanner := bufio.NewScanner(reader)
					for scanner.Scan() {
						lineChan <- scanner.Text()
					}
					close(lineChan)
				}()

				for {
					select {
					case line, ok := <-lineChan:
						if !ok {
							// Flush remaining lines before exiting
							if len(lines) > 0 && program != nil {
								program.Send(outputLinesMsg{lines: lines})
							}
							return
						}
						lines = append(lines, line)
						// Auto-flush if buffer is large to keep it responsive
						if len(lines) >= 50 && program != nil {
							program.Send(outputLinesMsg{lines: lines})
							lines = nil
						}
					case <-ticker.C:
						if len(lines) > 0 && program != nil {
							program.Send(outputLinesMsg{lines: lines})
							lines = nil
						}
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

func (m *ProgramBoxModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// 1. Centralized scrollbar processing (Throttling, Clicks, Dragging)
	if newOff, cmd, changed := m.Scroll.Update(msg, m.viewport.YOffset(), m.viewport.TotalLineCount(), m.viewport.Height()); changed {
		m.viewport.SetYOffset(newOff)
		return m, cmd
	}

	// 2. Sub-dialog result handling (completes blocking prompts)
	if resultMsg, ok := msg.(SubDialogResultMsg); ok {
		if m.subDialog != nil {
			m.satisfySubDialogChan(resultMsg.Result)
			m.subDialog = nil
			m.subDialogChan = nil
		}
		return m, nil
	}

	var subCmd tea.Cmd
	// 3. Sub-dialog delegation
	if m.subDialog != nil {
		// Special case: WindowSizeMsg goes to both to ensure ProgramBox stays sized correctly
		if wsm, ok := msg.(tea.WindowSizeMsg); ok {
			m.width = wsm.Width
			m.height = wsm.Height
			m.subDialog, subCmd = m.subDialog.Update(msg)
		} else {
			// Exclusive delegation for interaction while sub-dialog is active
			m.subDialog, subCmd = m.subDialog.Update(msg)
			return m, subCmd
		}
	}

	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.calculateLayout()
		return m, subCmd

	case SubDialogMsg:
		m.subDialog = msg.Model
		m.subDialogChan = msg.Chan
		if s, ok := m.subDialog.(interface{ SetSize(int, int) }); ok {
			s.SetSize(m.width, m.height)
		}
		return m, nil

	case CloseDialogMsg:
		if m.subDialog != nil {
			m.satisfySubDialogChan(msg.Result)
			m.subDialog = nil
			m.subDialogChan = nil
			return m, nil
		}

	case outputLinesMsg:
		styles := GetStyles()
		for _, line := range msg.lines {
			rendered := RenderConsoleText(line, styles.Console)
			m.rawLines = append(m.rawLines, rendered)
		}
		if m.viewport.Width() > 0 {
			content := lipgloss.NewStyle().
				Width(m.viewport.Width()).
				Render(strings.Join(m.rawLines, "\n"))
			m.viewport.SetContent(content)
		} else {
			m.viewport.SetContent(strings.Join(m.rawLines, "\n"))
		}
		m.viewport.GotoBottom()
		if !m.done {
			return m, nil
		}

	case outputDoneMsg:
		m.done = true
		m.err = msg.err
		m.SetSize(m.width, m.height)
		if m.viewport.Width() > 0 {
			content := lipgloss.NewStyle().
				Width(m.viewport.Width()).
				Render(strings.Join(m.rawLines, "\n"))
			m.viewport.SetContent(content)
		}
		m.viewport.GotoBottom()
		if m.AutoExit && m.err == nil {
			result := tea.Msg(true)
			if m.SuccessMsg != nil {
				result = m.SuccessMsg
			}
			return m, func() tea.Msg { return CloseDialogMsg{Result: result} }
		}
		return m, nil

	case tea.KeyPressMsg:
		closeDialog := func() tea.Msg { return CloseDialogMsg{} }
		switch {
		case key.Matches(msg, Keys.Esc):
			if m.done {
				if m.err == nil && m.SuccessMsg != nil {
					return m, func() tea.Msg { return CloseDialogMsg{Result: m.SuccessMsg} }
				}
				return m, closeDialog
			}
		case key.Matches(msg, Keys.ForceQuit):
			return m, closeDialog
		case key.Matches(msg, Keys.Enter), msg.String() == "o", msg.String() == "O", key.Matches(msg, Keys.Space):
			if m.done {
				result := tea.Msg(true)
				if m.err == nil && m.SuccessMsg != nil {
					result = m.SuccessMsg
				}
				return m, func() tea.Msg { return CloseDialogMsg{Result: result} }
			}
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

	case tea.MouseWheelMsg, tea.MouseMotionMsg, tea.MouseClickMsg, tea.MouseReleaseMsg:
		// Logic handled in centralized m.Scroll.Update at top of function.
		return m, nil

	case LayerHitMsg:
		if m.done && ButtonIDMatches(msg.ID, "OK") && msg.Button == tea.MouseLeft {
			result := tea.Msg(true)
			if m.err == nil && m.SuccessMsg != nil {
				result = m.SuccessMsg
			}
			return m, func() tea.Msg { return CloseDialogMsg{Result: result} }
		}

	case UpdateTaskMsg:
		m.UpdateTaskStatus(msg.Label, msg.Status, msg.ActiveApp)
		return m, nil

	case UpdatePercentMsg:
		m.SetPercent(msg.Percent)
		m.calculateLayout()
		return m, nil
	}

	if _, ok := msg.(ToggleFocusedMsg); ok {
		if m.done {
			return m, func() tea.Msg { return CloseDialogMsg{} }
		}
		return m, nil
	}

	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}




func (m *ProgramBoxModel) satisfySubDialogChan(result any) {
	if m.subDialogChan == nil {
		return
	}
	// Handle result based on channel type
	switch ch := m.subDialogChan.(type) {
	case chan bool:
		if r, ok := result.(bool); ok {
			ch <- r
		} else {
			ch <- false // Default/cancel
		}
	case chan promptResultMsg:
		if r, ok := result.(promptResultMsg); ok {
			ch <- r
		} else {
			ch <- promptResultMsg{confirmed: false}
		}
	}
}
