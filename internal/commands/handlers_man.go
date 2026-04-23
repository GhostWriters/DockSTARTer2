package commands

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"regexp"
	"golang.org/x/term"

	"DockSTARTer2/internal/appenv"
	"DockSTARTer2/internal/graphics"
	"DockSTARTer2/internal/logger"

	// "github.com/eliukblau/pixterm/pkg/ansimage"
	"github.com/pgavlin/goldmark"
	"github.com/pgavlin/goldmark/renderer"
	"github.com/pgavlin/goldmark/text"
	"github.com/pgavlin/goldmark/util"
	kit_renderer "github.com/pgavlin/markdown-kit/renderer"
	"github.com/pgavlin/markdown-kit/styles"
	_ "github.com/gen2brain/svg"
)

func HandleMan(ctx context.Context, group *CommandGroup) error {
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

	// Use smart graphics detection for high-fidelity on Linux and clean links on Windows
	canDisplay := graphics.CanDisplayGraphics()
	encoder := graphics.SixelGraphicsEncoder()

	// Use markdown-kit renderer with auto-detected theme
	kitR := kit_renderer.New(
		kit_renderer.WithTheme(styles.AutoTheme()),
		kit_renderer.WithWordWrap(width),
		kit_renderer.WithSoftBreak(width != 0),
		kit_renderer.WithImages(canDisplay, 0, ""),
		kit_renderer.WithImageEncoder(encoder),
		kit_renderer.WithHyperlinks(true),
	)

	// Create a goldmark renderer and register our terminal NodeRenderer
	mainR := renderer.NewRenderer(renderer.WithNodeRenderers(
		util.Prioritized(kitR, 100),
	))

	// Parse the markdown into an AST
	source := []byte(out)

	// Pre-process: convert Shields.io images to links to ensure visibility/clickability
	reBadge := regexp.MustCompile(`!\[([^\]]*)\]\(([^)]*shields\.io[^)]*)\)`)
	source = reBadge.ReplaceAll(source, []byte(`[$1]($2)`))

	parser := goldmark.DefaultParser()
	doc := parser.Parse(text.NewReader(source))


	var buf bytes.Buffer
	if err := mainR.Render(&buf, source, doc); err != nil {
		return err
	}
	// Output directly to stdout as bytes to avoid any string mangling (important for graphics)
	os.Stdout.Write(buf.Bytes())
	return nil
}

