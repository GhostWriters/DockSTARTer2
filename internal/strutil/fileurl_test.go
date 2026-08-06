package strutil

import (
	"runtime"
	"testing"
)

func TestFileURL(t *testing.T) {
	tests := []struct {
		name        string
		path        string
		want        string
		windowsOnly bool
	}{
		{name: "posix absolute", path: "/home/clhatch/.config/compose/.env", want: "file:///home/clhatch/.config/compose/.env"},
		{name: "windows drive backslashes", path: `C:\Users\clhatch\.config\compose\.env`, want: "file:///C:/Users/clhatch/.config/compose/.env", windowsOnly: true},
		{name: "windows drive forward slashes", path: "C:/Users/clhatch/.config/compose/.env", want: "file:///C:/Users/clhatch/.config/compose/.env"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// filepath.ToSlash only converts "\" on Windows -- by design,
			// since a non-Windows binary only ever sees native "/" paths.
			if tt.windowsOnly && runtime.GOOS != "windows" {
				t.Skip("backslash separators are only converted on windows")
			}
			if got := FileURL(tt.path); got != tt.want {
				t.Errorf("FileURL(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}
