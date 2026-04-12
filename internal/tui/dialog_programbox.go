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

	// Scrollbar drag state
	sbInfo    ScrollbarInfo
	sbAbsTopY int
	sbDrag    ScrollbarDragState
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
	return m.sbDrag.Dragging
}

// scrollbarDragTo scrolls the viewport so the thumb at mouseY maps to the correct position.
// Returns true if the scroll position changed (caller should invalidate render cache).
func (m *ProgramBoxModel) scrollbarDragTo(mouseY int) bool {
	info := m.sbInfo
	if !info.Needed {
		return false
	}
	total := m.viewport.TotalLineCount()
	visible := m.viewport.VisibleLineCount()
	if total <= visible {
		return false
	}
	maxOff := total - visible
	newOff, _ := m.sbDrag.ScrollOffset(mouseY, m.sbAbsTopY, maxOff, info)
	if newOff == m.viewport.YOffset() {
		return false
	}
	m.viewport.GotoTop()
	m.viewport.ScrollDown(newOff)
	return true
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
	// Handle sub-dialog result specifically (it signals closing of the sub-dialog)
	if resultMsg, ok := msg.(SubDialogResultMsg); ok {
		if m.subDialogChan != nil {
			switch ch := m.subDialogChan.(type) {
			case chan bool:
				if r, ok := resultMsg.Result.(bool); ok {
					ch <- r
				} else {
					ch <- false
				}
				close(ch)
			case chan promptResultMsg:
				if r, ok := resultMsg.Result.(promptResultMsg); ok {
					ch <- r
				} else {
					ch <- promptResultMsg{confirmed: false}
				}
				close(ch)
			case chan int:
				if r, ok := resultMsg.Result.(int); ok {
					ch <- r
				} else {
					ch <- -1
				}
				close(ch)
			}
			m.subDialogChan = nil
		}
		m.subDialog = nil
		return m, nil
	}

	var subCmd tea.Cmd
	// Handle sub-dialog updates if active
	if m.subDialog != nil {
		// Special case: WindowSizeMsg goes to both to ensure ProgramBox stays sized correctly
		if wsm, ok := msg.(tea.WindowSizeMsg); ok {
			m.width = wsm.Width
			m.height = wsm.Height
			m.subDialog, subCmd = m.subDialog.Update(msg)
			// We fall through to let ProgramBox also handle the resize (viewport mainly)
		} else {
			// Exclusive delegations for interaction
			m.subDialog, subCmd = m.subDialog.Update(msg)
			return m, subCmd
		}
	}

	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		// AppModel sends a contentSizeMsg right after SetSize. Handle it here so we
		// re-run calculateLayout() (which re-sets the viewport height) instead of
		// forwarding to m.viewport.Update which would resize the viewport to the full
		// content-area height and make the dialog too tall.
		m.width = msg.Width
		m.height = msg.Height
		m.calculateLayout()
		return m, subCmd

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
			m.satisfySubDialogChan(msg.Result)
			m.subDialog = nil
			m.subDialogChan = nil
			return m, nil
		}

	case SubDialogResultMsg:
		if m.subDialog != nil {
			m.satisfySubDialogChan(msg.Result)
			m.subDialog = nil
			m.subDialogChan = nil
		}
		return m, nil

	case outputLinesMsg:
		// Convert semantic theme tags to ANSI colors before displaying
		styles := GetStyles()
		for _, line := range msg.lines {
			rendered := RenderConsoleText(line, styles.Console)
			m.rawLines = append(m.rawLines, rendered)
		}

		// Render with wrapping to viewport width
		if m.viewport.Width() > 0 {
			content := lipgloss.NewStyle().
				Width(m.viewport.Width()).
				Render(strings.Join(m.rawLines, "\n"))
			m.viewport.SetContent(content)
		} else {
			m.viewport.SetContent(strings.Join(m.rawLines, "\n"))
		}
		m.viewport.GotoBottom()

		// Continue reading if not done
		if !m.done {
			return m, nil
		}

	case outputDoneMsg:
		m.done = true
		m.err = msg.err
		// Recalculate size now that done is true (shrinks viewport for OK button)
		m.SetSize(m.width, m.height)

		// Final content update with correct wrapping for final size
		if m.viewport.Width() > 0 {
			content := lipgloss.NewStyle().
				Width(m.viewport.Width()).
				Render(strings.Join(m.rawLines, "\n"))
			m.viewport.SetContent(content)
		}
		m.viewport.GotoBottom()

		// If AutoExit is enabled and no error occurred, close immediately
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
		// Scrollbar thumb drag start (routed by model_mouse.go section B0).
		if msg.Button == tea.MouseLeft {
			m.sbDrag.StartDrag(msg.Y, m.sbAbsTopY, m.sbInfo)
		}
		return m, nil

	case tea.MouseMotionMsg:
		if m.sbDrag.Dragging {
			m.scrollbarDragTo(msg.Y) // viewport re-renders on next View(); no explicit cache to invalidate
		}
		return m, nil

	case tea.MouseReleaseMsg:
		if m.sbDrag.Dragging {
			m.sbDrag.StopDrag()
		}
		return m, nil

	case LayerHitMsg:
		// Scrollbar arrow/track clicks
		if strings.HasSuffix(msg.ID, ".sb.up") {
			if msg.Button != HoverButton {
				m.viewport.ScrollUp(1)
			}
			return m, nil
		}
		if strings.HasSuffix(msg.ID, ".sb.down") {
			if msg.Button != HoverButton {
				m.viewport.ScrollDown(1)
			}
			return m, nil
		}
		if strings.HasSuffix(msg.ID, ".sb.above") {
			if msg.Button != HoverButton {
				m.viewport.HalfPageUp()
			}
			return m, nil
		}
		if strings.HasSuffix(msg.ID, ".sb.below") {
			if msg.Button != HoverButton {
				m.viewport.HalfPageDown()
			}
			return m, nil
		}
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
		m.calculateLayout() // Re-budget viewport when progress bar appears/changes
		return m, nil
	}

	// Middle-click closes the dialog when the task is done
	if _, ok := msg.(ToggleFocusedMsg); ok {
		if m.done {
			return m, func() tea.Msg { return CloseDialogMsg{} }
		}
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
