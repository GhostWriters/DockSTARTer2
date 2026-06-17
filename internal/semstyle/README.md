# semstyle

A small, dependency-light engine for **semantic terminal styling** in Go. Write text with
named, tag-based markup and resolve it to ANSI escape sequences at render time:

```go
semstyle.ToConsoleANSI("{{|Error|}}failed{{[-]}}: {{[red::B]}}retry{{[-]}}")
```

`semstyle` is built around two ideas:

- **Semantic tags** — `{{|Name|}}…{{[-]}}` — reference a *named* style (e.g. `Error`,
  `Success`, `Title`) resolved against a style map. Change the map (a theme) and every tag
  re-styles, without touching call sites.
- **Direct tags** — `{{[fg:bg:flags]}}…{{[-]}}` — inline ANSI styling, e.g. `{{[red:black:B]}}`
  for bold red on black. Flags: `B`old, `D`im, `U`nderline, `I`talic, `L` blink, `R`everse,
  `S`trikethrough (uppercase = on, with `-` prefix = off).

It depends only on `lipgloss`, `colorprofile`, and `tcell/color` for color resolution — no
application, TTY, or config coupling.

## Two ways to use it

### 1. Package-level (simple, one global config)

A process-wide `Default` styler backs the package functions. This is all most programs need:

```go
import "…/semstyle"

fmt.Println(semstyle.ToConsoleANSI("{{|Notice|}}hello{{[-]}}"))
semstyle.RegisterConsoleTag("Notice", "{{[cyan::B]}}") // define a semantic tag
plain := semstyle.Strip(styled)                        // remove all tags + ANSI
```

### 2. Per-instance `Styler` (multiple independent configs)

Each `Styler` owns its own tag/color maps, so you can run several independent style
configurations in one process (e.g. different themes for different surfaces):

```go
s := semstyle.New()
s.RegisterThemeTag("Title", "{{[magenta::B]}}")
out := s.ToConsoleANSI("{{|Title|}}Report{{[-]}}")
```

The package functions are thin delegators to `Default`, so the global API and the
per-instance API are identical in behavior.

## Style resolution

Each `Styler` keeps two semantic maps:

- **console map** — built-in / base tags (the defaults from `RegisterBaseTags`).
- **theme map** — overrides loaded from a theme; takes precedence over the console map.

`ToConsoleANSI` resolves against the console map only; `ToThemeANSI` resolves theme-first
with console fallback. Supply a theme map with `SetThemeMap` (or the `semtheme` companion
package, which parses theme files into a map).

## Key API

| Function / method | Purpose |
|---|---|
| `ToConsoleANSI(s)` | Expand tags using base (console) styles → ANSI |
| `ToThemeANSI(s)` / `…WithPrefix` | Expand tags theme-first → ANSI |
| `Strip(s)` | Remove all semantic/direct tags **and** ANSI escapes |
| `RegisterConsoleTag(name, val)` / `…Raw` | Define a base semantic tag |
| `RegisterThemeTag(name, val)` / `…Raw` | Define a theme semantic tag |
| `SetThemeMap(m)` | Replace the theme map wholesale |
| `SetRenderPolicy(fn)` | Gate rendering (return false → strip instead of color) |
| `New()` | Create an independent `*Styler` |
| `SetDelimiters(…)` | Customize the tag delimiters (process-wide) |

## Render policy

By default `ToConsoleANSI` always emits ANSI. A host that wants to suppress color when
output is redirected (or any other condition) sets a policy:

```go
semstyle.SetRenderPolicy(func() bool { return isTerminal() })
```

When the policy returns false, `ToConsoleANSI` strips instead of rendering.

## Notes

- **Delimiters** (`{{|`, `|}}`, `{{[`, `]}}`) and the detected **color profile** are
  process-wide, not per-`Styler` — they describe the markup format and the terminal, not an
  individual style configuration.
- Hard-reset constants (`CodeHardReset`, etc.) are multi-parameter SGR variants useful when
  a compositor intercepts single-parameter resets.
