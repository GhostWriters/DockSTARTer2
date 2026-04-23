package cmd

import (
	"DockSTARTer2/internal/appenv"
	"DockSTARTer2/internal/logger"
	"bytes"
	"context"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"os"
	"regexp"

	// "github.com/eliukblau/pixterm/pkg/ansimage"
	"github.com/pgavlin/goldmark"
	"github.com/pgavlin/goldmark/renderer"
	"github.com/pgavlin/goldmark/text"
	"github.com/pgavlin/goldmark/util"
	kit_renderer "github.com/pgavlin/markdown-kit/renderer"
	"github.com/pgavlin/markdown-kit/styles"
	"github.com/pgavlin/goldmark/ast"
	_ "github.com/gen2brain/svg"
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

	// Force Kitty encoder for Linux/Web terminal support
	encoder := kit_renderer.KittyGraphicsEncoder()

	// Use markdown-kit renderer with auto-detected theme
	kitR := kit_renderer.New(
		kit_renderer.WithTheme(styles.AutoTheme()),
		kit_renderer.WithWordWrap(0), // Let the terminal handle wrapping
		kit_renderer.WithImages(true, 0, ""),
		kit_renderer.WithImageEncoder(encoder),
		kit_renderer.WithHyperlinks(true),
	)

	// Custom image renderer struct to fix SVG issues
	fixer := &imageFixerRenderer{
		kitR:    kitR,
		encoder: encoder,
	}

	// Create a goldmark renderer and register our terminal NodeRenderer
	mainR := renderer.NewRenderer(renderer.WithNodeRenderers(
		util.Prioritized(kitR, 100),
		util.Prioritized(fixer, 0),
	))

	// Parse the markdown into an AST
	source := []byte(out)
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

type imageFixerRenderer struct {
	kitR    *kit_renderer.Renderer
	encoder kit_renderer.ImageEncoder
}

func (r *imageFixerRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(ast.KindImage, r.renderImage)
}

func (r *imageFixerRenderer) renderImage(w util.BufWriter, source []byte, node ast.Node, enter bool) (ast.WalkStatus, error) {
	if !enter {
		return ast.WalkContinue, nil
	}

	img := node.(*ast.Image)
	dest := string(img.Destination)

	// 1. Fetch the image data manually so we can clean it
	resp, err := http.Get(dest)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[IMAGE ERROR] Fetch failed for %s: %v\n", dest, err)
		return r.kitR.RenderImage(w, source, node, enter) // Fallback to standard
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[IMAGE ERROR] Read failed for %s: %v\n", dest, err)
		return r.kitR.RenderImage(w, source, node, enter)
	}

	// 2. Fix shorthand decimal points in SVGs (e.g., .1 -> 0.1) which break the decoder
	if bytes.Contains(data, []byte("<svg")) {
		// Fix leading dots: (.1) -> (0.1),  .1 ->  0.1, etc.
		reLeading := regexp.MustCompile(`(^|[^0-9.])\.([0-9])`)
		data = reLeading.ReplaceAll(data, []byte(`${1}0.${2}`))

		// Fix stuck dots: 1.2.3 -> 1.2 0.3 (common in SVG paths)
		reStuck := regexp.MustCompile(`([0-9]+\.[0-9]+)\.([0-9])`)
		for reStuck.Match(data) {
			data = reStuck.ReplaceAll(data, []byte(`${1} 0.${2}`))
		}

		// Strip embedded icons (<image> tags) because ok-svg doesn't support nested SVGs
		reImage := regexp.MustCompile(`(?s)<image\b[^>]*/>`)
		data = reImage.ReplaceAll(data, []byte(""))

		// Strip links (<a> tags) because ok-svg doesn't support them
		reLinkOpen := regexp.MustCompile(`(?i)<a\b[^>]*>`)
		data = reLinkOpen.ReplaceAll(data, []byte(""))
		reLinkClose := regexp.MustCompile(`(?i)</a>`)
		data = reLinkClose.ReplaceAll(data, []byte(""))

		// Replace rgba(...,0) with none because ok-svg doesn't support rgba
		reRGBA := regexp.MustCompile(`rgba\([^)]*,0\)`)
		data = reRGBA.ReplaceAll(data, []byte("none"))
	}

	// 3. Decode the cleaned image
	imgObj, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		fmt.Fprintf(os.Stderr, "[IMAGE ERROR] Decode failed for %s: %v\n", dest, err)
		return r.kitR.RenderImage(w, source, node, enter)
	}

	// 4. Encode using Kitty
	if _, err := r.encoder(w, imgObj, r.kitR); err != nil {
		fmt.Fprintf(os.Stderr, "[IMAGE ERROR] Encode failed for %s: %v\n", dest, err)
		return r.kitR.RenderImage(w, source, node, enter)
	}

	return ast.WalkSkipChildren, nil
}
