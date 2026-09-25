package commands

import "testing"

// TestParseTintArgCounts checks --tint parses with no args (status) and with
// a ref plus optional type/element args.
func TestParseTintArgCounts(t *testing.T) {
	for _, args := range [][]string{
		{"--tint"},
		{"--tint", "embedded:ansi"},
		{"--tint", "embedded:ansi", "web"},
		{"--tint", "embedded:ansi", "web", "menu:"},
		{"--theme-tint", "web"},
	} {
		groups, err := Parse(args)
		if err != nil {
			t.Errorf("Parse(%v): %v", args, err)
			continue
		}
		if len(groups) != 1 || len(groups[0].Args) != len(args)-1 {
			t.Errorf("Parse(%v) = %+v, want one group with %d args", args, groups, len(args)-1)
		}
	}
}

// TestParseTintRejectsExplicitRemoteCLI checks naming the cli element with a
// connection type other than local is still rejected.
func TestParseTintRejectsExplicitRemoteCLI(t *testing.T) {
	if _, err := Parse([]string{"--tint", "embedded:ansi", "web", "cli:"}); err == nil {
		t.Error("--tint embedded:ansi web cli: parsed without error")
	}
}
