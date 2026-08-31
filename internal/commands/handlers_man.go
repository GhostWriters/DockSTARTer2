package commands

import (
	"context"
	"fmt"
	"golang.org/x/term"
	"os"
	"regexp"

	"DockSTARTer2/internal/appenv"
	"DockSTARTer2/internal/logger"

	glamour "charm.land/glamour/v2"
	glamouransi "charm.land/glamour/v2/ansi"
	glamourstyles "charm.land/glamour/v2/styles"
	"charm.land/lipgloss/v2"
)

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

	// Pre-process: convert Shields.io images to links to ensure visibility/clickability
	reBadge := regexp.MustCompile(`!\[([^\]]*)\]\(([^)]*shields\.io[^)]*)\)`)
	out = reBadge.ReplaceAllString(out, `[$1]($2)`)

	styleName := glamourstyles.LightStyle
	if lipgloss.HasDarkBackground(os.Stdin, os.Stdout) {
		styleName = glamourstyles.DarkStyle
	}

	r, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle(styleName),
		glamour.WithWordWrap(width),
		glamour.WithHyperlinkMode(glamouransi.HyperlinkModeInline),
	)
	if err != nil {
		return err
	}

	rendered, err := r.Render(out)
	if err != nil {
		return err
	}
	// Output directly to stdout to avoid any string mangling.
	os.Stdout.WriteString(rendered)
	return nil
}
