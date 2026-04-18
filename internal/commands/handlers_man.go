package commands

import (
	"context"
	"fmt"
	"os"

	"DockSTARTer2/internal/appenv"
	"DockSTARTer2/internal/logger"

	"charm.land/glamour/v2"
	"charm.land/lipgloss/v2"
)

func handleMan(ctx context.Context, group *CommandGroup) error {
	if len(group.Args) == 0 {
		logger.Error(ctx, "The '{{|UserCommand|}}%s{{[-]}}' command requires an application name.", group.Command)
		return fmt.Errorf("no application name provided")
	}

	out, err := appenv.GetAppMarkdown(ctx, group.Args[0])
	if err != nil {
		logger.Error(ctx, "%v", err)
		return err
	}

	style := "dark"
	if !lipgloss.HasDarkBackground(os.Stdin, os.Stderr) {
		style = "light"
	}

	r, err := glamour.NewTermRenderer(
		glamour.WithStylePath(style),
		glamour.WithWordWrap(0),
	)
	if err != nil {
		logger.Error(ctx, "Failed to initialize markdown renderer: %v", err)
		return err
	}

	rendered, err := r.Render(out)
	if err != nil {
		logger.Error(ctx, "Failed to render documentation: %v", err)
		return err
	}

	logger.Display(ctx, rendered)
	return nil
}
