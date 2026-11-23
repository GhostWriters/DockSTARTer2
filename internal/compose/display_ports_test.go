package compose

import (
	"strconv"
	"strings"
	"testing"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/compose/v5/pkg/api"
)

func TestParseBindError(t *testing.T) {
	for _, tc := range []struct {
		msg      string
		port     uint16
		proto    string
		hostHeld bool
		ok       bool
	}{
		{"driver failed programming external connectivity on endpoint web (abc): Bind for 0.0.0.0:18080 failed: port is already allocated", 18080, "", false, true},
		{"Bind for [::]:8443 failed: port is already allocated", 8443, "", false, true},
		{"driver failed programming external connectivity on endpoint api (abc): failed to bind host port 0.0.0.0:18081/tcp: address already in use", 18081, "tcp", true, true},
		{"failed to bind host port 0.0.0.0:53/udp: address already in use", 53, "udp", true, true},
		{"no such image", 0, "", false, false},
	} {
		port, proto, hostHeld, ok := parseBindError(tc.msg)
		if port != tc.port || proto != tc.proto || hostHeld != tc.hostHeld || ok != tc.ok {
			t.Errorf("parseBindError(%q) = %d %q %v %v; want %d %q %v %v", tc.msg, port, proto, hostHeld, ok, tc.port, tc.proto, tc.hostHeld, tc.ok)
		}
	}
}

func TestPublishedTarget(t *testing.T) {
	ports := []types.ServicePortConfig{
		{Target: 80, Published: "8080"},
		{Target: 53, Published: "5353", Protocol: "udp"},
		{Target: 8000, Published: "8400-8500", Protocol: "tcp"},
	}
	for _, tc := range []struct {
		port   uint16
		proto  string
		target uint32
		ok     bool
	}{
		{8080, "tcp", 80, true},
		{8080, "udp", 0, false},
		{5353, "udp", 53, true},
		{8450, "tcp", 8050, true},
		{9000, "tcp", 0, false},
	} {
		if target, ok := publishedTarget(ports, tc.port, tc.proto); target != tc.target || ok != tc.ok {
			t.Errorf("publishedTarget(%d/%s) = %d, %v; want %d, %v", tc.port, tc.proto, target, ok, tc.target, tc.ok)
		}
	}
}

func TestPortsLines(t *testing.T) {
	p := newGroupsTestProcessor("up")
	p.portConflict = &portConflict{
		failed:  portClaim{name: "web", service: true, target: 80},
		port:    8080,
		proto:   "tcp",
		holder:  &portClaim{name: "db", service: true, target: 8080},
		wanters: []portClaim{{name: "cache", service: true, target: 80}},
	}
	web := strings.Join(plainLines(p.portsLines("web", 160)), "\n")
	if !strings.Contains(web, "× Conflict") || !strings.Contains(web, "ports: 8080:80/tcp (held by db; also wanted by cache, not running)") {
		t.Errorf("web's line = %q, want the failure naming db and cache", web)
	}
	db := strings.Join(plainLines(p.portsLines("db", 160)), "\n")
	if !strings.Contains(db, "⚠ Conflict") || !strings.Contains(db, "8080:8080/tcp (wanted by web)") {
		t.Errorf("db's line = %q, want a warning that web wants its port", db)
	}
	cache := strings.Join(plainLines(p.portsLines("cache", 160)), "\n")
	if !strings.Contains(cache, "⚠ Conflict") || !strings.Contains(cache, "8080:80/tcp (held by db; wanted by web)") {
		t.Errorf("cache's line = %q, want a warning naming db and web", cache)
	}

	// The host holding the port; 53 names the usual program.
	p.portConflict = &portConflict{failed: portClaim{name: "web", service: true, target: 53}, port: 53, proto: "udp", hostHeld: true}
	if got := strings.Join(plainLines(p.portsLines("web", 160)), "\n"); !strings.Contains(got, "held by a program on the host, or a container using host networking, often systemd-resolved") {
		t.Errorf("web's line = %q, want the host named", got)
	}
	if lines := p.portsLines("db", 160); lines != nil {
		t.Errorf("db's lines = %q, want none", lines)
	}
}

func TestPortRanges(t *testing.T) {
	// Compose expands 8000-8100:8000-8100 into one entry per port.
	var expanded []types.ServicePortConfig
	for n := 8000; n <= 8100; n++ {
		expanded = append(expanded, types.ServicePortConfig{Target: uint32(n), Published: strconv.Itoa(n), Protocol: "tcp"})
	}
	expanded = append(expanded, types.ServicePortConfig{Target: 80, Published: "8200"})
	claim, ok := serviceClaim("azuracast", expanded, 8080, "tcp")
	if !ok || claim.target != 8080 || claim.rangeText() != "8000-8100" {
		t.Errorf("serviceClaim(8080) = %+v, %v; want target 8080 in 8000-8100", claim, ok)
	}
	if claim, _ := serviceClaim("azuracast", expanded, 8200, "tcp"); claim.rangeText() != "" {
		t.Errorf("serviceClaim(8200) range = %q, want a single port", claim.rangeText())
	}
	// A long-form range stays one entry.
	long := []types.ServicePortConfig{{Target: 8400, Published: "8400-8500", Protocol: "tcp"}}
	if claim, ok := serviceClaim("azuracast", long, 8450, "tcp"); !ok || claim.target != 8450 || claim.rangeText() != "8400-8500" {
		t.Errorf("serviceClaim(long 8450) = %+v, %v; want target 8450 in 8400-8500", claim, ok)
	}

	p := newGroupsTestProcessor("up")
	p.portConflict = &portConflict{
		failed: portClaim{name: "web", service: true, target: 8000, lo: 8000, hi: 8000},
		port:   8000, proto: "tcp",
		holder: &portClaim{name: "db", service: true, target: 8000, lo: 8000, hi: 8100},
	}
	if got := strings.Join(plainLines(p.portsLines("web", 160)), "\n"); !strings.Contains(got, "8000:8000/tcp (held by db, ports 8000-8100)") {
		t.Errorf("web's line = %q, want db's range named", got)
	}
	if got := strings.Join(plainLines(p.portsLines("db", 160)), "\n"); !strings.Contains(got, "8000:8000/tcp, part of 8000-8100 (wanted by web)") {
		t.Errorf("db's line = %q, want its own range", got)
	}

	// Both ranges: the line names every port they share, not only the first.
	p.portConflict.failed = portClaim{name: "web", service: true, target: 8050, lo: 8050, hi: 8060}
	p.portConflict.port = 8050
	if got := strings.Join(plainLines(p.portsLines("web", 160)), "\n"); !strings.Contains(got, "(held by db, ports 8000-8100; the ranges overlap on 8050-8060)") {
		t.Errorf("web's line = %q, want the overlap named", got)
	}
}

func TestPortHolderIsMarkedAsTheCause(t *testing.T) {
	p := newGroupsTestProcessor("up")
	p.On(containerEvent("db", api.StatusRunning, api.Done))
	p.On(containerEvent("web", api.StatusRunning, api.Done))
	p.portConflict = &portConflict{
		failed: portClaim{name: "cache", service: true, target: 80}, port: 80, proto: "tcp",
		holder:  &portClaim{name: "db", service: true, target: 80},
		wanters: []portClaim{{name: "web", service: true, target: 80}},
	}
	if icon := p.serviceIcon(p.tasks["db"], api.Done); !strings.Contains(icon, "DockerMarkerCause") {
		t.Errorf("db's icon = %q, want the cause marker: it holds the port", icon)
	}
	if lines := p.portsLines("db", 160); len(lines) == 0 || !strings.Contains(lines[0], "DockerMarkerCause") {
		t.Errorf("db's ports line = %q, want the cause marker", lines)
	}
	if icon := p.serviceIcon(p.tasks["web"], api.Done); !strings.Contains(icon, "DockerMarkerWarn") {
		t.Errorf("web's icon = %q, want a warning: it wants the port too", icon)
	}
}

func TestPortConflictShowsInTheDisplay(t *testing.T) {
	p := newGroupsTestProcessor("up")
	p.On(api.Resource{ID: "Container web", Status: api.Error, Text: "Bind for 0.0.0.0:8080 failed: port is already allocated"})
	p.portConflict = &portConflict{failed: portClaim{name: "web", service: true, target: 80}, port: 8080, proto: "tcp", holder: &portClaim{name: "portainer", target: 9000}}
	p.finished = true
	if got := renderPlain(p, 0); !strings.Contains(got, "ports: 8080:80/tcp (held by portainer)") {
		t.Errorf("display =\n%s\nwant web's ports line naming portainer", got)
	}
}

func TestServiceSpecsAndHostIPs(t *testing.T) {
	specs := serviceSpecs([]types.ServicePortConfig{
		{Target: 80, Published: "8080"},
		{Target: 8400, Published: "8400-8402", Protocol: "udp", HostIP: "127.0.0.1"},
	})
	if len(specs) != 4 || specs[0] != (portSpec{port: 8080, proto: "tcp", target: 80}) ||
		specs[3] != (portSpec{ip: "127.0.0.1", port: 8402, proto: "udp", target: 8402}) {
		t.Errorf("serviceSpecs = %+v", specs)
	}
	for _, tc := range []struct {
		a, b string
		want bool
	}{
		{"", "127.0.0.1", true},
		{"0.0.0.0", "192.168.1.5", true},
		{"::", "127.0.0.1", true},
		{"127.0.0.1", "127.0.0.1", true},
		{"127.0.0.1", "192.168.1.5", false},
	} {
		if got := hostIPsOverlap(tc.a, tc.b); got != tc.want {
			t.Errorf("hostIPsOverlap(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestPossibleConflictResolves(t *testing.T) {
	p := newGroupsTestProcessor("up")
	p.predicted = map[string][]predictedConflict{
		"web": {{lo: 80, hi: 80, proto: "tcp", target: 80, other: "azuracast"}},
		"db":  {{lo: 8100, hi: 8110, proto: "tcp", other: "web", inRun: true}},
	}
	if got := strings.Join(plainLines(p.predictedLines("web", 160)), "\n"); !strings.Contains(got, "? Conflict ") || !strings.Contains(got, "ports: 80:80/tcp (held by azuracast)") {
		t.Errorf("web before compose reports = %q, want an unconfirmed ports line", got)
	}
	if got := strings.Join(plainLines(p.predictedLines("db", 160)), "\n"); !strings.Contains(got, "8100-8110/tcp (also wanted by web)") {
		t.Errorf("db's line = %q, want the range another service of the run wants", got)
	}

	// db starts fine: a false alarm, its line goes.
	p.On(containerEvent("db", api.StatusStarted, api.Done))
	if lines := p.predictedLines("db", 160); lines != nil {
		t.Errorf("db after starting = %q, want no line", lines)
	}

	// web fails on the port: the confirmed conflict's line replaces the possible one.
	p.On(api.Resource{ID: "Container web", Status: api.Error, Text: "Bind for 0.0.0.0:80 failed: port is already allocated"})
	p.portConflict = &portConflict{failed: portClaim{name: "web", service: true, target: 80}, port: 80, proto: "tcp", holder: &portClaim{name: "azuracast", target: 80}}
	got := strings.Join(plainLines(p.portsLines("web", 160)), "\n")
	if !strings.Contains(got, "× Conflict") || strings.Contains(got, "? Conflict") {
		t.Errorf("web after failing = %q, want only the confirmed Conflict line", got)
	}
}

func TestPossibleConflictsGroupByPort(t *testing.T) {
	p := newGroupsTestProcessor("up")
	p.predicted = map[string][]predictedConflict{"swag": {
		{lo: 80, hi: 80, proto: "tcp", target: 80, other: "porthog"},
		{lo: 80, hi: 80, proto: "tcp", target: 80, other: "azuracast", inRun: true},
		{lo: 80, hi: 80, proto: "tcp", target: 80, other: "nginx", inRun: true},
		{lo: 443, hi: 443, proto: "tcp", target: 443, other: "nginx", inRun: true},
	}}
	got := strings.Join(plainLines(p.predictedLines("swag", 200)), "\n")
	if !strings.Contains(got, "80:80/tcp (held by porthog; also wanted by azuracast, nginx), 443:443/tcp (also wanted by nginx)") {
		t.Errorf("swag's line = %q, want one entry per port naming every claimant", got)
	}
}

func TestPossibleConflictStaysWhenNotReached(t *testing.T) {
	p := newGroupsTestProcessor("up")
	p.predicted = map[string][]predictedConflict{"web": {{lo: 443, hi: 443, proto: "tcp", target: 443, other: "swag"}}}
	p.finished = true // the run stopped before compose reached web
	if got := strings.Join(plainLines(p.predictedLines("web", 160)), "\n"); !strings.Contains(got, "? Conflict ") {
		t.Errorf("web not reached = %q, want its unconfirmed line kept", got)
	}
}

func TestPossibleConflictsFoldApart(t *testing.T) {
	p := newGroupsTestProcessor("up")
	p.preRun = map[string]string{"db": "running", "web": "running", "cache": "running"}
	p.imageServices["example/cache:1"] = []string{"cache"}
	p.imageOrder = append(p.imageOrder, "example/cache:1")
	p.imageMetaBefore["example/cache:1"] = imageMeta{}
	p.predicted = map[string][]predictedConflict{
		"web":   {{lo: 80, hi: 80, proto: "tcp", target: 80, other: "cache", inRun: true}},
		"cache": {{lo: 80, hi: 80, proto: "tcp", target: 80, other: "web", inRun: true}},
	}
	got := renderPlain(p, 0)
	if !strings.Contains(got, "1 service: db") || !strings.Contains(got, "2 services: cache, web") ||
		!strings.Contains(got, "? Conflict          ports: 80/tcp") || strings.Contains(got, "web:") {
		t.Errorf("display before compose reports =\n%s\nwant db folded alone, cache and web folded together with one unconfirmed line", got)
	}
	// All start fine: the unconfirmed line goes and they fold together.
	for _, svc := range []string{"db", "web", "cache"} {
		p.On(containerEvent(svc, api.StatusRunning, api.Done))
	}
	p.finished, p.succeeded = true, true
	if got := renderPlain(p, 0); !strings.Contains(got, "3 services: cache, db, web") || strings.Contains(got, "? Conflict") {
		t.Errorf("display after all run =\n%s\nwant all three folded, no unconfirmed line", got)
	}
}

func TestPortsLineStylesTheHostPortAndTheMessage(t *testing.T) {
	p := newGroupsTestProcessor("up")
	for _, style := range []string{"{{|DockerStatusWarn|}}", "{{|DockerStatusFail|}}"} {
		lines := p.portsLineAs(portConflictText, "I", style, "80:80/tcp (held by swag; wanted by nginx)", 200)
		if len(lines) != 1 {
			t.Fatalf("lines = %q, want one", lines)
		}
		for _, want := range []string{style + "80{{[-]}}", "{{[::D]}}:80/tcp {{[-]}}", style + "(held by swag; wanted by nginx){{[-]}}"} {
			if !strings.Contains(lines[0], want) {
				t.Errorf("line = %q, want %q: the host port and the message in the line's style, the rest dim", lines[0], want)
			}
		}
	}
	// A service holding the port, a warning marked as the cause: its port follows the
	// marker (red), its message the status (yellow).
	cause := p.portsLineAs(portConflictText, "{{|DockerMarkerCause|}}!{{[-]}}", "{{|DockerStatusWarn|}}", "80:80/tcp (wanted by nginx)", 200)
	for _, want := range []string{"{{|DockerMarkerCause|}}80{{[-]}}", "{{|DockerStatusWarn|}}(wanted by nginx){{[-]}}"} {
		if !strings.Contains(cause[0], want) {
			t.Errorf("line = %q, want %q", cause[0], want)
		}
	}
	// Wrapped, the message keeps its style on every line.
	lines := p.portsLineAs(portConflictText, "I", "{{|DockerStatusFail|}}", "80:80/tcp (held by a program on the host, or a container using host networking, often systemd-resolved)", 70)
	if len(lines) < 2 {
		t.Fatalf("lines = %q, want a wrapped line", lines)
	}
	for _, line := range lines[1:] {
		if !strings.Contains(line, "{{|DockerStatusFail|}}") {
			t.Errorf("wrapped line = %q, want the message still in the error style", line)
		}
	}
}
