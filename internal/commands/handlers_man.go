package commands

import (
	"context"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	tea "charm.land/bubbletea/v2"
	glamour "charm.land/glamour/v2"
	glamouransi "charm.land/glamour/v2/ansi"
	glamourstyles "charm.land/glamour/v2/styles"
	"charm.land/lipgloss/v2"
	"golang.org/x/term"

	"DockSTARTer2/internal/appenv"
	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/graphics"
	"DockSTARTer2/internal/logger"
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

// manProtocol maps a session's resolved "can show Kitty placeholder images"
// flag (see AppModel.kittyImages) to a protocol, for --man run from the
// console panel.
func manProtocol(kittyImages bool) graphics.Protocol {
	if kittyImages {
		return graphics.ProtocolKitty
	}
	return graphics.ProtocolNone
}

func HandleMan(ctx context.Context, group *CommandGroup, protocol graphics.Protocol) error {
	if len(group.Args) == 0 {
		logger.Error(ctx, "The '{{|UserCommand|}}%s{{[-]}}' command requires an application name.", group.Command)
		return fmt.Errorf("no application name provided")
	}

	out, err := appenv.GetAppMarkdown(ctx, group.Args[0])
	if err != nil {
		logger.Error(ctx, "%v", err)
		return err
	}

	// Under -g or the console panel the output goes to a viewport inside a
	// running TUI, not to the terminal.
	tuiW, inTUI := ctx.Value(console.TUIWriterKey).(io.Writer)

	// Width for proper wrapping and soft-break handling: the viewport's, or
	// the terminal's.
	width := 0
	styleName := glamourstyles.DarkStyle
	if inTUI {
		if console.OutputContentWidthFn != nil {
			width = console.OutputContentWidthFn()
		}
		if width <= 0 {
			width = 80
		}
	} else {
		if w, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil {
			width = w
		}
		if !lipgloss.HasDarkBackground(os.Stdin, os.Stdout) {
			styleName = glamourstyles.LightStyle
		}
	}

	mode := config.LoadAppConfig().Appearance.ForConnType(console.ConnTypeFromContext(ctx)).MarkdownHyperlinks
	// glamour itself only has two modes (Auto/Inline) -- "off" renders with
	// Auto (link text + visible URL) and strips the resulting OSC8 escapes
	// afterward, leaving plain readable text with no embedded hyperlink.
	hyperlinkMode := glamouransi.HyperlinkModeInline
	if mode == "off" || mode == "auto" {
		hyperlinkMode = glamouransi.HyperlinkModeAuto
	}

	opts := []glamour.TermRendererOption{
		glamour.WithStandardStyle(styleName),
		glamour.WithWordWrap(width),
		glamour.WithHyperlinkMode(hyperlinkMode),
		// Inline covers tables too: no reference numbers and URL list under them.
		glamour.WithInlineTableLinks(hyperlinkMode == glamouransi.HyperlinkModeInline),
	}
	// Kitty placeholders are plain text cells (the image data goes to the
	// terminal separately), so they survive a TUI. A drawn image needs no
	// "Image: alt" caption, so the caption is empty; an image that fails
	// still shows its link.
	var imageOpts []glamour.TermRendererOption
	switch {
	case protocol == graphics.ProtocolKitty:
		// Placeholders also outside a TUI: unlike direct images, which are
		// drawn below their paragraph, they keep one-row badges inline.
		imageOpts = []glamour.TermRendererOption{glamour.WithImageProtocol(glamouransi.ImageProtocolKittyPlaceholders)}
	case protocol == graphics.ProtocolSixel && !inTUI:
		imageOpts = []glamour.TermRendererOption{glamour.WithImageProtocol(glamouransi.ImageProtocolSixel)}
	}
	if imageOpts != nil {
		style := glamourstyles.DarkStyleConfig
		if styleName == glamourstyles.LightStyle {
			style = glamourstyles.LightStyleConfig
		}
		style.ImageText.Format = `{{""}}` // an empty caption: nothing is written
		opts = append(opts, glamour.WithStyles(style), glamour.WithRemoteImages())
		opts = append(opts, imageOpts...)
	}
	r, err := glamour.NewTermRenderer(opts...)
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
	if !inTUI {
		// Output directly to stdout to avoid any string mangling. The image
		// data goes first, so the placeholders show their images.
		w := console.SpinnerSafeWriter(os.Stdout)
		if gfx := r.GraphicsCommands(); len(gfx) > 0 {
			if _, err := io.WriteString(w, strings.Join(gfx, "")); err != nil {
				return err
			}
		}
		_, err = io.WriteString(w, rendered)
		return err
	}
	// The image data has to reach the terminal before the placeholders show.
	if gfx := r.GraphicsCommands(); len(gfx) > 0 {
		if send := console.SendFuncFromContext(ctx); send != nil {
			send(tea.RawMsg{Msg: strings.Join(gfx, "")})
		}
	}
	_, err = io.WriteString(tuiW, rendered+"\n")
	return err
}
