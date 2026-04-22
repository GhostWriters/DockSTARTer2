package cmd

import (
	"DockSTARTer2/internal/appenv"
	"DockSTARTer2/internal/logger"
	"bytes"
	"context"
	"fmt"
	"image/color"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"

	"github.com/eliukblau/pixterm/pkg/ansimage"
	"github.com/pgavlin/goldmark"
	"github.com/pgavlin/goldmark/renderer"
	"github.com/pgavlin/goldmark/text"
	"github.com/pgavlin/goldmark/util"
	kit_renderer "github.com/pgavlin/markdown-kit/renderer"
	"github.com/pgavlin/markdown-kit/styles"
	_ "github.com/pgavlin/svg2"
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

	// Determine the best image encoder for the current terminal
	supportsKitty := os.Getenv("TERM") == "xterm-kitty" || os.Getenv("KITTY_WINDOW_ID") != ""
	var encoder kit_renderer.ImageEncoder
	if supportsKitty {
		encoder = kit_renderer.KittyGraphicsEncoder()
	} else {
		// ANSI blocks fallback for terminals that don't support Kitty (like Windows Terminal)
		encoder = kit_renderer.ANSIGraphicsEncoder(color.Transparent, ansimage.DitheringWithChars)
	}

	// Use markdown-kit renderer with auto-detected theme
	kitR := kit_renderer.New(
		kit_renderer.WithTheme(styles.AutoTheme()),
		kit_renderer.WithWordWrap(0), // Let the terminal handle wrapping
		kit_renderer.WithImages(true, 0, ""),
		kit_renderer.WithImageEncoder(encoder),
		kit_renderer.WithHyperlinks(true),
	)

	// Create a goldmark renderer and register our terminal NodeRenderer
	mainR := renderer.NewRenderer(renderer.WithNodeRenderers(util.Prioritized(kitR, 100)))

	// Parse the markdown into an AST
	source := []byte(out)
	parser := goldmark.DefaultParser()
	doc := parser.Parse(text.NewReader(source))

	var buf bytes.Buffer
	if err := mainR.Render(&buf, source, doc); err != nil {
		logger.Error(ctx, "Failed to render documentation: %v", err)
		return err
	}
	rendered := buf.String()

	logger.Display(ctx, rendered)
	return nil
}
