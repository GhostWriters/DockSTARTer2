package screens

import (
	"fmt"
	"testing"

	"DockSTARTer2/internal/displayengine"
)

func items(tags ...string) []displayengine.MenuItem {
	out := make([]displayengine.MenuItem, len(tags))
	for i, t := range tags {
		out[i] = displayengine.MenuItem{Tag: t}
	}
	return out
}

// shape renders a grouped list as tags, with "--Label" for a labeled
// divider and "--" for a plain one.
func shape(list []displayengine.MenuItem) []string {
	var out []string
	for _, it := range list {
		if it.IsSeparator {
			out = append(out, "--"+it.Tag)
		} else {
			out = append(out, it.Tag)
		}
	}
	return out
}

func TestGroupedItemsSingleShortGroupIsLabeled(t *testing.T) {
	got := shape(groupedItems([]listGroup{{Label: "Bundled", Items: items("Alpha", "Beta")}, {Label: "User"}}))
	if fmt.Sprint(got) != "[--Bundled Alpha Beta]" {
		t.Errorf("got %v", got)
	}
}

func TestGroupedItemsLabelsEveryShownGroup(t *testing.T) {
	got := shape(groupedItems([]listGroup{
		{Label: "Current"},
		{Label: "Bundled", Items: items("Alpha", "Beta")},
		{Label: "User", Items: items("Mine")},
	}))
	if fmt.Sprint(got) != "[--Bundled Alpha Beta --User Mine]" {
		t.Errorf("got %v", got)
	}
}

func TestGroupedItemsLetterDividersOnlyWhenLong(t *testing.T) {
	var tags []string
	for i := range letterDividerMinItems + 1 {
		tags = append(tags, string(rune('a'+i/2))+"x")
	}
	got := groupedItems([]listGroup{{Label: "Repo", Items: items(tags...)}})
	dividers := 0
	for i, it := range got {
		if !it.IsSeparator || it.Tag != "" {
			continue
		}
		dividers++
		if i == 0 || i == len(got)-1 || firstLetter(got[i-1].Tag) == firstLetter(got[i+1].Tag) {
			t.Errorf("divider at %d is not between two letters", i)
		}
	}
	if want := (letterDividerMinItems+1+1)/2 - 1; dividers != want {
		t.Errorf("%d letter dividers, want %d", dividers, want)
	}

	short := groupedItems([]listGroup{{Label: "Repo", Items: items(tags[:letterDividerMinItems]...)}})
	for _, it := range short {
		if it.IsSeparator && it.Tag == "" {
			t.Fatalf("a list of %d items got a letter divider", letterDividerMinItems)
		}
	}
}
