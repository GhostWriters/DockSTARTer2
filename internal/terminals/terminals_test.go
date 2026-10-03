package terminals

import (
	"testing"

	"github.com/charmbracelet/colorprofile"
)

func TestEmbeddedTableParses(t *testing.T) {
	list, err := Entries()
	if err != nil {
		t.Fatalf("Entries: %v", err)
	}
	if len(list) == 0 {
		t.Fatal("Entries: empty table")
	}
}

func TestLookup(t *testing.T) {
	tests := []struct {
		reply   string
		name    string
		version string
		profile colorprofile.Profile
		ok      bool
	}{
		{"WezTerm 20240203-110809-5046fc22", "WezTerm", "20240203", colorprofile.TrueColor, true},
		{"kitty(0.35.2)", "kitty", "0.35.2", colorprofile.TrueColor, true},
		{"XTerm(390)", "XTerm", "390", colorprofile.TrueColor, true},
		{"XTerm(279)", "XTerm", "279", colorprofile.Unknown, true},
		{"tmux 3.4", "tmux", "3.4", colorprofile.Unknown, true},
		{"SomeTerm 1.0", "", "", colorprofile.Unknown, false},
		{"", "", "", colorprofile.Unknown, false},
	}
	for _, tt := range tests {
		info, ok := Lookup(tt.reply)
		if ok != tt.ok || info.Name != tt.name || info.Version != tt.version || info.Profile != tt.profile {
			t.Errorf("Lookup(%q) = %+v, %v; want {%s %s %v}, %v", tt.reply, info, ok, tt.name, tt.version, tt.profile, tt.ok)
		}
	}
}

func TestLookupDA1(t *testing.T) {
	if info, ok := LookupDA1("61;6;7;22;23;24;28;32;42"); !ok || info.Name != "Windows ConPTY" || info.Profile != colorprofile.TrueColor {
		t.Errorf("LookupDA1(ConPTY) = %+v, %v", info, ok)
	}
	for _, params := range []string{"6", "65;4;6;18;22", ""} {
		if info, ok := LookupDA1(params); ok {
			t.Errorf("LookupDA1(%q) = %+v; want no match", params, info)
		}
	}
}

func TestParseReportsBadEntries(t *testing.T) {
	data := []byte(`
[[terminal]]
name  = "Bad"
match = '('
color = "truecolor"

[[terminal]]
name  = "Odd"
match = '^Odd'
color = "rainbow"

[[terminal]]
name  = "Empty"
color = "truecolor"

[[terminal]]
name  = "Good"
match = '^Good'
color = "256"
`)
	list, err := parse(data)
	if err == nil {
		t.Fatal("parse: want error for bad entries")
	}
	if len(list) != 1 || list[0].Name != "Good" {
		t.Fatalf("parse kept %+v; want only Good", list)
	}
}

func TestHasTrueColorCap(t *testing.T) {
	for reply, want := range map[string]bool{
		"RGB":           true,
		"RGB=8/8/8":     true,
		"Tc":            true,
		"colors=256;Tc": true,
		"colors=256":    false,
		"":              false,
	} {
		if got := HasTrueColorCap(reply); got != want {
			t.Errorf("HasTrueColorCap(%q) = %v; want %v", reply, got, want)
		}
	}
}

func TestUpgrade(t *testing.T) {
	tests := []struct {
		base, p, want colorprofile.Profile
	}{
		{colorprofile.ANSI256, colorprofile.TrueColor, colorprofile.TrueColor},
		{colorprofile.ANSI, colorprofile.ANSI256, colorprofile.ANSI256},
		{colorprofile.TrueColor, colorprofile.ANSI256, colorprofile.TrueColor},
		{colorprofile.ASCII, colorprofile.TrueColor, colorprofile.ASCII},
		{colorprofile.NoTTY, colorprofile.TrueColor, colorprofile.NoTTY},
		{colorprofile.ANSI256, colorprofile.Unknown, colorprofile.ANSI256},
	}
	for _, tt := range tests {
		if got := Upgrade(tt.base, tt.p); got != tt.want {
			t.Errorf("Upgrade(%v, %v) = %v; want %v", tt.base, tt.p, got, tt.want)
		}
	}
}

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"390", "282", 1},
		{"0.35.2", "0.35.2", 0},
		{"1.2", "1.10", -1},
		{"1.2", "1.2.0", 0},
		{"", "1", -1},
	}
	for _, tt := range tests {
		if got := compareVersions(tt.a, tt.b); got != tt.want {
			t.Errorf("compareVersions(%q, %q) = %d; want %d", tt.a, tt.b, got, tt.want)
		}
	}
}
