package docker

import (
	"errors"
	"strings"
	"testing"

	"github.com/GhostWriters/semstyle"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"
	"github.com/moby/moby/api/types/image"

	"DockSTARTer2/internal/dockerlayout"
)

// prunePlain returns the prune report's lines as plain text.
func prunePlain(r PruneReport, imageServices map[string][]string, open bool) string {
	lines, _ := buildPruneLines(r, imageServices, false, open, 160)
	for i := range lines {
		lines[i] = ansi.Strip(lines[i])
	}
	return strings.Join(lines, "\n")
}

func TestPruneUnusedImagesListAsImages(t *testing.T) {
	r := PruneReport{ImagesDeleted: []image.DeleteResponse{
		{Untagged: "sha256:067bd9"}, {Deleted: "sha256:l1"}, {Deleted: "sha256:l2"},
		{Untagged: "sha256:cf4fe1"}, {Deleted: "sha256:l3"},
	}, SpaceReclaimed: 1 << 20}
	got := prunePlain(r, nil, false)
	for _, want := range []string{"✓ docker system prune: 2 images, 3 layers", "✓ Untagged    changed:", "✓ Untagged      2 unused images:", "✓ Untagged        image: sha256:067bd9 [2]",
		"✓ Untagged        image: sha256:cf4fe1 [1]", "Total reclaimed space:"} {
		if !strings.Contains(got, want) {
			t.Errorf("display =\n%s\nwant %q", got, want)
		}
	}
	if strings.Contains(got, "Unknown") || strings.Contains(got, "services:") {
		t.Errorf("display =\n%s\nwant no <Unknown> service rows", got)
	}
}

func TestPruneServicesNetworksAndVolumes(t *testing.T) {
	r := PruneReport{
		ImagesDeleted:     []image.DeleteResponse{{Untagged: "example/web:1"}, {Deleted: "sha256:l1"}},
		ContainersDeleted: []string{"c1"},
		ContainerInfo:     map[string]containerMeta{"c1": {service: "web", name: "web-1"}},
		NetworksDeleted:   []string{"net_a", "net_b"},
		VolumesDeleted:    []string{"vol_a"},
	}
	svcs := map[string][]string{"example/web:1": {"web"}}
	got := prunePlain(r, svcs, false)
	for _, want := range []string{"1 service:", "Removed         web:", "Deleted           container_name: web-1",
		"Untagged          image: example/web:1 [1]", "✓ Removed       2 networks: net_a, net_b",
		"✓ Removed       1 volume: vol_a"} {
		if !strings.Contains(got, want) {
			t.Errorf("display =\n%s\nwant %q", got, want)
		}
	}
	// -v: every row on its own.
	if got := prunePlain(r, svcs, true); !strings.Contains(got, "Removed         net_a:") || !strings.Contains(got, "Removed         net_b:") {
		t.Errorf("-v display =\n%s\nwant each network on its own row", got)
	}
}

func TestShortVolumeName(t *testing.T) {
	anon := "fb4482247887ec9ce5aff1cb55cb7bd067702854594604a185e0b8e7087a4a4f"
	for name, want := range map[string]string{anon: "fb4482247887", "compose_data": "compose_data", strings.ToUpper(anon): strings.ToUpper(anon)} {
		if got := shortVolumeName(name); got != want {
			t.Errorf("shortVolumeName(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestPruneInstancesFold(t *testing.T) {
	r := PruneReport{ImagesDeleted: []image.DeleteResponse{{Untagged: "example/sonarr:1"}, {Deleted: "sha256:l1"}}}
	got := prunePlain(r, map[string][]string{"example/sonarr:1": {"sonarr", "sonarr__4k"}}, false)
	if !strings.Contains(got, "Removed         2 services: sonarr, sonarr__4k") || strings.Count(got, "image: example/sonarr:1") != 1 {
		t.Errorf("display =\n%s\nwant both instances on one line, the image once", got)
	}
}

func TestPruneErrorMarksItsHeading(t *testing.T) {
	r := PruneReport{VolumesError: errors.New("permission denied"), NetworksDeleted: []string{"net_a"}}
	lines, errs := buildPruneLines(r, nil, false, false, 160)
	got := ansi.Strip(strings.Join(lines, "\n"))
	if !strings.Contains(got, "× Error       changed:") || !strings.Contains(got, "× Error         volumes:") ||
		!strings.Contains(got, "✓ Removed       1 network: net_a") || len(errs) != 1 {
		t.Errorf("display =\n%s\nerrs = %q\nwant changed: and volumes: failed, networks complete", got, errs)
	}
}

func TestPruneFoldsKindsOntoTheirHeadings(t *testing.T) {
	// Only networks and volumes, all Removed: changed: says Removed too.
	r := PruneReport{NetworksDeleted: []string{"net_a", "net_b"}, VolumesDeleted: []string{"vol_a"}}
	got := prunePlain(r, nil, false)
	for _, want := range []string{"✓ Removed     changed:", "✓ Removed       2 networks: net_a, net_b", "✓ Removed       1 volume: vol_a"} {
		if !strings.Contains(got, want) {
			t.Errorf("display =\n%s\nwant %q", got, want)
		}
	}
	if strings.Contains(got, "Complete") || strings.Contains(got, "networks:\n") {
		t.Errorf("display =\n%s\nwant no Complete heading and no heading line over the folded one", got)
	}
	// -v: one heading, Removed, with each network its own row; a lone volume still folds.
	got = prunePlain(r, nil, true)
	for _, want := range []string{"✓ Removed       2 networks:\n", "Removed         net_a:", "Removed         net_b:", "✓ Removed       1 volume: vol_a"} {
		if !strings.Contains(got, want) {
			t.Errorf("-v display =\n%s\nwant %q", got, want)
		}
	}
}

func TestPruneFoldedServicesKeepTheirNameStyle(t *testing.T) {
	prev := semstyle.GetPreferredProfile()
	semstyle.SetPreferredProfile(colorprofile.ANSI)
	t.Cleanup(func() { semstyle.SetPreferredProfile(prev) })
	r := PruneReport{ImagesDeleted: []image.DeleteResponse{{Untagged: "example/zzuser:1"}, {Deleted: "sha256:l1"}}}
	lines, _ := buildPruneLines(r, map[string][]string{"example/zzuser:1": {"zzuser1", "zzuser2"}}, false, false, 160)
	joined := strings.Join(lines, "\n")
	// A user-defined service's name is the plain user style, not the bold built-in one.
	for _, name := range []string{"zzuser1", "zzuser2"} {
		if want := semstyle.ToANSI(dockerlayout.StyleServiceName(name)); !strings.Contains(joined, want) {
			t.Errorf("folded line lacks %q styled as a service name", name)
		}
	}
}

func TestPruneImagesHeadingWordIsImageStyle(t *testing.T) {
	prev := semstyle.GetPreferredProfile()
	semstyle.SetPreferredProfile(colorprofile.ANSI)
	t.Cleanup(func() { semstyle.SetPreferredProfile(prev) })
	r := PruneReport{ImagesDeleted: []image.DeleteResponse{{Untagged: "sha256:067bd9"}, {Deleted: "sha256:l1"}}}
	lines, _ := buildPruneLines(r, nil, false, false, 160)
	want := semstyle.ToANSI("{{|DockerImage|}}Untagged{{[-]}}")
	for _, l := range lines {
		if strings.Contains(ansi.Strip(l), "unused image:") && !strings.Contains(l, want) {
			t.Errorf("images heading %q lacks the image style for its status word", l)
		}
	}
}
