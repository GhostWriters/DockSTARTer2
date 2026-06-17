# semtheme

The optional **theming layer** for [`semstyle`](../semstyle). It parses theme files (TOML)
into resolved semantic style maps and registers them into a `semstyle` styler.

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

| Function | Purpose |
|---|---|
| `Parse(data)` | Unmarshal TOML → `ThemeFile` |
| `ResolveColors(tf)` | Resolve palette + semantic refs → `map[name]rawStyle` |
| `ResolveValue(raw, …)` | Resolve a single value string → raw `fg:bg:flags` |
| `RegisterInto(data, prefix)` | Parse, resolve, and register into the default styler under a prefix; returns the theme's UI defaults |
| `PrefixTag(prefix, name)` | Join a namespace prefix with a tag name (`prefix_name`) |

## Types

- **`ThemeFile`** — the parsed theme (metadata, syntax/delimiters, defaults, palette, styles).
- **`ThemeDefaults`** — optional UI hints a theme may carry (borders, shadow, spinner, panel
  modes, etc.); pointer fields distinguish "unset" from a zero value. Interpreting these is
  the host application's responsibility.

## Prefixes

A non-empty `prefix` namespaces a theme's tags (`prefix_tagname`), so multiple themes can be
registered into one styler without collisions — useful for previews or per-surface themes.
Registry lookups are case-insensitive.
