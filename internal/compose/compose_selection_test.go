package compose

import (
	"slices"
	"testing"

	"github.com/compose-spec/compose-go/v2/types"
)

func TestIncludesDependencies(t *testing.T) {
	bringUp := []string{"up", "update", "create", "start", "pull"}
	takeDown := []string{"stop", "kill", "pause", "unpause", "rm", "down", "restart"}
	for _, cmd := range bringUp {
		if !includesDependencies(cmd, DepsDefault) || !includesDependencies(cmd, DepsInclude) {
			t.Errorf("%s: want dependencies by default and with -D", cmd)
		}
		if includesDependencies(cmd, DepsNone) {
			t.Errorf("%s: want none with --no-deps", cmd)
		}
	}
	for _, cmd := range takeDown {
		if includesDependencies(cmd, DepsDefault) || includesDependencies(cmd, DepsNone) {
			t.Errorf("%s: want none by default and with --no-deps", cmd)
		}
		if !includesDependencies(cmd, DepsInclude) {
			t.Errorf("%s: want dependencies with -D", cmd)
		}
	}
}

func TestExtraServices(t *testing.T) {
	// app depends on vpn (restart: true) and db.
	project := &types.Project{Services: types.Services{
		"vpn": {Name: "vpn"},
		"db":  {Name: "db"},
		"app": {Name: "app", DependsOn: types.DependsOnConfig{
			"vpn": {Condition: types.ServiceConditionHealthy, Restart: true, Required: true},
			"db":  {Condition: types.ServiceConditionStarted, Required: true},
		}},
	}}
	for _, tc := range []struct {
		command                       string
		deps                          DepsMode
		names                         []string
		wantDependencies, wantDepends []string
	}{
		{"stop", DepsDefault, []string{"app"}, nil, nil},
		{"stop", DepsInclude, []string{"app"}, []string{"db", "vpn"}, nil},
		{"up", DepsDefault, []string{"app"}, []string{"db", "vpn"}, nil},
		{"up", DepsNone, []string{"app"}, nil, nil},
		{"restart", DepsDefault, []string{"vpn"}, nil, []string{"app"}},
		{"restart", DepsNone, []string{"vpn"}, nil, nil},
		{"restart", DepsInclude, []string{"app"}, []string{"db", "vpn"}, nil},
	} {
		deps, dependents := extraServices(project, tc.command, tc.deps, tc.names)
		if !slices.Equal(deps, tc.wantDependencies) || !slices.Equal(dependents, tc.wantDepends) {
			t.Errorf("%s %v %v: dependencies %v dependents %v, want %v and %v",
				tc.command, tc.deps, tc.names, deps, dependents, tc.wantDependencies, tc.wantDepends)
		}
	}
}

func TestWithRestartDependents(t *testing.T) {
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

	if got := sorted(withRestartDependents(project, []string{"vpn"})); !slices.Equal(got, []string{"app", "vpn", "worker"}) {
		t.Errorf("restart vpn = %v, want vpn, app and, through app, worker", got)
	}
	if got := sorted(withRestartDependents(project, []string{"db"})); !slices.Equal(got, []string{"db"}) {
		t.Errorf("restart db = %v, want db only: app's entry on db has no restart: true", got)
	}
}
