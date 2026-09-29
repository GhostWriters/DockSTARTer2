#!/usr/bin/env python3
"""Syncs the built-in themes' [styles] tables with .FALLBACKS.ds2theme.

1. Adds every fallback style a theme doesn't mention as a commented
   "# Key = fallback" line: into .TEMPLATE.ds2theme in .FALLBACKS order,
   then into each theme in .TEMPLATE order, after the nearest earlier key it
   already has.
2. Aligns each block (a run of key lines between blank lines and "##"
   headings) of every theme and .TEMPLATE: the "=" column, and a "# = <fallback>"
   annotation column two spaces past the block's longest line. Active lines
   are annotated with their current fallback unless their value is it;
   commented lines show it as their value.

Run after adding or changing a fallback. Safe to re-run: a second run
changes nothing. Usage: scripts/sync_themes.py [--check]
  --check  report what would change without writing (exit 1 if anything)
"""
import os
import re
import sys

THEMES = os.path.join(os.path.dirname(os.path.abspath(__file__)), '..', 'internal', 'assets', 'themes')
FALLBACKS = os.path.join(THEMES, '.FALLBACKS.ds2theme')
TEMPLATE = os.path.join(THEMES, '.TEMPLATE.ds2theme')

VALUE = r'("(?:[^"\\]|\\.)*"|\'[^\']*\')'
KEY = re.compile(r'^(\s*#?\s*)([A-Za-z][A-Za-z0-9]*)(\s*)=\s*(.*?)\s*$')
ACTIVE = re.compile(r'^(\s+)([A-Za-z][A-Za-z0-9]*)\s*=\s*' + VALUE + r'\s*(?:#\s*=\s*(.*?))?\s*$')
COMMENTED = re.compile(r'^#\s*([A-Za-z][A-Za-z0-9]*)\s*=\s*' + VALUE + r'\s*$')
FB_KEY = re.compile(r'^([A-Za-z][A-Za-z0-9]*)\s*=\s*' + VALUE)


def read(path):
    raw = open(path, encoding='utf-8', newline='').read()
    return raw.replace('\r\n', '\n'), '\r\n' in raw


def write(path, text, crlf):
    if crlf:
        text = text.replace('\n', '\r\n')
    open(path, 'w', encoding='utf-8', newline='').write(text)


def styles_bounds(lines):
    start = next(i for i, line in enumerate(lines) if line.strip() == '[styles]')
    end = next((i for i in range(start + 1, len(lines)) if re.match(r'^\[', lines[i])), len(lines))
    return start, end


def is_key_line(line):
    return not line.lstrip().startswith('##') and KEY.match(line) is not None


def fallback_values():
    """Fallback style values and their order, from .FALLBACKS' [styles]."""
    lines = read(FALLBACKS)[0].split('\n')
    start, end = styles_bounds(lines)
    values, order = {}, []
    for line in lines[start + 1:end]:
        m = FB_KEY.match(line)
        if m and m.group(1) not in values:
            values[m.group(1)] = m.group(2)
            order.append(m.group(1))
    return values, order


def key_lines(lines):
    """Key -> line index within [styles], active or commented."""
    start, end = styles_bounds(lines)
    out = {}
    for i in range(start + 1, end):
        if is_key_line(lines[i]):
            out.setdefault(KEY.match(lines[i]).group(2), i)
    return out


def ordered_keys(lines):
    start, end = styles_bounds(lines)
    keys = []
    for i in range(start + 1, end):
        if is_key_line(lines[i]):
            key = KEY.match(lines[i]).group(2)
            if key not in keys:
                keys.append(key)
    return keys


def add_missing(text, order, values):
    """text with each style in order it lacks added as a commented line after
    the nearest earlier one it has (or before the nearest later one)."""
    lines = text.split('\n')
    for idx, key in enumerate(order):
        present = key_lines(lines)
        if key in present or key not in values:
            continue
        line = '# ' + key + ' = ' + values[key]
        before = next((present[k] for k in reversed(order[:idx]) if k in present), None)
        if before is not None:
            lines.insert(before + 1, line)
            continue
        after = next((present[k] for k in order[idx + 1:] if k in present), None)
        if after is not None:
            lines.insert(after, line)
    return '\n'.join(lines)


def parse(line):
    """(kind, key, value, indent) for a key line, else None."""
    m = ACTIVE.match(line)
    if m:
        return 'active', m.group(2), m.group(3), m.group(1)
    m = COMMENTED.match(line)
    if m:
        return 'commented', m.group(1), m.group(2), '# '
    return None


def align(text, values):
    lines = text.split('\n')
    start, end = styles_bounds(lines)
    i = start + 1
    while i < end:
        if parse(lines[i]) is None:
            i += 1
            continue
        j = i
        while j < end and parse(lines[j]) is not None:
            j += 1
        rows = []
        for kind, key, value, indent in (parse(line) for line in lines[i:j]):
            if kind == 'commented' and key in values:
                value = values[key]
            rows.append((kind, key, value, indent))
        eq = max(len(indent) + len(key) for _, key, _, indent in rows) + 1
        heads = [indent + key + ' ' * (eq - len(indent) - len(key)) + '= ' + value for _, key, value, indent in rows]
        # Past every line in the block, annotated or not.
        col = max(len(head) for head in heads) + 2
        for n, ((kind, key, value, _), head) in enumerate(zip(rows, heads)):
            # Annotated unless the value is its fallback anyway.
            if kind == 'active' and key in values and value != values[key]:
                head += ' ' * (col - len(head)) + '# = ' + values[key]
            lines[i + n] = head
        i = j
    return '\n'.join(lines)


def main():
    check = '--check' in sys.argv
    values, fb_order = fallback_values()
    changed = []

    text, crlf = read(TEMPLATE)
    new = align(add_missing(text, fb_order, values), values)
    if new != text:
        changed.append(os.path.basename(TEMPLATE))
        if not check:
            write(TEMPLATE, new, crlf)
    tpl_order = ordered_keys(new.split('\n'))

    for name in sorted(os.listdir(THEMES)):
        if not name.endswith('.ds2theme') or name.startswith('.'):
            continue
        path = os.path.join(THEMES, name)
        text, crlf = read(path)
        new = align(add_missing(text, tpl_order, values), values)
        if new != text:
            changed.append(name)
            if not check:
                write(path, new, crlf)

    verb = 'would change' if check else 'changed'
    print(f'{verb}: {", ".join(changed)}' if changed else 'all themes in sync')
    return 1 if check and changed else 0


if __name__ == '__main__':
    sys.exit(main())
