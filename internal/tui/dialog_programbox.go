package tui

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"time"

	"DockSTARTer2/internal/displayengine"
	"DockSTARTer2/internal/tui/components/streamvp"

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
	title        string
	command      string // Command being executed (displayed above output)
	sv           streamvp.Model
	titleSpinner displayengine.TitleSpinner // title-bar spinner (advanced by global tick)
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
	// SuccessMsg is forwarded as displayengine.CloseDialogMsg.Result when the task completes without error
	// and the user dismisses the dialog.  shouldForwardResult routes it to the active screen.
	SuccessMsg tea.Msg
	task       func(context.Context, io.Writer) error
	focused    bool
	ctx        context.Context

	// sendFunc, if set, delivers a tea.Msg to the owning session's own
	// Program (see AppModel.Send) instead of the process-wide global program
	// var -- set via SetSendFunc when this dialog is shown (model_update.go),
	// since ProgramBoxModel doesn't otherwise know which session owns it.
	sendFunc func(tea.Msg)

	// choiceFunc, if set, is a session-scoped equivalent of the package-level
	// PromptChoice, for the same reason as sendFunc.
	choiceFunc func(title, question string, choices ...string) int

	// displayengine.Overlay prompts (for blocking prompts during task)
	subDialog     tea.Model
	subDialogChan any

	// Progress tracking
	Tasks    []Task
	Percent  float64
	progress progress.Model

	// headerLineCount tracks lines present before the first displayengine.ReplaceOutputMsg (-1 = not yet set).
	headerLineCount int

	// displayengine.Scrollbar component
	Scroll displayengine.Scrollbar

	// outer is the outer container displayengine.MenuModel (title, buttons) owning the
	// header/command/viewport content sections, following the pattern used
	// by every other migrated dialog. Rendering/hit-regions/sizing delegate
	// to it; sub-dialog overlay compositing and task-goroutine orchestration
	// stay on this wrapper (see dialog_programbox_view.go / this file's
	// Update).
	outer           *displayengine.MenuModel
	subtitleSection *displayengine.MenuModel // subtitle, as a standard plain-text Content section
	commandSection  *displayengine.MenuModel // nil when command == ""
	viewportSection *displayengine.MenuModel
	headerSection   *displayengine.MenuModel // Tasks/Percent only -- subtitle moved to subtitleSection
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

// SetProgramBoxHeaderMsg updates a running ProgramBoxModel's subtitle and/or
// command-line display -- e.g. once a choice-dependent command (like
// Stop/Down) is actually known, having started the dialog without one. Sent
// via SetProgramBoxHeader from the task goroutine.
type SetProgramBoxHeaderMsg struct {
	Subtitle string
	Command  string
}

// newProgramBox creates a new program box dialog (internal use)
func newProgramBox(title, subtitle, command string) *ProgramBoxModel {
	styles := displayengine.GetStyles()
	sv := streamvp.New()
	sv.SetStyle(lipgloss.NewStyle().
		Background(styles.Console.GetBackground()).
		Foreground(styles.Console.GetForeground()))

	m := &ProgramBoxModel{
		title:           title,
		command:         command,
		sv:              sv,
		Tasks:           []Task{},
		focused:         true,
		ctx:             context.Background(),
		Scroll:          displayengine.Scrollbar{ID: "programbox_viewport"},
		headerLineCount: -1,
		// Every real caller ends up filling the full dialog content area
		// regardless of whether it calls SetMaximized itself (getDialogArea
		// hands ProgramBox the full content area either way, so the
		// non-maximized "centered at natural size" path was already a
		// no-op for callers that never explicitly set it) -- defaulting to
		// maximized removes the ambiguity around what "natural height"
		// should mean for a scrolling viewport section that has none.
		maximized: true,
	}
	m.titleSpinner.Start()

	// Initialize progress bar with default options
	m.progress = progress.New()

	outer := displayengine.NewMenuModel("programbox_dialog", title, "", nil)
	outer.SetMaximized(true)
	outer.SetIsDialog(true)
	outer.SetTitleSpinnerIndicator(m.currentSpinnerIndicators)
	// No SetButtons call here -- the OK button is only added once the task
	// completes (outputDoneMsg calls outer.SetButtons(m.okButtons())), so
	// there's never a moment where a button exists but shouldn't be shown.
	m.subtitleSection = displayengine.NewPlainTextSection("programbox_subtitle", subtitle)
	outer.AddContentSection(m.subtitleSection)
	m.headerSection = newProgramBoxHeaderSection("programbox_header", m)
	outer.AddContentSection(m.headerSection)
	m.commandSection = newProgramBoxCommandSection("programbox_command", m)
	outer.AddContentSection(m.commandSection)
	m.viewportSection = newStreamOutputSection("programbox_viewport", m)
	outer.AddContentSection(m.viewportSection)
	m.outer = outer

	return m
}

// okButtons returns the single OK button def, added to outer only once the
// task completes (see outputDoneMsg's handler). ZoneID "btn-select" matches
// the primary-action convention every other migrated dialog uses.
// displayengine.MenuModel's Esc/title-bar-[x] handling falls back to this single button
// when no btn-back/btn-cancel/btn-exit ZoneID is present, so Esc/[x]
// correctly close via this Action without needing a borrowed ZoneID.
func (m *ProgramBoxModel) okButtons() []displayengine.ButtonDef {
	return []displayengine.ButtonDef{
		{Label: "OK", ZoneID: "btn-select", Action: func() tea.Msg {
			result := tea.Msg(true)
			if m.err == nil && m.SuccessMsg != nil {
				result = m.SuccessMsg
			}
			return displayengine.CloseDialogMsg{Result: result}
		}, Help: "Dismiss and return."},
	}
}

// pbRenderFn returns the render function used by streamvp for the program box.
func pbRenderFn() func(string) string {
	styles := displayengine.GetStyles()
	return func(raw string) string {
		return displayengine.RenderConsoleText(raw, styles.Console)
	}
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
	if m.outer != nil {
		m.outer.SetMaximized(maximized)
	}
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

// WithDialogType sets the dialog type for title area styling (e.g. displayengine.DialogTypeSuccess).
func (m *ProgramBoxModel) WithDialogType(t displayengine.DialogType) *ProgramBoxModel {
	m.outer.SetDialogType(t)
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

// SetSendFunc sets this dialog's session-scoped Send callback (see sendFunc).
func (m *ProgramBoxModel) SetSendFunc(fn func(tea.Msg)) {
	m.sendFunc = fn
}

// SetChoiceFunc sets this dialog's session-scoped choice callback (see choiceFunc).
func (m *ProgramBoxModel) SetChoiceFunc(fn func(title, question string, choices ...string) int) {
	m.choiceFunc = fn
}

// Choice shows a blocking multi-choice sub-dialog, using this dialog's
// session-scoped callback if set (see choiceFunc), falling back to the
// package-level (global-program-based) PromptChoice otherwise.
func (m *ProgramBoxModel) Choice(title, question string, choices ...string) int {
	if m.choiceFunc != nil {
		return m.choiceFunc(title, question, choices...)
	}
	return PromptChoice(title, question, choices...)
}

// ReplaceOutput replaces this dialog's current viewport output with lines,
// using this dialog's session-scoped send callback if set (see sendFunc),
// falling back to the global Send otherwise. Suitable for
// console.WithReplaceOutputFunc, e.g. for compose's live progress lines.
func (m *ProgramBoxModel) ReplaceOutput(lines []string) {
	send := m.sendFunc
	if send == nil {
		send = Send
	}
	send(displayengine.ReplaceOutputMsg{Lines: lines})
}

// SetFocused sets the focus state
func (m *ProgramBoxModel) SetFocused(focused bool) {
	m.focused = focused
	m.outer.SetFocused(focused)
}

func (m *ProgramBoxModel) IsFocused() bool { return m.focused }

// IsScrollbarDragging reports whether the viewport scrollbar thumb is being dragged.
func (m *ProgramBoxModel) IsScrollbarDragging() bool {
	return m.outer.IsScrollbarDragging()
}

// FocusTitleBar, BlurTitleBar, TitleBarFocused implement displayengine.TitleBarFocusable
// by delegating to outer, which now owns the sole displayengine.TitleBarFocus state (see
// newProgramBox's SetTitleSpinnerIndicator wiring for how outer's title-bar
// spinner reflects this wrapper's task-running state).
func (m *ProgramBoxModel) FocusTitleBar()        { m.outer.FocusTitleBar() }
func (m *ProgramBoxModel) BlurTitleBar()         { m.outer.BlurTitleBar() }
func (m *ProgramBoxModel) TitleBarFocused() bool { return m.outer.TitleBarFocused() }

func (m *ProgramBoxModel) Init() tea.Cmd {
	// If a task function was set (dialog mode), start it now
	if m.task != nil {
		task := m.task
		m.task = nil // Prevent double-start

		m.sv.CommandRunning = true
		lockID := fmt.Sprintf("programbox-%p", m)
		return tea.Batch(
			func() tea.Msg { return displayengine.ConsoleLockMsg{ID: lockID, Locked: true} },
			func() tea.Msg {
				reader, writer := io.Pipe()

				errChan := make(chan error, 1)

				// Start reading output in a goroutine — sends each line immediately to the viewport.
				send := m.sendFunc
				if send == nil {
					send = Send
				}
				go func() {
					scanner := bufio.NewScanner(reader)
					for scanner.Scan() {
						send(outputLinesMsg{lines: []string{scanner.Text()}})
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
					send(outputDoneMsg{err: err})
				}()

				return nil
			},
		)
	}
	return nil
}

func (m *ProgramBoxModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Sub-dialog result handling (completes blocking prompts)
	if resultMsg, ok := msg.(SubDialogResultMsg); ok {
		if m.subDialog != nil {
			m.satisfySubDialogChan(resultMsg.Result)
			m.subDialog = nil
			m.subDialogChan = nil
		}
		return m, nil
	}

	var subCmd tea.Cmd
	// Sub-dialog delegation
	if m.subDialog != nil {
		switch wsm := msg.(type) {
		case tea.WindowSizeMsg:
			// Special case: WindowSizeMsg goes to both to ensure ProgramBox stays sized correctly
			m.width = wsm.Width
			m.height = wsm.Height
			m.outer.SetSize(wsm.Width, wsm.Height)
			m.subDialog, subCmd = m.subDialog.Update(msg)
		case outputLinesMsg, displayengine.ReplaceOutputMsg, UpdateTaskMsg, UpdatePercentMsg:
			// Streaming output and task/progress state must keep flowing into
			// the viewport/header even while a sub-dialog (e.g. a confirm
			// prompt raised mid-task) is up, or the viewport looks frozen
			// until an unrelated redraw catches it up. Fall through instead
			// of exclusively delegating. outputDoneMsg is deliberately NOT
			// included -- its task goroutine is blocked upstream on the
			// sub-dialog's own answer channel, so letting it through early
			// would show the OK button before the sub-dialog is answered.
		default:
			// Exclusive delegation for interaction while sub-dialog is active
			m.subDialog, subCmd = m.subDialog.Update(msg)
			return m, subCmd
		}
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.outer.SetSize(msg.Width, msg.Height)
		return m, subCmd

	case SubDialogMsg:
		m.subDialog = msg.Model
		m.subDialogChan = msg.Chan
		if s, ok := m.subDialog.(interface{ SetSize(int, int) }); ok {
			s.SetSize(m.width, m.height)
		}
		return m, nil

	case displayengine.CloseDialogMsg:
		if m.subDialog != nil {
			m.satisfySubDialogChan(msg.Result)
			m.subDialog = nil
			m.subDialogChan = nil
			return m, nil
		}

	case outputLinesMsg:
		m.sv.CommandRunning = true
		m.sv.AppendLines(msg.lines, pbRenderFn())
		// Both caches must be invalidated: the viewport section's own (keyed
		// on its own state, not m.sv's) AND outer's top-level cache -- outer's
		// ViewString() checks its OWN cache before ever reaching
		// viewWithSections()'s per-section loop, so leaving outer's cache
		// valid means the section-level invalidation below is never even
		// consulted; the whole dialog keeps showing its last-rendered string
		// until something else invalidates outer for an unrelated reason.
		m.viewportSection.InvalidateCache()
		m.outer.InvalidateCache()
		if !m.done {
			return m, nil
		}

	case displayengine.ReplaceOutputMsg:
		displayengine.SetActiveOutputWidth(m.sv.Width())
		m.sv.CommandRunning = false
		if m.headerLineCount < 0 {
			m.headerLineCount = m.sv.TotalLineCount()
		}
		m.sv.ReplaceTailLines(m.headerLineCount, msg.Lines, pbRenderFn())
		m.viewportSection.InvalidateCache()
		m.outer.InvalidateCache()
		if !m.done {
			return m, nil
		}

	case outputDoneMsg:
		m.done = true
		m.titleSpinner.Stop()
		m.sv.CommandRunning = false
		m.sv.ClearSpinner()
		lockID := fmt.Sprintf("programbox-%p", m)
		m.err = msg.err
		m.outer.SetButtons(m.okButtons())
		m.outer.SetSize(m.width, m.height)
		unlockCmd := func() tea.Msg { return displayengine.ConsoleLockMsg{ID: lockID, Locked: false} }
		if m.AutoExit && m.err == nil {
			result := tea.Msg(true)
			if m.SuccessMsg != nil {
				result = m.SuccessMsg
			}
			return m, tea.Batch(unlockCmd, func() tea.Msg { return displayengine.CloseDialogMsg{Result: result} })
		}
		return m, unlockCmd

	case UpdateTaskMsg:
		m.UpdateTaskStatus(msg.Label, msg.Status, msg.ActiveApp)
		m.headerSection.InvalidateCache()
		m.outer.InvalidateCache()
		return m, nil

	case UpdatePercentMsg:
		m.SetPercent(msg.Percent)
		m.headerSection.InvalidateCache()
		m.outer.InvalidateCache()
		return m, nil

	case SetProgramBoxHeaderMsg:
		if m.subtitleSection != nil {
			m.subtitleSection.SetPlainText(msg.Subtitle)
		}
		m.command = msg.Command
		// InvalidateCache alone only clears the cached rendered string --
		// row-height allocation is computed by calculateSectionLayout, which
		// only runs from SetSize. Without re-triggering it, the header/
		// command rows keep whatever height was allocated when first shown
		// (possibly blank/shorter), so new content ends up squeezed into a
		// stale size instead of the box resizing to fit it.
		m.outer.SetSize(m.outer.Width(), m.outer.Height())
		return m, nil
	}

	newOuter, cmd := m.outer.Update(msg)
	if o, ok := newOuter.(*displayengine.MenuModel); ok {
		m.outer = o
	}
	return m, cmd
}

// AdvanceSpinners advances the title-bar spinner and the inline streamvp spinner
// if their interval has elapsed. Returns true if anything changed (caller should
// trigger a re-render). Called by the global tick in AppModel.Update.
func (m *ProgramBoxModel) AdvanceSpinners(now time.Time) bool {
	if m.done {
		return false
	}
	changed := m.titleSpinner.AdvanceSpinner(now)
	if m.sv.AdvanceSpinner(now) {
		changed = true
	}
	if changed {
		// Same stale-cache class as outputLinesMsg/displayengine.ReplaceOutputMsg: both the
		// viewport section's own cache and outer's top-level cache must be
		// invalidated, or the advanced spinner frame never actually reaches
		// the screen despite AdvanceSpinners correctly mutating m.sv/
		// m.titleSpinner every tick.
		m.viewportSection.InvalidateCache()
		m.outer.InvalidateCache()
	}
	return changed
}

// currentSpinnerIndicators returns the left and right spinner frame characters for the title bar,
// or "" when the spinner is disabled or the task is complete.
func (m *ProgramBoxModel) currentSpinnerIndicators() (left, right string) {
	if m.done {
		return "", ""
	}
	return m.titleSpinner.Indicators()
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
