# semtheme

The optional **theming layer** for [`semstyle`](..) (this is the `semstyle/theme`
subpackage). It parses theme files (TOML) into resolved semantic style maps and registers
them into a `semstyle` styler.

`semtheme` depends only on `semstyle` (plus a TOML parser). It knows nothing about
application config, file paths, or logging — the host application discovers theme bytes
(from disk, an embed, a URL, …) and hands them here. This keeps the theming logic reusable.

## Theme file format

A theme is TOML with semantic style definitions, an optional reusable palette, optional
custom delimiters, and optional UI defaults:

```toml
[metadata]
name        = "Midnight"
description = "Dark theme"
author      = "you"

[palette]
accent = "#7aa2f7"
bg     = "#1a1b26"

[styles]
Title   = "{{[$accent:$bg:B]}}"   # palette vars via $name
Error   = "{{[red::B]}}"
Notice  = "{{|Title|}}"            # semantic reference to another style
```

- **`[palette]`** entries are substituted (`$name`) into style values; palette entries may
  reference each other.
- **`[styles]`** values may use direct tags (`{{[fg:bg:flags]}}`), reference other styles in
  the file by name, or reference global semantic tags from the styler.
- Circular references are detected and reported as errors.

## Usage

```go
data, _ := os.ReadFile("midnight.theme")          // host fetches the bytes

// Parse + register directly into the default styler (prefix "" = the main theme):
defaults, err := semtheme.RegisterInto(data, "")

// …or work with the parsed form yourself:
tf, _      := semtheme.Parse(data)                // -> ThemeFile
styles, _  := semtheme.ResolveColors(tf)          // -> map[name]rawStyle
```

To register into a specific styler instance, resolve and apply the map:

```go
tf, _     := semtheme.Parse(data)
styles, _ := semtheme.ResolveColors(tf)
s := semstyle.New()
s.SetThemeMap(styles)
```

## API

### Theme file parsing

| Function | Purpose |
| --- | --- |
| `Parse(data)` | Unmarshal TOML → `ThemeFile` |
| `ResolveColors(tf)` | Resolve palette + semantic refs → `map[name]rawStyle` |
| `ResolveValue(raw, …)` | Resolve a single value string → raw `fg:bg:flags` |
| `RegisterInto(data, prefix)` | Parse, resolve, and register into the default styler under a prefix; returns the theme's opaque `[defaults]` table (`map[string]any`) |
| `PrefixTag(prefix, name)` | Join a namespace prefix with a tag name (`prefix_name`) |

### Lipgloss style conversion

These functions convert semantic/direct tags or raw style codes into **lipgloss styles**,
for use when building TUI components rather than writing ANSI to a terminal:

| Function | Purpose |
| --- | --- |
| `ToStyle(st, text, style, reset)` | Resolve all tags in `text` and apply them to `style`; resets to `reset` on a reset tag |
| `CodeToStyle(code, style, reset)` | Apply a raw `fg:bg:flags` code directly to `style` |
| `CodeToFlags(code)` | Parse the flags field of a raw code → `StyleFlags` struct |
| `ResetFlags(style)` | Clear all text attributes (bold, italic, etc.) from a style |

**`StyleFlags`** holds the parsed on/off state for each modifier (Bold, Underline, Italic,
Dim, Blink, Reverse, Strikethrough, HighIntensity). Its `.Apply(style)` method applies all
flags to a lipgloss style in one call.

Example — resolve a semantic tag to a lipgloss style:

```go
import (
    "…/semstyle"
    semtheme "…/semstyle/theme"
    "charm.land/lipgloss/v2"
)

base := lipgloss.NewStyle()
styled := semtheme.ToStyle(semstyle.Default, "{{|Error|}}", base, base)
// styled now has the fg/bg/flags of the Error semantic tag applied
```

## Types

- **`ThemeFile`** — the parsed theme: metadata, optional `[syntax]` delimiters, `[palette]`,
  `[styles]`, and `Defaults` (the opaque `[defaults]` table as `map[string]any`).
- **`StyleFlags`** — parsed modifier state from a flags field; `.Apply(style)` applies all flags.

`semtheme` intentionally does **not** define a typed defaults struct. The `[defaults]` table
is app-specific UI vocabulary (borders, panels, …), so it's passed through untyped; the
consuming application decodes it into its own settings (e.g. with `mapstructure`).

## Prefixes

A non-empty `prefix` namespaces a theme's tags (`prefix_tagname`), so multiple themes can be
registered into one styler without collisions — useful for previews or per-surface themes.
Registry lookups are case-insensitive.
