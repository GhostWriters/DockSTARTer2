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

func TestVerboseLevelForms(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want int
	}{
		{[]string{"-v", "-c", "up"}, 1},
		{[]string{"-vv", "-c", "up"}, 2},
		{[]string{"-v", "-v", "-c", "up"}, 2},
		{[]string{"-v=0", "-c", "up"}, 0},
		{[]string{"--verbose=0", "-c", "up"}, 0},
		{[]string{"-vv", "-v=0", "-c", "up"}, 0},
		{[]string{"-v=4", "-c", "up"}, 4},
		{[]string{"-v:4", "-c", "up"}, 4},
		{[]string{"--verbose=3", "-c", "up"}, 3},
		{[]string{"--verbose:3", "-c", "up"}, 3},
		{[]string{"-v=4", "-v", "-c", "up"}, 5},
		{[]string{"-v", "-v=4", "-c", "up"}, 4},
		{[]string{"-xv=2", "-c", "up"}, 2},
		{[]string{"-vxc", "up"}, 1},
	} {
		groups, err := Parse(tc.args)
		if err != nil {
			t.Errorf("%v: %v", tc.args, err)
			continue
		}
		got := 0
		for _, flag := range groups[0].Flags {
			got, _ = ApplyVerbose(flag, got)
		}
		if got != tc.want {
			t.Errorf("%v: level %d, want %d", tc.args, got, tc.want)
		}
	}
	for _, args := range [][]string{{"-v=x", "-c", "up"}, {"-v=-1", "-c", "up"}, {"-v=", "-c", "up"}, {"-f=2", "-c", "up"}} {
		if _, err := Parse(args); err == nil {
			t.Errorf("%v: want an invalid option error", args)
		}
	}
}
