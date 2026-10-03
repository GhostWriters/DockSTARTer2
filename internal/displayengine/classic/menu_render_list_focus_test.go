package classic

import "testing"

func TestFocusedItemDesc(t *testing.T) {
	cases := []struct {
		in, want string
		ok       bool
	}{
		{"{{|ItemList|}}a", "{{|ItemListFocused|}}a", true},
		{"{{|ItemListUserDefined|}}a", "{{|ItemListUserDefinedFocused|}}a", true},
		{"{{|ItemList::::http://x|}}a{{[-]}}{{|ItemList|}} b", "{{|ItemListFocused::::http://x|}}a{{[-]}}{{|ItemListFocused|}} b", true},
		{"{{|Other|}}a", "", false},
		{"plain", "", false},
	}
	for _, c := range cases {
		got, ok := focusedItemDesc(c.in)
		if got != c.want || ok != c.ok {
			t.Errorf("focusedItemDesc(%q) = %q, %v; want %q, %v", c.in, got, ok, c.want, c.ok)
		}
	}
}
