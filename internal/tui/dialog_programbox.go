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
	task     func(context.Context, io.Writer) error
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
	sbInfo     ScrollbarInfo
	sbAbsTopY  int
	sbDragging bool
}

// SubDialogMsg signals a request to show a sub-dialog and blocks the task
type SubDialogMsg struct {
	Model tea.Model
	Chan  any
}

// SubDialogResultMsg signals the completion of a sub-dialog
type SubDialogResultMsg struct {
	Result any
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
	return m.sbDragging
}

// scrollbarDragTo scrolls the viewport so the thumb at mouseY maps to the correct position.
// Returns true if the scroll position changed (caller should invalidate render cache).
func (m *ProgramBoxModel) scrollbarDragTo(mouseY int) bool {
	info := m.sbInfo
	if !info.Needed {
		return false
	}
	trackH := info.Height - 2
	if trackH <= 0 {
		return false
	}
	total := m.viewport.TotalLineCount()
	visible := m.viewport.VisibleLineCount()
	if total <= visible {
		return false
	}
	maxOff := total - visible
	trackRelY := mouseY - (m.sbAbsTopY + 1)
	if trackRelY < 0 {
		trackRelY = 0
	}
	if trackRelY > trackH {
		trackRelY = trackH
	}
	newOff := trackRelY * maxOff / trackH
	if newOff == m.viewport.YOffset() {
		return false
	}
	m.viewport.GotoTop()
	m.viewport.ScrollDown(newOff)
	return true
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
	case tea.WindowSizeMsg:
		// AppModel sends a contentSizeMsg right after SetSize. Handle it here so we
		// re-run calculateLayout() (which re-sets the viewport height) instead of
		// forwarding to m.viewport.Update which would resize the viewport to the full
		// content-area height and make the dialog too tall.
		m.width = msg.Width
		m.height = msg.Height
		m.calculateLayout()
		return m, nil

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
				// Handle result based on channel type
				switch ch := m.subDialogChan.(type) {
				case chan bool:
					if r, ok := msg.Result.(bool); ok {
						ch <- r
					} else {
						ch <- false // Default/cancel
					}
				case chan promptResultMsg:
					if r, ok := msg.Result.(promptResultMsg); ok {
						ch <- r
					} else {
						ch <- promptResultMsg{confirmed: false}
					}
				}
			}
			m.subDialog = nil
			m.subDialogChan = nil
			return m, nil
		}

	case outputLineMsg:
		// Convert semantic theme tags to ANSI colors before displaying
		styles := GetStyles()
		rendered := RenderThemeText(msg.line, styles.Console)
		m.rawLines = append(m.rawLines, rendered)

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
				return m, func() tea.Msg { return CloseDialogMsg{Result: true} }
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
			m.sbDragging = true
		}
		return m, nil

	case tea.MouseMotionMsg:
		if m.sbDragging {
			m.scrollbarDragTo(msg.Y) // viewport re-renders on next View(); no explicit cache to invalidate
		}
		return m, nil

	case tea.MouseReleaseMsg:
		if m.sbDragging {
			m.sbDragging = false
		}
		return m, nil

	case LayerHitMsg:
		// Scrollbar arrow/track clicks
		if strings.HasSuffix(msg.ID, ".sb.up") {
			m.viewport.ScrollUp(1)
			return m, nil
		}
		if strings.HasSuffix(msg.ID, ".sb.down") {
			m.viewport.ScrollDown(1)
			return m, nil
		}
		if strings.HasSuffix(msg.ID, ".sb.above") {
			m.viewport.HalfPageUp()
			return m, nil
		}
		if strings.HasSuffix(msg.ID, ".sb.below") {
			m.viewport.HalfPageDown()
			return m, nil
		}
		if m.done && buttonIDMatches(msg.ID, "OK") {
			return m, func() tea.Msg { return CloseDialogMsg{Result: true} }
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
