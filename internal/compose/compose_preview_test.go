package compose

import (
	"testing"

	"github.com/compose-spec/compose-go/v2/types"
)

func TestMergeDependsOn(t *testing.T) {
	services := types.Services{}
	template := []byte(`
services:
  app:
    image: example/app
    depends_on:
      - db
      - cache
  db:
    image: example/db
`)
	override := []byte(`
services:
  app:
    depends_on:
      db:
        condition: service_healthy
        restart: true
      search:
        condition: service_started
        required: false
`)
	if err := mergeDependsOn(services, template); err != nil {
		t.Fatal(err)
	}
	if err := mergeDependsOn(services, override); err != nil {
		t.Fatal(err)
	}

	app := services["app"].DependsOn
	want := types.DependsOnConfig{
		"db":     {Condition: types.ServiceConditionHealthy, Restart: true, Required: true},
		"cache":  {Condition: types.ServiceConditionStarted, Required: true},
		"search": {Condition: types.ServiceConditionStarted, Required: false},
	}
	if len(app) != len(want) {
		t.Fatalf("app depends_on = %+v, want %+v", app, want)
	}
	for name, dep := range want {
		if app[name].Condition != dep.Condition || app[name].Restart != dep.Restart || app[name].Required != dep.Required {
			t.Errorf("%s = %+v, want %+v", name, app[name], dep)
		}
	}
	if _, ok := services["db"]; !ok {
		t.Errorf("db missing: every service is listed, with or without depends_on")
	}
}
