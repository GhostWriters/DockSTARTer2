package classic

import (
	"strings"
	"testing"
)

// TestMaxRawTitleWidth_NeverGrows is a round-trip test against the actual
// renderer: for a range of content widths and widget counts, truncating a
// long title to MaxRawTitleWidth's result and rendering it via
// RenderBorderedBoxCtx (titleTag "RAW", exactly renderPane's own call
// shape) must never produce a box wider than the contentWidth requested --
// i.e. must never trigger RenderBorderedBoxCtx's grow-to-fit title/widget
// behavior. This is the property MaxRawTitleWidth exists to guarantee, so
// it's verified against the real render rather than just its own formula.
func TestMaxRawTitleWidth_NeverGrows(t *testing.T) {
	ctx := StyleContext{LineCharacters: true, DrawBorders: true}
	longTitle := ".env.app.immich___postgres"

	widgetSets := [][]WidgetDef{
		nil,
		{{ID: "a", Glyph: "▤", IconName: "SideBySide"}},
		{{ID: "a", Glyph: "▤", IconName: "SideBySide"}, {ID: "b", Glyph: "□", IconName: "Maximize"}},
	}

	for _, align := range []string{"left", "center"} {
		for _, widgets := range widgetSets {
			// Below minPaneContentWidth (the tabbed vars editor's own
			// floor -- see tabbed_vars_editor.go), even an empty title
			// might not leave room for the widgets themselves, which no
			// amount of title truncation can fix. Real panes never render
			// narrower than that floor, so this is the range that matters.
			for contentWidth := 30; contentWidth <= 60; contentWidth++ {
				maxTitle := MaxRawTitleWidth(contentWidth, true, align, widgets, ctx)
				title := TruncateRight(longTitle, maxTitle)
				tabRow := RenderTitleSegmentCtx(title, false, false, true, "TitleSubMenu", ctx)
				tbs := TitleBarState{Show: len(widgets) > 0, Widgets: widgets}

				box := RenderBorderedBoxCtx(tabRow, "x", contentWidth, 3, false, false, true, align, "RAW", ctx, tbs)
				lines := strings.Split(box, "\n")
				if len(lines) == 0 {
					t.Fatalf("contentWidth=%d align=%s widgets=%d: empty render", contentWidth, align, len(widgets))
				}
				gotWidth := WidthWithoutZones(lines[0])
				if gotWidth > contentWidth+2 { // +2 for the box's own left/right border chars
					t.Errorf("contentWidth=%d align=%s widgets=%d maxTitle=%d: top row width=%d, want <= %d",
						contentWidth, align, len(widgets), maxTitle, gotWidth, contentWidth+2)
				}
			}
		}
	}
}
