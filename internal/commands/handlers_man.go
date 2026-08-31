package commands

import (
	"context"
	"fmt"
	"golang.org/x/term"
	"os"
	"regexp"

	"DockSTARTer2/internal/appenv"
	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/logger"

	glamour "charm.land/glamour/v2"
	glamouransi "charm.land/glamour/v2/ansi"
	glamourstyles "charm.land/glamour/v2/styles"
	"charm.land/lipgloss/v2"
)

// osc8MarkerRegex matches a single OSC 8 hyperlink marker (open or close),
// mirroring displayengine/classic's own -- duplicated locally rather than
// imported, since internal/displayengine already imports internal/commands
// (via panel_update.go), so the reverse import would cycle. Matches markers
// independently rather than paired open-content-close spans so nested
// hyperlinks (e.g. an image inside a link) strip correctly.
var osc8MarkerRegex = regexp.MustCompile(`\x1b\]8;[^\x07\x1b]*(?:\x07|\x1b\\)`)

// stripHyperlinks removes OSC 8 hyperlink markers from rendered text,
// leaving just the visible content.
func stripHyperlinks(rendered string) string {
	return osc8MarkerRegex.ReplaceAllString(rendered, "")
}

func HandleMan(ctx context.Context, group *CommandGroup, canDisplayGraphics bool) error {
	if len(group.Args) == 0 {
		logger.Error(ctx, "The '{{|UserCommand|}}%s{{[-]}}' command requires an application name.", group.Command)
		return fmt.Errorf("no application name provided")
	}

	out, err := appenv.GetAppMarkdown(ctx, group.Args[0])
	if err != nil {
		logger.Error(ctx, "%v", err)
		return err
	}

	// Detect terminal width for proper wrapping and soft-break handling
	width := 0
	if w, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil {
		width = w
	}

	styleName := glamourstyles.LightStyle
	if lipgloss.HasDarkBackground(os.Stdin, os.Stdout) {
		styleName = glamourstyles.DarkStyle
	}

	mode := config.LoadAppConfig().UI.MarkdownHyperlinks
	// glamour itself only has two modes (Auto/Inline) -- "off" renders with
	// Auto (link text + visible URL) and strips the resulting OSC8 escapes
	// afterward, leaving plain readable text with no embedded hyperlink.
	hyperlinkMode := glamouransi.HyperlinkModeInline
	if mode == "off" || mode == "auto" {
		hyperlinkMode = glamouransi.HyperlinkModeAuto
	}

	r, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle(styleName),
		glamour.WithWordWrap(width),
		glamour.WithHyperlinkMode(hyperlinkMode),
	)
	if err != nil {
		return err
	}

	rendered, err := r.Render(out)
	if err != nil {
		return err
	}
	if mode == "off" {
		rendered = stripHyperlinks(rendered)
	}
	// Output directly to stdout to avoid any string mangling.
	os.Stdout.WriteString(rendered)
	return nil
}
