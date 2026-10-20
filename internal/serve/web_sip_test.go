package serve

import (
	"os"
	"slices"
	"testing"
	"time"

	"DockSTARTer2/internal/config"
)

// TestWebAllowedHosts checks the Host check applies only with plain HTTP, no
// password and no "*", and then lists the configured names and the hostname.
func TestWebAllowedHosts(t *testing.T) {
	var cfg config.ServerConfig
	cfg.Web.TLS = "none"
	cfg.Auth.Mode = "none"
	cfg.Web.AllowedHosts = []string{"ds2.example.com"}

	hosts, ok := webAllowedHosts(cfg)
	if !ok {
		t.Fatal("plain HTTP, no password: no check")
	}
	want := []string{"ds2.example.com"}
	if hostname, err := os.Hostname(); err == nil && hostname != "" {
		want = append(want, hostname)
	}
	if !slices.Equal(hosts, want) {
		t.Errorf("got %v, want %v", hosts, want)
	}

	for name, change := range map[string]func(*config.ServerConfig){
		"self-signed TLS": func(c *config.ServerConfig) { c.Web.TLS = "self-signed" },
		"password auth":   func(c *config.ServerConfig) { c.Auth.Mode = "password" },
		"*":               func(c *config.ServerConfig) { c.Web.AllowedHosts = []string{"*"} },
	} {
		c := cfg
		change(&c)
		if _, ok := webAllowedHosts(c); ok {
			t.Errorf("%s: the check applies", name)
		}
	}
}

func TestWebHostAllowed(t *testing.T) {
	hosts := []string{"ds2.example.com", "dstest"}
	for _, h := range []string{"10.10.0.215:40080", "[::1]:40080", "localhost:40080", "app.localhost",
		"ds2.example.com", "DS2.Example.com.:443", "dstest:40080"} {
		if !webHostAllowed(normalizeHost(h), hosts) {
			t.Errorf("%q was refused", h)
		}
	}
	for _, h := range []string{"dstest.lan:40080", "evil.example.com", "ds2.example.com.evil.com", ""} {
		if webHostAllowed(normalizeHost(h), hosts) {
			t.Errorf("%q was allowed", h)
		}
	}
}

func TestValidAllowedHost(t *testing.T) {
	for _, host := range []string{"ds2.example.com", "dstest", "*", "192.168.1.5", "my_host.lan", "dstest.lan."} {
		if err := config.ValidAllowedHost(host); err != nil {
			t.Errorf("%q: %v", host, err)
		}
	}
	for _, host := range []string{"", " ", "a.com b.com", "a.com,b.com", "https://ds2.example.com",
		"https:foo", "user@host", "host?x", "host:40080", "-host", "host-", "a..b", ".host", "*.example.com"} {
		if config.ValidAllowedHost(host) == nil {
			t.Errorf("%q was accepted", host)
		}
	}
}

// TestLogLimiter checks the first event logs at once, later ones at most
// once per interval, with the count of those skipped in between.
func TestLogLimiter(t *testing.T) {
	var l logLimiter
	start := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	steps := []struct {
		at      time.Duration
		logIt   bool
		skipped int
	}{
		{0, true, 0},
		{10 * time.Second, false, 0},
		{20 * time.Second, false, 0},
		{time.Minute, true, 2},
		{time.Minute + time.Second, false, 0},
		{3 * time.Minute, true, 1},
	}
	for _, s := range steps {
		logIt, skipped := l.allow(start.Add(s.at), time.Minute)
		if logIt != s.logIt || skipped != s.skipped {
			t.Errorf("at %v: got (%v, %d), want (%v, %d)", s.at, logIt, skipped, s.logIt, s.skipped)
		}
	}
}
