package commands

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/logger"
	"DockSTARTer2/internal/paths"
)

// tintSchemeBaseURL is where --theme-tint-repo resolves a scheme name to a
// downloadable file. Scheme names match a file's slug in this directory
// (e.g. "gruvbox-dark-hard" -> gruvbox-dark-hard.yaml).
const tintSchemeBaseURL = "https://raw.githubusercontent.com/tinted-theming/schemes/spec-0.11/base16/"

// tintHTTPClient is used for --theme-tint-repo's scheme download. A short
// timeout since this is a small, synchronous, interactive CLI command.
var tintHTTPClient = &http.Client{Timeout: 15 * time.Second}

// parseConnTypeList parses a --theme-tint* command's connection-type
// argument: "all", a single conn type, or a comma-separated list of them.
func parseConnTypeList(s string) ([]string, error) {
	if s == "all" {
		return []string{"local", "ssh", "web"}, nil
	}
	var types []string
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		switch part {
		case "local", "ssh", "web":
			if !slices.Contains(types, part) {
				types = append(types, part)
			}
		default:
			return nil, fmt.Errorf("unknown connection type %q (valid: local, ssh, web, all, or a comma-separated list)", part)
		}
	}
	if len(types) == 0 {
		return nil, fmt.Errorf("no connection type given (valid: local, ssh, web, all, or a comma-separated list)")
	}
	return types, nil
}

// parseOptionalConnTypeList is parseConnTypeList, but an empty string (the
// argument wasn't given at all) means "all" -- used by --theme-tint /
// --theme-no-tint, whose connection-type argument is optional.
func parseOptionalConnTypeList(s string) ([]string, error) {
	if s == "" {
		s = "all"
	}
	return parseConnTypeList(s)
}

// tintStateFile returns the path a connType's downloaded/copied scheme file
// is stored at -- one persistent copy per connection type in DS2's state
// dir, independent of wherever the original source (a URL or another local
// path) came from.
func tintStateFile(connType string) string {
	return filepath.Join(paths.GetStateDir(), "ansi_tint_"+connType+".yaml")
}

// setAnsiColorsField applies fn to the AnsiColors for each of connTypes on
// conf, in place.
func setAnsiColorsField(conf *config.AppConfig, connTypes []string, fn func(*config.AnsiColors)) {
	for _, ct := range connTypes {
		switch ct {
		case "local":
			fn(&conf.AnsiColors.Local)
		case "ssh":
			fn(&conf.AnsiColors.SSH)
		case "web":
			fn(&conf.AnsiColors.Web)
		}
	}
}

// applyTint validates data as a base16 scheme, then for each of connTypes:
// copies it to that connection type's state file, points
// ansi_palette.<connType>.scheme_file at it, and re-enables the tint
// (clearing any earlier --theme-no-tint).
func applyTint(ctx context.Context, connTypes []string, data []byte, source string) error {
	if _, err := config.ParseBase16Scheme(data); err != nil {
		return fmt.Errorf("%s does not look like a valid base16 scheme: %w", source, err)
	}

	if err := os.MkdirAll(paths.GetStateDir(), 0700); err != nil {
		return fmt.Errorf("creating state directory: %w", err)
	}

	conf := config.LoadAppConfig()
	for _, ct := range connTypes {
		dest := tintStateFile(ct)
		if err := os.WriteFile(dest, data, 0600); err != nil {
			return fmt.Errorf("writing tint file for %s: %w", ct, err)
		}
		setAnsiColorsField(&conf, []string{ct}, func(c *config.AnsiColors) {
			c.SchemeFile = tintStateFile(ct)
			c.Disabled = false
		})
	}

	if err := config.SaveAppConfig(conf); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}

	logger.Notice(ctx, "ANSI palette tint set from %s for: {{|Var|}}%s{{[-]}}", source, strings.Join(connTypes, ", "))
	return nil
}

// HandleThemeTintRepo implements --theme-tint-repo <types> <scheme-name>,
// downloading a named tinted-theming base16 scheme
// (github.com/tinted-theming/schemes) and applying it as an ANSI palette
// tint for the given connection type(s).
func HandleThemeTintRepo(ctx context.Context, group *CommandGroup) error {
	if len(group.Args) < 2 {
		logger.Error(ctx, "Usage: --theme-tint-repo <local|ssh|web|all|a,b,c> <scheme-name>")
		return fmt.Errorf("missing arguments")
	}
	connTypes, err := parseConnTypeList(group.Args[0])
	if err != nil {
		logger.Error(ctx, "%v", err)
		return err
	}
	schemeName := group.Args[1]

	url := tintSchemeBaseURL + schemeName + ".yaml"
	resp, err := tintHTTPClient.Get(url) //nolint:gosec,noctx
	if err != nil {
		logger.Error(ctx, "Downloading scheme '{{|Theme|}}%s{{[-]}}': %v", schemeName, err)
		return err
	}
	defer resp.Body.Close() //nolint:errcheck
	if resp.StatusCode != http.StatusOK {
		err := fmt.Errorf("scheme %q not found (HTTP %d) -- check the name against %s", schemeName, resp.StatusCode, "https://github.com/tinted-theming/schemes/tree/spec-0.11/base16")
		logger.Error(ctx, "%v", err)
		return err
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Error(ctx, "Reading downloaded scheme: %v", err)
		return err
	}

	return applyTint(ctx, connTypes, data, "'"+schemeName+"'")
}

// HandleThemeTintFile implements --theme-tint-file <types> <path>, applying
// a local base16 scheme YAML file as an ANSI palette tint for the given
// connection type(s).
func HandleThemeTintFile(ctx context.Context, group *CommandGroup) error {
	if len(group.Args) < 2 {
		logger.Error(ctx, "Usage: --theme-tint-file <local|ssh|web|all|a,b,c> <path>")
		return fmt.Errorf("missing arguments")
	}
	connTypes, err := parseConnTypeList(group.Args[0])
	if err != nil {
		logger.Error(ctx, "%v", err)
		return err
	}
	path := group.Args[1]

	data, err := os.ReadFile(path)
	if err != nil {
		logger.Error(ctx, "Reading '{{|File|}}%s{{[-]}}': %v", path, err)
		return err
	}

	return applyTint(ctx, connTypes, data, "'"+path+"'")
}

// HandleThemeTintOnOff implements --theme-tint [types] and --theme-no-tint
// [types], toggling whether an already-configured tint (SchemeFile/the 16
// explicit fields) is applied, without discarding any of it. types is
// optional on both -- omitted means "all".
func HandleThemeTintOnOff(ctx context.Context, group *CommandGroup) error {
	typesArg := ""
	if len(group.Args) > 0 {
		typesArg = group.Args[0]
	}
	connTypes, err := parseOptionalConnTypeList(typesArg)
	if err != nil {
		logger.Error(ctx, "%v", err)
		return err
	}

	disabled := group.Command == "--theme-no-tint"

	conf := config.LoadAppConfig()
	setAnsiColorsField(&conf, connTypes, func(c *config.AnsiColors) {
		c.Disabled = disabled
	})
	if err := config.SaveAppConfig(conf); err != nil {
		logger.Error(ctx, "Failed to save tint setting: %v", err)
		return err
	}

	if disabled {
		logger.Notice(ctx, "ANSI palette tint disabled for: {{|Var|}}%s{{[-]}}", strings.Join(connTypes, ", "))
	} else {
		logger.Notice(ctx, "ANSI palette tint enabled for: {{|Var|}}%s{{[-]}}", strings.Join(connTypes, ", "))
	}
	return nil
}
