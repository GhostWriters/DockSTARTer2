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
