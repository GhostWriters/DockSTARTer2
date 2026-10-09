package config

import "testing"

func TestValidFrameAncestor(t *testing.T) {
	for site, ok := range map[string]bool{
		"https://organizr.example.com":   true,
		"https://*.example.net":          true,
		"https://dash.example.com:8443":  true,
		"":                               false,
		"   ":                            false,
		"https://a.com,https://b.com":    false,
		"https://a.com https://b.com":    false,
		"https://a.com; script-src *":    false,
		" https://organizr.example.com":  false,
		"https://organizr.example.com\n": false,
	} {
		if err := ValidFrameAncestor(site); (err == nil) != ok {
			t.Errorf("ValidFrameAncestor(%q) error = %v; want ok = %v", site, err, ok)
		}
	}
}
