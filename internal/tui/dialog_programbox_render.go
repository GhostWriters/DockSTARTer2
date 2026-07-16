package tui

import (
	"context"
	"fmt"
	"io"
	"strings"

	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/displayengine"
	"DockSTARTer2/internal/logger"
	"DockSTARTer2/internal/strutil"
	"DockSTARTer2/internal/theme"

	"charm.land/lipgloss/v2"
)

// renderHeaderUI renders the tasks and progress bar. Subtitle is rendered by
// its own plain-text Content section (subtitleSection) instead of here.
func (m *ProgramBoxModel) renderHeaderUI(width int) string {
	if len(m.Tasks) == 0 && m.Percent == 0 {
		return ""
	}

	var b strings.Builder
	ctx := displayengine.GetActiveContext()
	bgStyle := ctx.Dialog

	spacer := bgStyle.Width(width).Render("")

	// Tasks
	if len(m.Tasks) > 0 {
		maxLabelLen := 0
		for _, t := range m.Tasks {
			if len(t.Label) > maxLabelLen {
				maxLabelLen = len(t.Label)
			}
		}

		for i, t := range m.Tasks {
			// Add blank line between different categories (e.g. Removing vs Adding)
			if i > 0 && t.Label != m.Tasks[i-1].Label {
				b.WriteString(spacer + "\n")
			}

			catStyle := theme.ThemeSemanticStyle("{{|ProgressWaiting|}}")
			statusText := " Waiting "
			switch t.Status {
			case StatusInProgress:
				catStyle = theme.ThemeSemanticStyle("{{|ProgressInProgress|}}")
				statusText = " In Progress "
			case StatusCompleted:
				catStyle = theme.ThemeSemanticStyle("{{|ProgressCompleted|}}")
				statusText = " Completed "
			}

			gapWidth := maxLabelLen - len(t.Label) + 2
			gap := bgStyle.Render(strutil.Repeat(" ", gapWidth))

			headerLine := lipgloss.JoinHorizontal(lipgloss.Top,
				bgStyle.Render(" "), // 1-space margin
				catStyle.Render(t.Label),
				gap,
				catStyle.Render("[ "+statusText+" ]"),
			)
			b.WriteString(bgStyle.Width(width).Render(headerLine) + "\n")

			if t.Command != "" || len(t.Apps) > 0 {
				var appParts []string
				if t.Command != "" {
					appParts = append(appParts, catStyle.Render(t.Command))
				}

				foundActive := false
				for _, app := range t.Apps {
					appStyle := theme.ThemeSemanticStyle("{{|ProgressWaiting|}}")
					switch t.Status {
					case StatusCompleted:
						appStyle = theme.ThemeSemanticStyle("{{|ProgressCompleted|}}")
					case StatusInProgress:
						if t.ActiveApp != "" {
							if app == t.ActiveApp {
								appStyle = theme.ThemeSemanticStyle("{{|ProgressInProgress|}}")
								foundActive = true
							} else if !foundActive {
								appStyle = theme.ThemeSemanticStyle("{{|ProgressCompleted|}}")
							}
						} else {
							appStyle = theme.ThemeSemanticStyle("{{|ProgressInProgress|}}")
						}
					}

					if app == t.ActiveApp {
						appParts = append(appParts, theme.ThemeSemanticStyle("{{|Highlight|}}").Render(app))
					} else {
						appParts = append(appParts, appStyle.Render(app))
					}
				}

				// Join with spaces and wrap with indentation matching the bash script
				// Bash uses Indent (3 spaces). Header is at 1, so apps at 4 = 3 space difference.
				appText := strings.Join(appParts, " ")
				appLine := lipgloss.NewStyle().
					Width(width).
					PaddingLeft(4).
					Background(ctx.Dialog.GetBackground()).
					Render(appText)

				b.WriteString(appLine + "\n")
			}
		}
	}

	// Progress Bar
	if m.Percent > 0 {
		barMargin := 10 // Safe margin for centering
		barWidth := width - barMargin
		if barWidth < 10 {
			barWidth = 10
		}
		m.progress.SetWidth(barWidth)

		barView := m.progress.ViewAs(m.Percent)

		barStyle := lipgloss.NewStyle().
			Padding(0, 1).
			Background(ctx.Dialog.GetBackground())

		barStyle = displayengine.ApplyThickBorderCtx(barStyle, ctx)
		borderedBar := displayengine.InjectBorderFlags(barStyle.Render(barView), ctx.BorderFlags, ctx.Border2Flags, true)

		// Center the multiline bordered bar line consistently
		centeredBar := lipgloss.NewStyle().
			Width(width).
			Align(lipgloss.Center).
			Background(ctx.Dialog.GetBackground()).
			Render(borderedBar)

		b.WriteString(centeredBar + "\n")
	}

	return strings.TrimSuffix(b.String(), "\n")
}

// calculateHeaderHeight returns the estimated height of the header UI.
// Subtitle is measured by its own plain-text Content section (subtitleSection)
// instead of here.
func (m *ProgramBoxModel) calculateHeaderHeight(width int) int {
	headerHeight := 0

	if len(m.Tasks) > 0 {
		for i, t := range m.Tasks {
			if i > 0 && t.Label != m.Tasks[i-1].Label {
				headerHeight++ // Blank line between task categories
			}
			headerHeight++ // Label/Status line

			if t.Command != "" || len(t.Apps) > 0 {
				// Replicate app list line building for height calculation (including wrap and indent)
				var appText strings.Builder
				if t.Command != "" {
					appText.WriteString(t.Command + " ")
				}
				for j, app := range t.Apps {
					appText.WriteString(app)
					if j < len(t.Apps)-1 {
						appText.WriteString(" ")
					}
				}

				// Measure height with word wrapping and 4-char padding using lipgloss
				// Background style isn't strictly needed just for height measurement
				rendered := lipgloss.NewStyle().Width(width).PaddingLeft(4).Render(appText.String())
				headerHeight += lipgloss.Height(rendered)
			}
		}
	}
	if m.Percent > 0 {
		headerHeight += 3 // Bordered bar (Border-Top, displayengine.Content, Border-Bottom). Gap before was removed.
	}
	return headerHeight
}

// SetSize updates the dialog dimensions (called by AppModel on window resize).
// Delegates to outer, which drives calculateSectionLayout -- the header/
// command/viewport sections each compute their own height (fixed sections
// via sectionHeightOverride, the viewport via IsVariableHeight filling
// whatever's left), replacing this wrapper's former hand-rolled
// calculateLayout.
func (m *ProgramBoxModel) SetSize(w, h int) {
	m.width = w
	m.height = h
	m.outer.SetSize(w, h)
}

// GetHelpText returns the dynamic help text based on the current state
// Implements displayengine.DynamicHelpProvider interface for use with DialogWithBackdrop
func (m *ProgramBoxModel) GetHelpText() string {
	scrollInfo := ""
	if m.sv.TotalLineCount() > m.sv.VisibleLineCount() {
		scrollPercent := m.sv.ScrollPercent()
		scrollInfo = fmt.Sprintf(" | %d%%", int(scrollPercent*100))
	}

	if m.done {
		if m.err != nil {
			return "Error: " + m.err.Error() + scrollInfo + " | Press Enter or Esc to close"
		}
		return "Complete" + scrollInfo + " | Press Enter or Esc to close | PgUp/PgDn to scroll"
	}
	return "Running." + scrollInfo + " | Press Ctrl+C to cancel | PgUp/PgDn to scroll"
}

// HelpText satisfies the model_update.go helpline interface (mirrors GetHelpText).
func (m *ProgramBoxModel) HelpText() string { return m.GetHelpText() }

// RunProgramBox displays a program box dialog that shows command output
func RunProgramBox(ctx context.Context, title, subtitle, command string, task func(context.Context, io.Writer) error) error {

	// Enable TUI mode for console prompts
	console.SetTUIEnabled(true)
	defer console.SetTUIEnabled(false)

	logger.TUIMode = true
	defer func() { logger.TUIMode = false }()

	// Initialize TUI if not already done
	cfg := config.LoadAppConfig()

	console.SpinnerEnabled = cfg.UI.Spinner
	console.SpinnerSpeed = console.AlignToRefreshRate(cfg.UI.SpinnerSpeed, cfg.UI.RefreshRate)
	console.LineCharacters = cfg.UI.LineCharacters
	if _, err := theme.Load(cfg.UI.Theme, ""); err == nil {
		displayengine.InitStyles(cfg)
	}

	// Create dialog model
	dialogModel := NewProgramBoxModel(title, subtitle, command).WithDialogType(displayengine.DialogTypeSuccess)
	dialogModel.ctx = ctx
	dialogModel.SetTask(task)
	dialogModel.SetMaximized(true)

	// Create full app model with standalone dialog to include log panel and backdrop
	model := NewAppModelStandalone(ctx, cfg, "local", "cli", parseSessionKey(nil), dialogModel)

	// Create Bubble Tea program
	p := NewProgram(model, ProgramOptions{RefreshRate: cfg.UI.RefreshRate})

	registerCallbacks()
	defer func() {
		program = nil
		deregisterCallbacks()
	}()

	// Listen for context cancellation to shutdown program (standalone)
	go func() {
		<-ctx.Done()
		if p != nil {
			p.Quit()
		}
	}()

	// Run the program (Init will start the task)
	finalModel, err := p.Run()

	// Reset terminal colors on exit to prevent "bleeding" into the shell prompt
	fmt.Print("\x1b[0m\n")

	if err != nil {
		return err
	}

	// Extract details from the model
	if m, ok := finalModel.(*AppModel); ok {
		if m.Fatal {
			logger.TUIMode = false
			console.AbortHandler(ctx)
			return console.ErrUserAborted
		}
		if box, ok := m.dialog.(*ProgramBoxModel); ok {
			return box.err
		}
	}

	return nil
}
