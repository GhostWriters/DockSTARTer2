package console

import "testing"

// A kind's styles derive from its hue: its names, final status and counts are the hue,
// its title and timer the bright version, bold.
func TestDockerKindStylesDeriveFromTheirHues(t *testing.T) {
	for name, tc := range map[string]struct{ got, want string }{
		"item name":    {Colors.App, "{{[-]}}{{[cyan::B]}}"},
		"user name":    {Colors.UserApp, "{{[-]}}{{[cyan]}}"},
		"network name": {Colors.IPAddress, "{{[-]}}{{[cyan]}}"},
		"volume name":  {Colors.Folder, "{{[-]}}{{[cyan::B]}}"},
		"final status": {Colors.DockerSection, "{{[-]}}{{[cyan::B]}}"},
		"title":        {Colors.DockerTitle, "{{[-]}}{{[bright-cyan::B]}}"},
		"image name":   {Colors.DockerImage, "{{[-]}}{{[magenta]}}"},
		"image title":  {Colors.DockerTitleImage, "{{[-]}}{{[bright-magenta::B]}}"},
		"version":      {Colors.Version, "{{[-]}}{{[cyan]}}"},
		"url":          {Colors.URL, "{{[-]}}{{[cyan::U]}}"},
	} {
		if tc.got != tc.want {
			t.Errorf("%s = %q, want %q", name, tc.got, tc.want)
		}
	}
}
