package compose

import (
	"context"
	"strings"
	"testing"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/compose/v5/pkg/api"

	"DockSTARTer2/internal/dockerlayout"
)

func TestModelDecls(t *testing.T) {
	got := modelDecls(&types.Project{Models: types.Models{
		"llm":   {Model: "ai/smollm2"},
		"embed": {Name: "embedder", Model: "ai/mxbai-embed-large"},
	}})
	if got["llm"] != "ai/smollm2" || got["embedder"] != "ai/mxbai-embed-large" || len(got) != 2 {
		t.Errorf("modelDecls = %v, want llm by its key and embedder by its name", got)
	}
}

func TestServiceModelDecls(t *testing.T) {
	got := serviceModelDecls(&types.Project{
		Services: types.Services{"app": {Name: "app", Models: map[string]*types.ServiceModelConfig{
			"creative_model": {EndpointVariable: "CREATIVE_URL", ModelVariable: "CREATIVE_MODEL"},
			"my-embed":       nil,
		}}},
		Models: types.Models{"my-embed": {Name: "embedder", Model: "ai/mxbai"}},
	})["app"]
	want := []serviceModel{
		{name: "creative_model", urlVar: "CREATIVE_URL", modelVar: "CREATIVE_MODEL"},
		{name: "embedder", urlVar: "MY_EMBED_URL", modelVar: "MY_EMBED_MODEL"},
	}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("serviceModelDecls = %+v, want %+v", got, want)
	}
}

func TestServiceModelLines(t *testing.T) {
	p := newGroupsTestProcessor("up")
	p.models = map[string]string{"llm": "ai/smollm2"}
	p.serviceModels = map[string][]serviceModel{"web": {{name: "llm", urlVar: "LLM_URL", modelVar: "LLM_MODEL"}}}
	if lines := plainLines(p.modelLines("web", 160, false)); len(lines) != 1 || !strings.Contains(lines[0], "Pending") ||
		!strings.Contains(lines[0], "model: llm") {
		t.Errorf("without -v = %q, want only web's model line, Pending", lines)
	}
	p.On(api.Resource{ID: "llm", Status: api.Done, Text: api.StatusConfigured})
	lines := plainLines(p.modelLines("web", 160, true))
	if len(lines) != 3 || !strings.Contains(lines[0], "Using") || !strings.Contains(lines[1], "endpoint_var: LLM_URL") ||
		!strings.Contains(lines[2], "model_var:    LLM_MODEL") {
		t.Errorf("-v = %q, want the model line Using, then its variables aligned", lines)
	}
	if lines := p.modelLines("db", 160, true); lines != nil {
		t.Errorf("db = %q, want none: it uses no model", lines)
	}
	p.command = "stop"
	if lines := p.modelLines("web", 160, true); lines != nil {
		t.Errorf("stop = %q, want none: teardown doesn't touch models", lines)
	}
}

func TestModelSettingsChange(t *testing.T) {
	p := newGroupsTestProcessor("up")
	p.On(containerEvent("db", api.StatusRunning, api.Done))
	p.On(containerEvent("web", api.StatusRunning, api.Done))
	p.models = map[string]string{"llm": "ai/smollm2", "embed": "ai/mxbai", "chat": "ai/chat"}
	p.modelSettings = map[string]modelSettings{
		"llm":   {contextSize: 4096, flags: []string{"--temp", "1"}},
		"embed": {contextSize: 2048},
		"chat":  {contextSize: 1024},
	}
	// llm's settings changed; embed's match; chat's couldn't be read.
	p.modelSettingsBefore = map[string]modelSettings{"llm": {contextSize: 2048}, "embed": {contextSize: 2048}}
	for _, name := range []string{"llm", "embed", "chat"} {
		p.On(api.Resource{ID: name, Status: api.Done, Text: api.StatusConfigured})
	}
	p.finished, p.succeeded = true, true
	got := renderPlain(p, 0)
	changed, unchanged := strings.Index(got, " changed ─"), strings.Index(got, "unchanged ─")
	llm := strings.Index(got, "Configured     1 model: llm")
	if changed < 0 || llm < changed || llm > unchanged || !strings.Contains(got, "Existing       2 models: chat, embed") {
		t.Errorf("display =\n%s\nwant llm Configured under changed, chat and embed Existing under unchanged", got)
	}
}

func TestFailedModelPullStaysFailed(t *testing.T) {
	p := newGroupsTestProcessor("up")
	p.models = map[string]string{"llm": "ai/nosuchmodel"}
	// compose's events for a failed pull: the error, then Pulled anyway.
	p.On(api.Resource{ID: "llm", Status: api.Working, Text: api.StatusPulling})
	p.On(api.Resource{ID: "llm", Status: api.Error, Text: "exit status 1"})
	p.On(api.Resource{ID: "llm", Status: api.Working, Text: api.StatusPulled})
	p.finished = true
	if got := renderPlain(p, 0); !strings.Contains(got, "× Error            llm:") || !strings.Contains(got, "× Error          1 model:") {
		t.Errorf("display =\n%s\nwant llm and its model heading failed", got)
	}
}

func TestFailModelsNamesTheCause(t *testing.T) {
	p := newGroupsTestProcessor("up")
	p.models = map[string]string{"llm": "ai/smollm2"}
	p.serviceModels = map[string][]serviceModel{"web": {{name: "llm", urlVar: "LLM_URL", modelVar: "LLM_MODEL"}}}
	const problem = "'models' support requires Docker Model plugin"
	p.failModels(problem)
	p.finished = true
	got := renderPlain(p, 0)
	// web keeps its row, its model line showing the failure; db, with nothing to show,
	// folds as Interrupted.
	if !strings.Contains(got, "× Error            llm:") || !strings.Contains(got, problem) || !strings.Contains(got, "× Error              model: llm") ||
		!strings.Contains(got, "⚠ Interrupted      web:") || !strings.Contains(got, "⚠ Interrupted    1 service: db") {
		t.Errorf("display =\n%s\nwant llm failed with compose's message, web's row with its model line Error, db folded", got)
	}
}

func TestModelLineLeavesServiceUnchanged(t *testing.T) {
	p := newGroupsTestProcessor("up")
	p.models = map[string]string{"llm": "ai/smollm2"}
	p.serviceModels = map[string][]serviceModel{"web": {{name: "llm", urlVar: "LLM_URL", modelVar: "LLM_MODEL"}}}
	p.On(containerEvent("db", api.StatusRunning, api.Done))
	p.On(containerEvent("web", api.StatusRunning, api.Done))
	p.On(api.Resource{ID: "llm", Status: api.Done, Text: api.StatusConfigured})
	p.finished, p.succeeded = true, true
	if got := renderPlain(p, 0); !strings.Contains(got, "2 services: db, web") {
		t.Errorf("display =\n%s\nwant web unchanged: its model line isn't a change", got)
	}
}

func TestModelsGroup(t *testing.T) {
	p := newGroupsTestProcessor("up")
	p.On(containerEvent("db", api.StatusRunning, api.Done))
	p.On(containerEvent("web", api.StatusRunning, api.Done))
	p.models = map[string]string{"llm": "ai/smollm2", "embed": "ai/mxbai"}
	if got := renderPlain(p, 0); !strings.Contains(got, "Pending        2 models: embed, llm") || !strings.Contains(got, ", 2 models") {
		t.Errorf("display before compose reports =\n%s\nwant both models Pending and counted in the summary", got)
	}
	// llm is pulled, then configured: a change, its model shown. embed is only configured,
	// as every model is on every run: unchanged.
	for _, e := range []api.Resource{
		{ID: "llm", Status: api.Working, Text: api.StatusPulling},
		{ID: "llm", Status: api.Working, Text: api.StatusPulled},
		{ID: "llm", Status: api.Working, Text: api.StatusConfiguring},
		{ID: "llm", Status: api.Done, Text: api.StatusConfigured},
		{ID: "embed", Status: api.Working, Text: api.StatusConfiguring},
		{ID: "embed", Status: api.Done, Text: api.StatusConfigured},
	} {
		p.On(e)
	}
	p.finished, p.succeeded = true, true
	got := renderPlain(p, 0)
	changed, unchanged := strings.Index(got, " changed ─"), strings.Index(got, "unchanged ─")
	llm := strings.Index(got, "Pulled         1 model: llm")
	if changed < 0 || llm < changed || llm > unchanged || !strings.Contains(got, "model: ai/smollm2") ||
		!strings.Contains(got, "Existing       1 model: embed") || strings.Contains(got, "Unknown") {
		t.Errorf("display =\n%s\nwant llm Pulled with its model under changed, embed Existing under unchanged", got)
	}
}

func TestOrphansGroup(t *testing.T) {
	p := newGroupsTestProcessor("up")
	p.On(containerEvent("db", api.StatusRunning, api.Done))
	p.On(containerEvent("web", api.StatusRunning, api.Done))
	p.On(api.Resource{ID: "Container autobrr", Status: api.Working, Text: api.StatusStopping})
	p.On(api.Resource{ID: "Container autobrr", Status: api.Done, Text: api.StatusRemoved})
	p.finished, p.succeeded = true, true
	got := renderPlain(p, 0)
	if !strings.Contains(got, "1 orphan:") || !strings.Contains(got, "Removed        1 orphan: autobrr") ||
		strings.Contains(got, "Unknown") || !strings.Contains(got, ", 1 orphan") || !strings.Contains(got, "2 services: db, web") {
		t.Errorf("display =\n%s\nwant autobrr Removed under changed as 1 orphan, not a service", got)
	}
}

// newContainerVerbProcessor returns a processor in container mode for docker verb on
// names, each in its state before the run (preRun), as NewContainerVerbDisplay sets it up.
func newContainerVerbProcessor(verb string, names []string, preRun map[string]string, detail map[string][2]string) *consoleEventProcessor {
	p := NewConsoleEventProcessor(context.Background(), nil, verb, map[string][]string{}, nil, map[string]string{}, "proj", false, 0, true, nil, nil).(*consoleEventProcessor)
	p.containerVerb = "docker " + verb
	p.containers = names
	p.containerPreRun = preRun
	p.containerDetail = detail
	return p
}

func TestContainerVerbRestartFolds(t *testing.T) {
	names := []string{"portainer", "radarr", "sonarr"}
	p := newContainerVerbProcessor("restart", names, map[string]string{"portainer": "running", "radarr": "running", "sonarr": "running"}, nil)
	for _, n := range names {
		p.On(api.Resource{ID: "Container " + n, Status: api.Working, Text: api.StatusRestarting})
	}
	if got := renderPlain(p, 0); !strings.Contains(got, "Restarting     3 containers: portainer, radarr, sonarr") {
		t.Errorf("display while restarting =\n%s\nwant all three on one Restarting line", got)
	}
	p.On(api.Resource{ID: "Container radarr", Status: api.Done, Text: api.StatusRestarted})
	p.On(api.Resource{ID: "Container sonarr", Status: api.Done, Text: api.StatusRestarted})
	if got := renderPlain(p, 0); !strings.Contains(got, "Restarted        2 containers: radarr, sonarr") || !strings.Contains(got, "portainer:") {
		t.Errorf("display with two done =\n%s\nwant radarr and sonarr folded Restarted, portainer still on its own row", got)
	}
	p.On(api.Resource{ID: "Container portainer", Status: api.Done, Text: api.StatusRestarted})
	p.finished, p.succeeded = true, true
	got := renderPlain(p, 0)
	if !strings.Contains(got, "docker restart: 3 containers") || !strings.Contains(got, "Restarted      3 containers: portainer, radarr, sonarr") ||
		strings.Contains(got, "orphan") {
		t.Errorf("display =\n%s\nwant the docker restart heading and all three folded Restarted", got)
	}
}

func TestContainerVerbStopLeavesStoppedAlone(t *testing.T) {
	p := newContainerVerbProcessor("stop", []string{"radarr", "wizarr"}, map[string]string{"radarr": "running", "wizarr": "stopped"}, nil)
	p.On(api.Resource{ID: "Container radarr", Status: api.Working, Text: api.StatusStopping})
	p.On(api.Resource{ID: "Container wizarr", Status: api.Working, Text: api.StatusStopping})
	p.On(api.Resource{ID: "Container radarr", Status: api.Done, Text: api.StatusStopped})
	p.On(api.Resource{ID: "Container wizarr", Status: api.Done, Text: api.StatusStopped})
	p.finished, p.succeeded = true, true
	got := renderPlain(p, 0)
	changed, unchanged := strings.Index(got, " changed ─"), strings.Index(got, "unchanged ─")
	stopped, notRunning := strings.Index(got, "Stopped        1 container: radarr"), strings.Index(got, "Not Running    1 container: wizarr")
	if changed < 0 || stopped < changed || stopped > unchanged || notRunning < unchanged {
		t.Errorf("display =\n%s\nwant radarr Stopped under changed, wizarr Not Running under unchanged", got)
	}
}

func TestContainerVerbFailureAndDetail(t *testing.T) {
	detail := map[string][2]string{"radarr": {"service", "radarr"}, "portainer": {"project", "tools"}}
	p := newContainerVerbProcessor("restart", []string{"portainer", "radarr"}, map[string]string{"portainer": "running", "radarr": "running"}, detail)
	p.On(api.Resource{ID: "Container radarr", Status: api.Done, Text: api.StatusRestarted})
	p.On(api.Resource{ID: "Container portainer", Status: api.Error, Text: "No such container: portainer"})
	p.finished, p.succeeded = true, false
	got := renderPlain(p, 0)
	if !strings.Contains(got, "Error            portainer:") || !strings.Contains(got, "No such container: portainer") {
		t.Errorf("display =\n%s\nwant portainer failed with its message", got)
	}
	if got := renderPlain(p, dockerlayout.VerboseDetails); !strings.Contains(got, "service: radarr") || !strings.Contains(got, "project: tools") {
		t.Errorf("-v display =\n%s\nwant each row naming what runs it", got)
	}
}

func TestServiceModelLineKeepsAProblemsWord(t *testing.T) {
	p := newGroupsTestProcessor("up")
	p.models = map[string]string{"llm": "ai/smollm2"}
	p.serviceModels = map[string][]serviceModel{"web": {{name: "llm", urlVar: "LLM_URL", modelVar: "LLM_MODEL"}}}
	p.On(api.Resource{ID: "llm", Status: api.Error, Text: "pull failed"})
	if lines := plainLines(p.modelLines("web", 160, false)); len(lines) != 1 || strings.Contains(lines[0], "Using") || !strings.Contains(lines[0], "model: llm") {
		t.Errorf("model line = %q, want the failed model's own status, not Using", lines)
	}
}
