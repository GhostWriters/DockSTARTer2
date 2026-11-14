package compose

import (
	"slices"
	"testing"

	"github.com/compose-spec/compose-go/v2/types"
)

func TestIncludesDependencies(t *testing.T) {
	for _, cmd := range []string{"up", "update", "create", "start", "pull", "something-else"} {
		if !includesDependencies(cmd) {
			t.Errorf("%s: want dependencies included", cmd)
		}
	}
	for _, cmd := range []string{"stop", "kill", "pause", "unpause", "rm", "down", "restart"} {
		if includesDependencies(cmd) {
			t.Errorf("%s: want only the requested services", cmd)
		}
	}
}

func TestWithDependents(t *testing.T) {
	// app depends on vpn (restart: true) and db; worker depends on app (restart: true).
	project := &types.Project{Services: types.Services{
		"vpn": {Name: "vpn"},
		"db":  {Name: "db"},
		"app": {Name: "app", DependsOn: types.DependsOnConfig{
			"vpn": {Condition: types.ServiceConditionHealthy, Restart: true, Required: true},
			"db":  {Condition: types.ServiceConditionStarted, Required: true},
		}},
		"worker": {Name: "worker", DependsOn: types.DependsOnConfig{
			"app": {Condition: types.ServiceConditionStarted, Restart: true, Required: true},
		}},
	}}
	sorted := func(s []string) []string { slices.Sort(s); return s }

	if got := sorted(withDependents(project, []string{"vpn"}, true)); !slices.Equal(got, []string{"app", "vpn", "worker"}) {
		t.Errorf("restart vpn = %v, want vpn, app and, through app, worker", got)
	}
	if got := sorted(withDependents(project, []string{"db"}, true)); !slices.Equal(got, []string{"db"}) {
		t.Errorf("restart db = %v, want db only: app's entry on db has no restart: true", got)
	}
	if got := sorted(withDependents(project, []string{"db"}, false)); !slices.Equal(got, []string{"app", "db", "worker"}) {
		t.Errorf("down db = %v, want db and everything depending on it", got)
	}
}
