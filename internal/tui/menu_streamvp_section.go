package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

// newStreamOutputSection builds a borderless, variable-height Content section
// wrapping a ProgramBoxModel's streaming output viewport (box.sv) -- the
// first variable-height contentRenderer section in the codebase, following
// the NewSinputSection recipe (contentRenderer + extraHitRegions +
// SetUpdateInterceptor) but reporting IsVariableHeight() true so it fills
// whatever height calculateSectionLayout has left after the fixed header/
// command sections and buttons, the same "grow to fill" mechanism Config
// Apps Menu's app list already uses for its own list.
//
// The viewport owns its own fully self-contained inner border (with a
// scroll-percent bottom border when content overflows) -- rendered inside
// this section's contentRenderer closure -- so the section itself is
// borderless (SetBorderless) to avoid a double-nested border.
func newStreamOutputSection(id string, box *ProgramBoxModel) *MenuModel {
	m := NewMenuModel(id, "", "", nil)
	m.SetSubMenuMode(true)
	m.SetIsDialog(false)
	m.SetButtons([]ButtonDef{})
	m.SetMaximized(true)
	m.SetVariableHeight(true)
	m.SetShowLockGutter(false)
	m.SetNoLeftMargin(true)
	m.SetBorderless(true)
	m.SetNonFocusable(true)
	m.SetWantsAllMessages(true)

	// Always maximized in practice (see newProgramBox), so calculateSectionLayout's
	// Pass 1 natural-height query (menu_sections.go's expandableNaturalTotal, used
	// only in the non-maximized shrink path) should never actually run for this
	// section -- but report a real, generous value rather than the generic
	// contentRenderer fallback (1 + BorderHeight()) in case a future caller ever
	// constructs a non-maximized ProgramBox.
	m.sectionHeightOverride = func(width int) int {
		return 20
	}

	m.contentRenderer = func(contentWidth int) string {
		ctx := GetActiveContext()

		viewportContent := MaintainBackground(box.sv.View(), ctx.Console)
		viewportContent = ApplyScrollbar(&box.Scroll, viewportContent, box.sv.TotalLineCount(), box.sv.Height(), box.sv.YOffset(), ctx.LineCharacters, ctx)

		viewportStyle := ctx.Console.Padding(0, 0)
		viewportStyle = ApplyInnerBorderCtx(viewportStyle, box.focused, ctx)
		viewportStyle = viewportStyle.BorderBottom(false)

		borderedViewport := InjectBorderFlags(
			viewportStyle.Height(box.sv.Height()).Render(viewportContent),
			ctx.BorderFlags, ctx.Border2Flags, false)

		totalWidth := box.sv.Width() + ScrollbarGutterWidth + 2
		borderedViewport = strings.TrimSuffix(borderedViewport, "\n")
		if box.Scroll.Info.Needed {
			borderedViewport = borderedViewport + "\n" + BuildScrollPercentBottomBorder(totalWidth, box.sv.ScrollPercent(), box.focused, ctx)
		} else {
			borderedViewport = borderedViewport + "\n" + BuildPlainBottomBorder(totalWidth, box.focused, ctx)
		}
		return borderedViewport
	}

	m.extraHitRegions = func(offsetX, offsetY, baseZ int) []HitRegion {
		var regions []HitRegion
		layout := GetLayout()

		regions = append(regions, HitRegion{
			ID:     id + ".viewport",
			X:      offsetX + layout.SingleBorder(),
			Y:      offsetY + layout.SingleBorder(),
			Width:  box.sv.Width(),
			Height: box.sv.Height(),
			ZOrder: baseZ + 10,
			Label:  "Output Viewport",
			Help: &HelpContext{
				ScreenName: box.title,
				PageTitle:  "Output Viewer",
				PageText:   "Displays the live output of a running command or script.",
				ItemText:   "Scroll with the mouse wheel or use Home/End/PgUp/PgDn to review output.",
			},
		})

		if currentConfig.UI.Scrollbar && box.Scroll.Info.Needed {
			sbX := offsetX + layout.SingleBorder() + box.sv.Width()
			sbTopY := offsetY + layout.SingleBorder()
			regions = append(regions, box.Scroll.HitRegions(sbX, sbTopY, baseZ+20, "Output")...)
		}

		return regions
	}

	m.SetUpdateInterceptor(func(msg tea.Msg, menu *MenuModel) (tea.Cmd, bool) {
		// Centralized scrollbar processing first (throttling, clicks, dragging) --
		// same pattern the pre-migration wrapper used at the top of its own Update.
		if newOff, cmd, changed := box.Scroll.Update(msg, box.sv.YOffset(), box.sv.TotalLineCount(), box.sv.Height()); changed {
			box.sv.SetYOffset(newOff)
			return cmd, true
		}

		switch m := msg.(type) {
		case tea.KeyPressMsg:
			switch m.String() {
			case "home":
				box.sv.GotoTop()
				return nil, true
			case "end":
				box.sv.GotoBottom()
				return nil, true
			}
		case LayerHitMsg, LayerWheelMsg,
			tea.MouseClickMsg, tea.MouseWheelMsg, tea.MouseMotionMsg, tea.MouseReleaseMsg:
			// Mouse/hit messages are already fully handled by box.Scroll.Update
			// above; don't also forward them into sv.ViewUpdate (matches the
			// pre-migration wrapper's explicit exclusion, avoiding focus
			// side-effects from the inner bubbles/viewport's own mouse handling).
			return nil, true
		}

		return box.sv.ViewUpdate(msg), true
	})

	m.onResize = func(width, height int) {
		innerW := width - ScrollbarGutterWidth - GetLayout().BorderWidth()
		innerH := height - GetLayout().BorderHeight()
		box.sv.SetSize(innerW, innerH)
		setActiveOutputWidth(innerW)
		box.sv.ReRenderWith(pbRenderFn())
	}

	return m
}
