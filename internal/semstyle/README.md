# semstyle

A small, dependency-light engine for **semantic terminal styling** in Go. Write text with
named, tag-based markup and resolve it to ANSI escape sequences at render time:

```go
semstyle.ToANSI("{{|Error|}}failed{{[-]}}: {{[red::B]}}retry{{[-]}}")
```

`semstyle` is built around two ideas:

- **Semantic tags** — `{{|Name|}}…{{[-]}}` — reference a *named* style (e.g. `Error`,
  `Success`, `Title`) resolved against a style map. Change the map (a theme) and every tag
  re-styles, without touching call sites.
- **Direct tags** — `{{[fg:bg:flags]}}…{{[-]}}` — inline ANSI styling, e.g. `{{[red:black:B]}}`
  for bold red on black. Flags: `B`old, `D`im, `U`nderline, `I`talic, `L` blink, `R`everse,
  `S`trikethrough (uppercase = on, lowercase = off, e.g. `b` = Bold off). `H` = High Intensity:
  shifts the fg and/or bg color to its bright variant (`red` → `bright-red`) without affecting
  attributes. A leading `-` in the flags field resets **all** attributes first, then applies the
  remaining flags (e.g. `{{[-B]}}` = reset all, then Bold on — use `b` for Bold off without a
  reset).

It depends only on `lipgloss`, `colorprofile`, and `tcell/color` for color resolution — no
application, TTY, or config coupling.

## Layout

- **`semstyle`** (this package) — the styling engine. Import it alone if you only need
  tag-based styling and define styles in code.
- **`semstyle/theme`** — an optional layer that parses theme **files** (TOML) into style
  maps for the engine. Import it only if you want file-driven themes; it pulls in a TOML
  parser. See [theme/README.md](theme/README.md) and the [Theming](#theming) section below.

They're one module, two packages: styling-only consumers never compile the theme package or
its TOML dependency.

## Two ways to use it

### 1. Package-level (simple, one global config)

A process-wide `Default` styler backs the package functions. This is all most programs need:

```go
import "…/semstyle"

fmt.Println(semstyle.ToANSI("{{|Notice|}}hello{{[-]}}"))
semstyle.RegisterConsoleTag("Notice", "{{[cyan::B]}}") // define a semantic tag
plain := semstyle.ToPlain(styled)                      // remove all tags + ANSI
```

### 2. Per-instance `Styler` (multiple independent configs)

Each `Styler` owns its own tag/color maps, so you can run several independent style
configurations in one process (e.g. different themes for different surfaces):

```go
s := semstyle.New()
s.RegisterThemeTag("Title", "{{[magenta::B]}}")
out := s.ToANSI("{{|Title|}}Report{{[-]}}")
```

The package functions are thin delegators to `Default`, so the global API and the
per-instance API are identical in behavior.

## Style resolution

Each `Styler` keeps two semantic maps:

- **console map** — built-in / base tags (the defaults from `RegisterBaseTags`).
- **theme map** — overrides loaded from a theme; takes precedence over the console map.

`ToANSI` without a prefix resolves against the console map only; with a prefix it resolves
theme-first with console fallback. Supply a theme map with `SetThemeMap` (or the `semtheme`
companion package, which parses theme files into a map).

## Key API

| Function / method | Purpose |
| --- | --- |
| `ToANSI(s, prefix...)` | Expand tags → ANSI; console map by default, theme map when prefix given |
| `ToTags(s, prefix...)` | Expand semantic tags → direct tags; stops before ANSI conversion |
| `ToPlain(s)` | Remove all tags **and** ANSI escapes → plain text |
| `StripTags(s)` | Remove tags only, leaving any existing ANSI intact |
| `ToColor(s)` | Color name or hex string → `color.Color` |
| `ToColorStr(c)` | `color.Color` → hex or ANSI index string |
| `Sprintf(fmt, a...)` | Format string then apply `ToANSI` |
| `RegisterConsoleTag(name, val)` / `…Raw` | Define a base semantic tag |
| `RegisterThemeTag(name, val)` / `…Raw` | Define a theme semantic tag |
| `SetThemeMap(m)` | Replace the theme map wholesale |
| `SetRenderPolicy(fn)` | Gate rendering (return false → `ToPlain` instead of color) |
| `New()` | Create an independent `*Styler` |
| `(*Styler).SetDelimiters(…)` | Customize this Styler's tag delimiters |
| `(*Styler).RegisterHyperlinkTag(name)` | Make a tag render its content as a terminal hyperlink |

## Converting to lipgloss styles

`ToANSI` produces terminal escape sequences for plain output. When building a
**lipgloss** `Style` (e.g. for a TUI component), use `ToTags` first to resolve semantic
names to raw `fg:bg:flags` codes, then feed the result into a style-builder. The
`fg:bg:flags` direct-tag format maps cleanly onto lipgloss foreground/background/modifier
calls — host applications typically wrap this in a helper, for example:

```go
rawCode := semstyle.StripDelimiters(semstyle.ToTags("{{|Error|}}"))
// rawCode = "red::B"  (or whatever Error resolves to)
style := semtheme.CodeToStyle(rawCode, lipgloss.NewStyle(), lipgloss.NewStyle())
```

`ToTags` is also the right choice when passing styled text to a TUI compositor that
understands direct tags natively rather than ANSI escapes.

## Delimiters

The default delimiters are the package-level standard: `{{|`…`|}}` (semantic) and `{{[`…`]}}`
(direct). Each `Styler` gets these at construction and can override them independently:

```go
s := semstyle.New()
s.SetDelimiters("{|", "|}", "{[", "]}")    // this Styler only
```

`semstyle.SetDelimiters(...)` (package-level) changes the standard values and applies them to
`Default`. Delimiters are per-`Styler` state, so different stylers can use different markup
syntaxes in the same process.

## Hyperlinks

Hyperlinking is opt-in and keyed on a tag name (not a style flag, so theme authors don't
have to remember anything). Register a tag, and its content up to the next reset becomes a
terminal hyperlink, using the content's plain text as the destination:

```go
semstyle.RegisterHyperlinkTag("URL")
semstyle.ToConsoleANSI("{{|URL|}}https://example.com{{[-]}}") // clickable link
```

Off by default. The registration lives on the `Styler` (independent of the console/theme
style maps), so it persists across theme changes.

## Render policy

By default `ToANSI` always emits ANSI. A host that wants to suppress color when output is
redirected (or any other condition) sets a policy:

```go
semstyle.SetRenderPolicy(func() bool { return isTerminal() })
```

When the policy returns false, `ToANSI` strips instead of rendering.

## Theming

The `semstyle/theme` subpackage parses theme **files** (TOML) into a style map you can hand
to a styler. It's optional — import it only for file-driven themes.

```go
import (
    "…/semstyle"
    semtheme "…/semstyle/theme"
)

data, _ := os.ReadFile("midnight.theme")   // your app fetches the bytes
defaults, _ := semtheme.RegisterInto(data, "") // parse + register into the Default styler
// `defaults` is the theme's opaque [defaults] table (map[string]any) for your app to interpret.
```

A theme file carries `[metadata]`, an optional `[palette]` (reusable `$vars`), `[styles]`
(semantic name → style, may reference palette vars or other styles), optional `[syntax]`
delimiter overrides, and an opaque `[defaults]` table that `semtheme` passes through without
interpreting (so any app can define whatever UI defaults it wants). Full details and the
theme-file format are in [theme/README.md](theme/README.md).

## Notes

- **Delimiters** and **hyperlink tags** are per-`Styler`; the package-level standard
  delimiter values seed each new `Styler`.
- The detected **color profile** is process-wide — it describes the output terminal, not an
  individual style configuration.
- Hard-reset constants (`CodeHardReset`, etc.) are multi-parameter SGR variants useful when
  a compositor intercepts single-parameter resets.
