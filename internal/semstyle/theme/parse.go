// Package semtheme parses theme files (TOML) into resolved semantic style maps for the
// semstyle engine. It is the optional theming layer: it depends on semstyle (for tag
// delimiters and registration) but knows nothing about app config, file paths, or logging —
// those concerns stay in the host application, which discovers theme data and feeds it here.
package semtheme

import (
	"fmt"
	"strings"

	"DockSTARTer2/internal/semstyle"

	"github.com/pelletier/go-toml/v2"
)

// ThemeFile is the parsed structure of a theme (TOML) file.
//
// Defaults is an opaque, app-defined table: semtheme does not interpret it. Whatever keys a
// theme places under [defaults] are parsed generically; the consuming application decodes
// them into its own settings (e.g. via mapstructure). This keeps semtheme free of any
// particular UI vocabulary (borders, dialogs, panels, …).
type ThemeFile struct {
	Metadata struct {
		Name        string `toml:"name"`
		Description string `toml:"description"`
		Author      string `toml:"author"`
	} `toml:"metadata"`
	Syntax *struct {
		SemanticPrefix string `toml:"semantic_prefix"`
		SemanticSuffix string `toml:"semantic_suffix"`
		DirectPrefix   string `toml:"direct_prefix"`
		DirectSuffix   string `toml:"direct_suffix"`
	} `toml:"syntax"`
	Defaults map[string]any    `toml:"defaults"`
	Palette  map[string]string `toml:"palette"`
	Styles   map[string]string `toml:"styles"`
}

// PrefixTag joins an optional namespace prefix with a tag name. With no prefix the name
// is returned as-is; otherwise the prefix (trailing "_" trimmed) and name are joined by "_".
func PrefixTag(prefix, name string) string {
	if prefix == "" {
		return name
	}
	p := strings.TrimSuffix(prefix, "_")
	return p + "_" + name
}

// substitutePaletteVars replaces $varname tokens in s with values from palette.
func substitutePaletteVars(s string, palette map[string]string) string {
	if !strings.ContainsRune(s, '$') {
		return s
	}
	var b strings.Builder
	i := 0
	for i < len(s) {
		if s[i] != '$' {
			b.WriteByte(s[i])
			i++
			continue
		}
		j := i + 1
		for j < len(s) && (s[j] == '_' || (s[j] >= 'A' && s[j] <= 'Z') || (s[j] >= 'a' && s[j] <= 'z') || (j > i+1 && s[j] >= '0' && s[j] <= '9')) {
			j++
		}
		if j > i+1 {
			name := s[i+1 : j]
			if val, ok := palette[name]; ok {
				b.WriteString(val)
				i = j
				continue
			}
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

// resolveThemeValue recursively resolves a theme value string, handling semantic references
// and overrides, returning a raw fg:bg:flags style string.
func resolveThemeValue(raw string, rawValues map[string]string, visiting map[string]bool,
	semPre, semSuf, dirPre, dirSuf string) (string, error) {

	var finalFG, finalBG string
	var finalFlags string

	mergeStyle := func(styleStr string) {
		inner := styleStr
		switch {
		case strings.HasPrefix(inner, dirPre) && strings.HasSuffix(inner, dirSuf):
			inner = inner[len(dirPre) : len(inner)-len(dirSuf)]
		case strings.HasPrefix(inner, semPre) && strings.HasSuffix(inner, semSuf):
			inner = inner[len(semPre) : len(inner)-len(semSuf)]
		default:
			inner = semstyle.StripDelimiters(inner)
		}
		if inner == "-" {
			inner = "-:-:-"
		}

		parts := strings.Split(inner, ":")

		if len(parts) > 0 && parts[0] != "" {
			finalFG = parts[0]
		}
		if len(parts) > 1 && parts[1] != "" {
			finalBG = parts[1]
		}
		if len(parts) > 2 {
			for _, f := range parts[2] {
				finalFlags += string(f)
			}
		}
	}

	cur := raw
	for {
		nextSem := strings.Index(cur, semPre)
		nextDir := strings.Index(cur, dirPre)
		if nextSem == -1 && nextDir == -1 {
			break
		}

		var start int
		var closeSuf string
		switch {
		case nextSem == -1:
			start, closeSuf = nextDir, dirSuf
		case nextDir == -1:
			start, closeSuf = nextSem, semSuf
		case nextDir < nextSem:
			start, closeSuf = nextDir, dirSuf
		default:
			start, closeSuf = nextSem, semSuf
		}

		end := strings.Index(cur[start:], closeSuf)
		if end == -1 {
			break
		}
		end += start + len(closeSuf)

		tag := cur[start:end]

		if strings.HasPrefix(tag, dirPre) {
			mergeStyle(tag)
		} else if strings.HasPrefix(tag, semPre) {
			refKey := strings.TrimSuffix(strings.TrimPrefix(tag, semPre), semSuf)

			semanticRef := refKey
			modifiers := ""
			if idx := strings.IndexByte(refKey, ':'); idx >= 0 {
				semanticRef = refKey[:idx]
				modifiers = refKey[idx+1:]
			}

			targetKey := semanticRef
			if _, exists := rawValues[targetKey]; exists {
				if visiting[targetKey] {
					return "", fmt.Errorf("circular reference to %q in theme", targetKey)
				}
				visiting[targetKey] = true
				resolvedRef, err := resolveThemeValue(rawValues[targetKey], rawValues, visiting,
					semPre, semSuf, dirPre, dirSuf)
				delete(visiting, targetKey)
				if err != nil {
					return "", err
				}
				mergeStyle(resolvedRef)
				if modifiers != "" {
					mergeStyle(modifiers)
				}
				cur = cur[end:]
				continue
			}

			// Fallback to global semantic tags (e.g. Notice, Success). Re-wrap in the
			// engine's standard delimiters so ExpandConsoleTags can resolve it regardless
			// of the file-specific delimiters in use.
			standardTag := semstyle.SemanticPrefix + semanticRef + semstyle.SemanticSuffix
			expanded := semstyle.ToTags(standardTag)
			if expanded != standardTag && expanded != "" {
				mergeStyle(expanded)
			}
			if modifiers != "" {
				mergeStyle(modifiers)
			}
		}

		cur = cur[end:]
	}

	return fmt.Sprintf("%s:%s:%s", finalFG, finalBG, finalFlags), nil
}

// ResolveValue resolves a single theme value string against rawValues, returning a raw
// fg:bg:flags style string. Exported for callers that resolve individual values.
func ResolveValue(raw string, rawValues map[string]string, visiting map[string]bool,
	semPre, semSuf, dirPre, dirSuf string) (string, error) {
	return resolveThemeValue(raw, rawValues, visiting, semPre, semSuf, dirPre, dirSuf)
}

// ResolveColors resolves all style values in tf (palette substitution + semantic
// reference resolution), returning a map keyed like tf.Styles with raw fg:bg:flags values.
func ResolveColors(tf ThemeFile) (map[string]string, error) {
	semPre, semSuf := semstyle.SemanticPrefix, semstyle.SemanticSuffix
	dirPre, dirSuf := semstyle.DirectPrefix, semstyle.DirectSuffix
	if tf.Syntax != nil {
		if tf.Syntax.SemanticPrefix != "" {
			semPre = tf.Syntax.SemanticPrefix
		}
		if tf.Syntax.SemanticSuffix != "" {
			semSuf = tf.Syntax.SemanticSuffix
		}
		if tf.Syntax.DirectPrefix != "" {
			dirPre = tf.Syntax.DirectPrefix
		}
		if tf.Syntax.DirectSuffix != "" {
			dirSuf = tf.Syntax.DirectSuffix
		}
	}

	colors := tf.Styles
	if len(tf.Palette) > 0 {
		resolved := make(map[string]string, len(tf.Palette))
		for k, v := range tf.Palette {
			resolved[k] = v
		}
		for range len(tf.Palette) {
			changed := false
			for k, v := range resolved {
				if s := substitutePaletteVars(v, resolved); s != v {
					resolved[k] = s
					changed = true
				}
			}
			if !changed {
				break
			}
		}
		colors = make(map[string]string, len(tf.Styles))
		for key, raw := range tf.Styles {
			colors[key] = substitutePaletteVars(raw, resolved)
		}
	}

	resolved := make(map[string]string, len(colors))
	for key, raw := range colors {
		visiting := make(map[string]bool)
		styleValue, err := resolveThemeValue(raw, colors, visiting, semPre, semSuf, dirPre, dirSuf)
		if err != nil {
			return nil, fmt.Errorf("theme key %q: %w", key, err)
		}
		resolved[key] = styleValue
	}
	return resolved, nil
}

// Parse unmarshals TOML theme data into a ThemeFile.
func Parse(data []byte) (ThemeFile, error) {
	var tf ThemeFile
	if err := toml.Unmarshal(data, &tf); err != nil {
		return ThemeFile{}, err
	}
	return tf, nil
}

// RegisterInto parses TOML theme data, resolves its styles, and registers them into the
// semstyle theme map under the given prefix. Returns the theme's opaque [defaults] table for
// the caller to interpret. When prefix is empty (the main theme) it re-applies base tags and
// rebuilds the color map.
func RegisterInto(data []byte, prefix string) (map[string]any, error) {
	tf, err := Parse(data)
	if err != nil {
		return nil, err
	}
	resolved, err := ResolveColors(tf)
	if err != nil {
		return nil, err
	}
	for key, styleValue := range resolved {
		semstyle.RegisterThemeTagRaw(PrefixTag(prefix, key), styleValue)
	}
	if prefix == "" {
		semstyle.BuildColorMap()
	}
	return tf.Defaults, nil
}
