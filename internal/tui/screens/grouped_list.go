package screens

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"DockSTARTer2/internal/displayengine"
)

// listGroup is one source's items (e.g. Bundled, User) in a grouped list.
type listGroup struct {
	Label string
	Items []displayengine.MenuItem
}

// letterDividerMinItems is how many items a group must show before it gets
// dividers between first letters.
const letterDividerMinItems = 20

// groupedItems joins groups into one list. Each group with items starts
// with a category heading carrying its label, so a filtered list still
// shows where its items come from; in a group with more than
// letterDividerMinItems items, a plain divider also separates each change
// of first letter.
func groupedItems(groups []listGroup) []displayengine.MenuItem {
	var items []displayengine.MenuItem
	for _, g := range groups {
		if len(g.Items) == 0 {
			continue
		}
		items = append(items, displayengine.MenuItem{Tag: g.Label, IsSeparator: true, IsCategory: true})
		lastLetter := ""
		for _, it := range g.Items {
			if len(g.Items) > letterDividerMinItems {
				letter := firstLetter(it.Tag)
				if lastLetter != "" && letter != lastLetter {
					items = append(items, displayengine.MenuItem{IsSeparator: true})
				}
				lastLetter = letter
			}
			items = append(items, it)
		}
	}
	return items
}

// firstLetter returns tag's first letter, uppercased, or "#" when it starts
// with anything else.
func firstLetter(tag string) string {
	plain := strings.TrimSpace(displayengine.GetPlainText(tag))
	r, size := utf8.DecodeRuneInString(plain)
	switch {
	case size == 0:
		return ""
	case unicode.IsLetter(r):
		return string(unicode.ToUpper(r))
	}
	return "#"
}
