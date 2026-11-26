package commands

import "testing"

// -c status takes any number of apps.
func TestParseComposeStatus(t *testing.T) {
	for _, args := range [][]string{
		{"-c", "status"},
		{"-c", "status", "plex"},
		{"--compose", "status", "plex", "sonarr", "radarr"},
	} {
		groups, err := Parse(args)
		if err != nil {
			t.Errorf("Parse(%v): %v", args, err)
			continue
		}
		if len(groups) != 1 || len(groups[0].Args) != len(args)-1 || groups[0].Args[0] != "status" {
			t.Errorf("Parse(%v) = %+v, want one group with status and its apps", args, groups)
		}
	}
}

// -c status only reads container states, so it runs while the configuration is being
// edited; every other compose command takes the edit lock.
func TestGroupSessionLocked(t *testing.T) {
	for _, tc := range []struct {
		group CommandGroup
		want  bool
	}{
		{CommandGroup{Command: "-c", Args: []string{"status"}}, false},
		{CommandGroup{Command: "--compose", Args: []string{"status", "plex"}}, false},
		{CommandGroup{Command: "-c", Args: []string{"up"}}, true},
		{CommandGroup{Command: "-c"}, true},
		{CommandGroup{Command: "--add", Args: []string{"plex"}}, true},
		{CommandGroup{Command: "-V"}, false},
	} {
		if got := GroupSessionLocked(tc.group); got != tc.want {
			t.Errorf("GroupSessionLocked(%+v) = %v, want %v", tc.group, got, tc.want)
		}
	}
}
