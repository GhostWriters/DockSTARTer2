package serve

import (
	"os"
	"slices"
	"testing"

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
	if !slices.Contains(hosts, "ds2.example.com") {
		t.Errorf("%v is missing ds2.example.com", hosts)
	}
	if hostname, err := os.Hostname(); err == nil && !slices.Contains(hosts, hostname) {
		t.Errorf("%v is missing the hostname %q", hosts, hostname)
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
	for _, host := range []string{"ds2.example.com", "dstest", "*", "192.168.1.5"} {
		if err := config.ValidAllowedHost(host); err != nil {
			t.Errorf("%q: %v", host, err)
		}
	}
	for _, host := range []string{"", " ", "a.com b.com", "a.com,b.com", "https://ds2.example.com"} {
		if config.ValidAllowedHost(host) == nil {
			t.Errorf("%q was accepted", host)
		}
	}
}
