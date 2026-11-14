package compose

import (
	"slices"
	"testing"

	"github.com/compose-spec/compose-go/v2/types"
)

func TestIncludesDependenciesAndDependents(t *testing.T) {
	for _, cmd := range []string{"up", "update", "create", "start", "pull"} {
		for _, deps := range []DepsMode{DepsDefault, DepsInclude} {
			if !includesDependencies(cmd, deps) || includesDependents(cmd, deps) {
				t.Errorf("%s %v: want dependencies only", cmd, deps)
			}
		}
		if includesDependencies(cmd, DepsNone) {
			t.Errorf("%s --no-deps: want no dependencies", cmd)
		}
	}
	for _, cmd := range []string{"stop", "kill", "pause", "unpause", "rm", "down", "restart"} {
		for _, deps := range []DepsMode{DepsDefault, DepsInclude, DepsNone} {
			if includesDependencies(cmd, deps) {
				t.Errorf("%s %v: want no dependencies", cmd, deps)
			}
		}
		if !includesDependents(cmd, DepsInclude) || includesDependents(cmd, DepsDefault) || includesDependents(cmd, DepsNone) {
			t.Errorf("%s: want dependents only with -D", cmd)
		}
	}
}

func TestExtraServices(t *testing.T) {
	// app depends on vpn (restart: true) and db; front depends on app.
	project := &types.Project{Services: types.Services{
		"vpn": {Name: "vpn"},
		"db":  {Name: "db"},
		"app": {Name: "app", DependsOn: types.DependsOnConfig{
			"vpn": {Condition: types.ServiceConditionHealthy, Restart: true, Required: true},
			"db":  {Condition: types.ServiceConditionStarted, Required: true},
		}},
		"front": {Name: "front", DependsOn: types.DependsOnConfig{
			"app": {Condition: types.ServiceConditionStarted, Required: true},
		}},
	}}
	for _, tc := range []struct {
		command                       string
		deps                          DepsMode
		names                         []string
		wantDependencies, wantDepends []string
		wantRestartOnly               bool
	}{
		{"stop", DepsDefault, []string{"app"}, nil, nil, false},
		{"stop", DepsInclude, []string{"app"}, nil, []string{"front"}, false},
		{"stop", DepsInclude, []string{"vpn"}, nil, []string{"app", "front"}, false},
		{"up", DepsDefault, []string{"front"}, []string{"app", "db", "vpn"}, nil, false},
		{"up", DepsNone, []string{"front"}, nil, nil, false},
		{"restart", DepsDefault, []string{"vpn"}, nil, []string{"app"}, true},
		{"restart", DepsInclude, []string{"vpn"}, nil, []string{"app", "front"}, false},
		{"restart", DepsNone, []string{"vpn"}, nil, nil, false},
	} {
		deps, dependents, restartOnly := extraServices(project, tc.command, tc.deps, tc.names)
		if !slices.Equal(deps, tc.wantDependencies) || !slices.Equal(dependents, tc.wantDepends) || restartOnly != tc.wantRestartOnly {
			t.Errorf("%s %v %v: dependencies %v dependents %v restartOnly %v, want %v, %v, %v",
				tc.command, tc.deps, tc.names, deps, dependents, restartOnly, tc.wantDependencies, tc.wantDepends, tc.wantRestartOnly)
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
}
