package tui

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"time"

	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/tui/components/streamvp"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/progress"
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
	sv           streamvp.Model
	spinnerFrame int       // title-bar spinner frame (advanced by global tick)
	lastSpinner  time.Time // when the title-bar spinner was last advanced
	done         bool
	err          error
	width        int
	height       int

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
	focused    bool
	ctx        context.Context

	// Overlay prompts (for blocking prompts during task)
	subDialog     tea.Model
	subDialogChan any

	// Progress tracking
	Tasks    []Task
	Percent  float64
	progress progress.Model

	// Unified layout (deterministic sizing)
	layout          DialogLayout
	id              string
	dialogType      DialogType
	TitleBarFocus

	// headerLineCount tracks lines present before the first replaceOutputMsg (-1 = not yet set).
	headerLineCount int

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
	styles := GetStyles()
	sv := streamvp.New()
	sv.SetStyle(lipgloss.NewStyle().
		Background(styles.Console.GetBackground()).
		Foreground(styles.Console.GetForeground()))

	m := &ProgramBoxModel{
		id:              "programbox_dialog",
		title:           title,
		subtitle:        subtitle,
		command:         command,
		sv:              sv,
		Tasks:           []Task{},
		focused:         true,
		ctx:             context.Background(),
		Scroll:          Scrollbar{ID: "programbox_dialog"},
		headerLineCount: -1,
	}

	// Initialize progress bar with default options
	m.progress = progress.New()

	return m
}

// pbRenderFn returns the render function used by streamvp for the program box.
func pbRenderFn() func(string) string {
	styles := GetStyles()
	return func(raw string) string {
		return RenderConsoleText(raw, styles.Console)
	}
}

// AddTask adds a task category to the progress header
// titleBarHitRegions returns hit regions for the title bar widgets,
// using m.layout.LargeTitleBar to determine the correct Y offset.
func (m *ProgramBoxModel) titleBarHitRegions(offsetX, offsetY, contentWidth, baseZ int) []HitRegion {
	ctx := GetActiveContext()
	activeW := m.ActiveWidgets()
	widgetStr := BuildInactiveTitleWidgetsFor(activeW, ctx)
	widgetWidth := lipgloss.Width(GetPlainText(widgetStr))
	if widgetWidth == 0 {
		return nil
	}
	dialogWidth := contentWidth + 2
	widgetsStartX := offsetX + dialogWidth - 1 - 1 - widgetWidth
	return TitleBarWidgetRegions(m.id, activeW, widgetsStartX, TitleBarWidgetY(offsetY, m.layout.LargeTitleBar), baseZ)
}

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

// WithDialogType sets the dialog type for title area styling (e.g. DialogTypeSuccess).
func (m *ProgramBoxModel) WithDialogType(t DialogType) *ProgramBoxModel {
	m.dialogType = t
	return m
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

func (m *ProgramBoxModel) IsFocused() bool { return m.focused }

// IsScrollbarDragging reports whether the viewport scrollbar thumb is being dragged.
func (m *ProgramBoxModel) IsScrollbarDragging() bool {
	return m.Scroll.Drag.Dragging
}

func (m *ProgramBoxModel) Init() tea.Cmd {
	// If a task function was set (dialog mode), start it now
	if m.task != nil {
		task := m.task
		m.task = nil // Prevent double-start

		m.sv.CommandRunning = true
		lockID := fmt.Sprintf("programbox-%p", m)
		return tea.Batch(
			func() tea.Msg { return ConsoleLockMsg{ID: lockID, Locked: true} },
			func() tea.Msg {
				reader, writer := io.Pipe()

				errChan := make(chan error, 1)

				// Start reading output in a goroutine — sends each line immediately to the viewport.
				go func() {
					scanner := bufio.NewScanner(reader)
					for scanner.Scan() {
						if program != nil {
							program.Send(outputLinesMsg{lines: []string{scanner.Text()}})
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

				go func() {
					err := <-errChan
					if program != nil {
						program.Send(outputDoneMsg{err: err})
					}
				}()

				return nil
			},
		)
	}
	return nil
}

func (m *ProgramBoxModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {

	if m.HandleWidgetClearPress(msg) {
		return m, nil
	}

	// 1. Centralized scrollbar processing (Throttling, Clicks, Dragging)
	if newOff, cmd, changed := m.Scroll.Update(msg, m.sv.YOffset(), m.sv.TotalLineCount(), m.sv.Height()); changed {
		m.sv.SetYOffset(newOff)
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
		m.sv.CommandRunning = true
		m.sv.AppendLines(msg.lines, pbRenderFn())
		if !m.done {
			return m, nil
		}

	case replaceOutputMsg:
		setActiveOutputWidth(m.sv.Width())
		m.sv.CommandRunning = false
		if m.headerLineCount < 0 {
			m.headerLineCount = m.sv.TotalLineCount()
		}
		m.sv.ReplaceTailLines(m.headerLineCount, msg.lines, pbRenderFn())
		if !m.done {
			return m, nil
		}

	case outputDoneMsg:
		m.done = true
		m.sv.CommandRunning = false
		m.sv.ClearSpinner()
		lockID := fmt.Sprintf("programbox-%p", m)
		m.err = msg.err
		m.SetSize(m.width, m.height)
		unlockCmd := func() tea.Msg { return ConsoleLockMsg{ID: lockID, Locked: false} }
		if m.AutoExit && m.err == nil {
			result := tea.Msg(true)
			if m.SuccessMsg != nil {
				result = m.SuccessMsg
			}
			return m, tea.Batch(unlockCmd, func() tea.Msg { return CloseDialogMsg{Result: result} })
		}
		return m, unlockCmd

	case tea.KeyPressMsg:
		closeDialog := func() tea.Msg { return CloseDialogMsg{} }
		if handled, cmd := m.HandleTitleBarKey(msg, func() tea.Msg { return CloseDialogMsg{} }); handled {
			return m, cmd
		}
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
			m.sv.GotoTop()
			return m, nil
		case key.Matches(msg, Keys.End):
			m.sv.GotoBottom()
			return m, nil
		}

	case tea.MouseWheelMsg, tea.MouseMotionMsg, tea.MouseClickMsg, tea.MouseReleaseMsg:
		// Logic handled in centralized m.Scroll.Update at top of function.
		return m, nil

	case LayerHitMsg:
		if handled, cmd := m.HandleTitleBarHit(msg, func() tea.Msg { return CloseDialogMsg{} }); handled {
			return m, cmd
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
		m.calculateLayout()
		return m, nil
	}

	if _, ok := msg.(ToggleFocusedMsg); ok {
		if m.done {
			return m, func() tea.Msg { return CloseDialogMsg{} }
		}
		return m, nil
	}

	// Only pass messages the viewport actually needs — not mouse or semantic messages
	// which can trigger focus side-effects (e.g. clearing header focus).
	switch msg.(type) {
	case LayerHitMsg, LayerWheelMsg,
		tea.MouseClickMsg, tea.MouseWheelMsg, tea.MouseMotionMsg, tea.MouseReleaseMsg:
		return m, nil
	}
	cmd = m.sv.ViewUpdate(msg)
	return m, cmd
}

// AdvanceSpinners advances the title-bar spinner and the inline streamvp spinner
// if their interval has elapsed. Returns true if anything changed (caller should
// trigger a re-render). Called by the global tick in AppModel.Update.
func (m *ProgramBoxModel) AdvanceSpinners(now time.Time) bool {
	if m.done || !console.SpinnerEnabled {
		return false
	}
	changed := false
	fps := time.Duration(console.SpinnerSpeed) * time.Millisecond
	if fps > 0 && now.Sub(m.lastSpinner) >= fps {
		m.lastSpinner = now
		m.spinnerFrame++
		changed = true
	}
	if m.sv.AdvanceSpinner(now) {
		changed = true
	}
	return changed
}

// currentSpinnerIndicators returns the left and right spinner frame characters for the title bar,
// or "" when the spinner is disabled or the task is complete.
func (m *ProgramBoxModel) currentSpinnerIndicators() (left, right string) {
	if m.done || !console.SpinnerEnabled {
		return "", ""
	}
	ctx := GetActiveContext()
	return console.TitleSpinnerFrames(m.spinnerFrame, ctx.LineCharacters)
}

func (m *ProgramBoxModel) satisfySubDialogChan(result any) {
	if m.subDialogChan == nil {
		return
	}
	// Handle result based on channel type
	switch ch := m.subDialogChan.(type) {
	case chan int:
		if r, ok := result.(int); ok {
			ch <- r
		} else {
			ch <- -1
		}
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
