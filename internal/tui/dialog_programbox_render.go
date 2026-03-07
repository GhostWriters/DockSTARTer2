package tui

import (
	"context"
	"fmt"
	"io"
	"strings"

	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/logger"
	"DockSTARTer2/internal/strutil"
	"DockSTARTer2/internal/theme"

	"charm.land/lipgloss/v2"
)

// renderHeaderUI renders the tasks and progress bar
func (m *ProgramBoxModel) renderHeaderUI(width int) string {
	if m.subtitle == "" && len(m.Tasks) == 0 && m.Percent == 0 {
		return ""
	}

	var b strings.Builder
	ctx := GetActiveContext()
	bgStyle := lipgloss.NewStyle().Background(ctx.Dialog.GetBackground())
	hasPrevious := false

	subtitleStyle := bgStyle.Width(width).Padding(0, 2)
	spacer := bgStyle.Width(width).Render("")

	// Subtitle (rendered as a heading)
	if m.subtitle != "" {
		subtitle := RenderThemeText(m.subtitle, ctx.Dialog)
		renderedSubtitle := subtitleStyle.Render(subtitle)
		b.WriteString(renderedSubtitle + "\n")
		hasPrevious = true
	}

	// Tasks
	if len(m.Tasks) > 0 {
		if hasPrevious {
			b.WriteString(spacer + "\n") // Gap after subtitle
		}
		hasPrevious = true

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
							}
						} else {
							appStyle = SemanticStyle("{{|Theme_ProgressInProgress|}}")
						}
					}

					if app == t.ActiveApp {
						appParts = append(appParts, SemanticStyle("{{|Theme_Highlight|}}").Render(app))
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

		barStyle = ApplyThickBorderCtx(barStyle, ctx)
		borderedBar := barStyle.Render(barView)

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

// calculateHeaderHeight returns the estimated height of the header UI
func (m *ProgramBoxModel) calculateHeaderHeight(width int) int {
	headerHeight := 0
	hasPrevious := false

	if m.subtitle != "" {
		// Render subtitle to calculate its actual wrapped height
		wrappedSubtitle := lipgloss.NewStyle().Width(width).Render(m.subtitle)
		headerHeight += lipgloss.Height(wrappedSubtitle)
		hasPrevious = true
	}

	if len(m.Tasks) > 0 {
		if hasPrevious {
			headerHeight++ // Gap after subtitle
		}
		hasPrevious = true

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
		headerHeight += 3 // Bordered bar (Border-Top, Content, Border-Bottom). Gap before was removed.
	}
	return headerHeight
}

// SetSize updates the dialog dimensions (called by AppModel on window resize).
func (m *ProgramBoxModel) SetSize(w, h int) {
	m.width = w
	m.height = h
	m.calculateLayout()
}

// calculateLayout performs all vertical budgeting in one place.
// This implements the "calculate once, use everywhere" pattern.
func (m *ProgramBoxModel) calculateLayout() {
	if m.width == 0 || m.height == 0 {
		return
	}

	layout := GetLayout()
	hasShadow := currentConfig.UI.Shadow
	contentW := m.width - 2
	shadowHeight := 0
	if hasShadow {
		shadowHeight = DialogShadowHeight
	}

	// 1. Header
	headerHeight := m.calculateHeaderHeight(contentW)

	// 2. Command
	commandLines := 0
	if m.command != "" {
		commandLines = 1 + 1 // 1 line for command + 1 line for gap
	}

	internalOverhead := headerHeight + commandLines

	// 3. Buttons — width and height aware via ButtonRowHeight.
	// Compute how many rows the button row itself can have after reserving:
	//   outer borders(2) + overhead + min viewport(2) + viewport borders(2) + shadow.
	buttons := 0
	if m.done {
		const minVpRows = 2
		availableForButton := m.height - 2 - internalOverhead - minVpRows - 2 - shadowHeight
		buttons = ButtonRowHeight(contentW, availableForButton, ButtonSpec{Text: "OK"})
	}

	// 4. Viewport height.
	// DialogContentHeight always budgets DialogButtonHeight (3) when hasButtons=true;
	// recover the freed lines when flat buttons (1) are used instead.
	vpHeight := layout.DialogContentHeight(m.height, internalOverhead, m.done, hasShadow)
	if m.done && buttons != DialogButtonHeight {
		vpHeight += DialogButtonHeight - buttons
	}
	// Subtract internal viewport chrome (top border + custom bottom line).
	vpHeight -= 2
	if vpHeight < 2 {
		vpHeight = 2
	}

	overhead := m.height - vpHeight

	// Save to layout struct
	m.layout = DialogLayout{
		Width:          m.width,
		Height:         m.height,
		HeaderHeight:   headerHeight,
		CommandHeight:  commandLines,
		ViewportHeight: vpHeight,
		ButtonHeight:   buttons,
		ShadowHeight:   shadowHeight,
		Overhead:       overhead,
	}

	// Update viewport dimensions
	m.viewport.SetWidth(m.width - 4)
	m.viewport.SetHeight(vpHeight)

	// Refresh content with new word wrap width
	if m.viewport.Width() > 0 && len(m.rawLines) > 0 {
		content := lipgloss.NewStyle().
			Width(m.viewport.Width()).
			Render(strings.Join(m.rawLines, "\n"))
		m.viewport.SetContent(content)
	}
}

// GetHelpText returns the dynamic help text based on the current state
// Implements DynamicHelpProvider interface for use with DialogWithBackdrop
func (m *ProgramBoxModel) GetHelpText() string {
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

	// Enable TUI mode for console prompts
	console.SetTUIEnabled(true)
	defer console.SetTUIEnabled(false)

	logger.TUIMode = true
	defer func() { logger.TUIMode = false }()

	// Initialize TUI if not already done
	cfg := config.LoadAppConfig()

	currentConfig = cfg // Set global config so styles like AddShadow work correctly
	if _, err := theme.Load(cfg.UI.Theme, ""); err == nil {
		InitStyles(cfg)
	}

	// Create dialog model
	dialogModel := NewProgramBoxModel(title, subtitle, "")
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
	p := NewProgram(model)

	registerCallbacks()
	defer func() {
		program = nil
		deregisterCallbacks()
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
