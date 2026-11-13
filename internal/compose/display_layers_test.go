package compose

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/GhostWriters/semstyle"
	"github.com/docker/compose/v5/pkg/api"
)

const (
	testImg   = "example/app:latest"
	otherImg  = "example/other:latest"
	rawSizeR1 = 100
	rawSizeR2 = 200
)

// newLayerTestProcessor returns a processor holding testImg's pre-flight layers as
// "Already exists" tasks and its downloads as rawID-keyed tasks, each sized so a
// misplaced download shows: r1 is 100 bytes, r2 200, and so on.
func newLayerTestProcessor(preflight, rawIDs []string) *consoleEventProcessor {
	p := &consoleEventProcessor{
		logCtx:            context.Background(),
		command:           "pull",
		tasks:             make(map[string]*consoleTask),
		imageLayerDiffIDs: make(map[string][]string),
		diffIDImageCount:  make(map[string]int),
		diffIDGroupNum:    make(map[string]int),
		imagePullOrder:    make(map[string][]string),
		localDiffIDs:      make(map[string]bool),
		imageOrder:        []string{testImg},
	}
	if len(preflight) > 0 {
		p.imageLayerDiffIDs[testImg] = preflight
	}
	p.addCachedLayerTasks(preflight)
	for i, id := range rawIDs {
		addRawDownload(p, testImg, id, int64(100*(i+1)))
	}
	return p
}

func addRawDownload(p *consoleEventProcessor, img, id string, size int64) {
	if p.tasks[id] == nil {
		p.tasks[id] = &consoleTask{id: id, parentID: img, text: "Pull complete", status: api.Done, total: size, current: size, percent: 100}
		p.ids = append(p.ids, id)
	}
	p.imagePullOrder[img] = append(p.imagePullOrder[img], id)
}

// wantDownload fails unless sha holds the download of the given size.
func wantDownload(t *testing.T, p *consoleEventProcessor, sha string, size int64) {
	t.Helper()
	if tk := p.tasks[sha]; tk == nil || tk.text != "Pull complete" || tk.total != size {
		t.Errorf("%s = %+v, want the %d-byte download", sha, tk, size)
	}
}

func TestApplyInspectResultMapsDownloadsInOrder(t *testing.T) {
	p := newLayerTestProcessor([]string{"sha:a", "sha:b"}, []string{"r1", "r2"})
	p.applyInspectResult(testImg, []string{"sha:a", "sha:c", "sha:d"})

	if got := p.imageLayerDiffIDs[testImg]; !slices.Equal(got, []string{"sha:a", "sha:c", "sha:d"}) {
		t.Fatalf("layers = %v", got)
	}
	wantDownload(t, p, "sha:c", rawSizeR1)
	wantDownload(t, p, "sha:d", rawSizeR2)
	if p.tasks["r1"] != nil || p.tasks["r2"] != nil || len(p.imagePullOrder[testImg]) != 0 {
		t.Errorf("raw downloads left after mapping")
	}
}

func TestApplyInspectResultKeepsDownloadsWhenAmbiguous(t *testing.T) {
	// sha:e is cached but unrecognised, so three slots face two downloads.
	p := newLayerTestProcessor([]string{"sha:a"}, []string{"r1", "r2"})
	p.applyInspectResult(testImg, []string{"sha:a", "sha:c", "sha:d", "sha:e"})

	if got := p.imageLayerDiffIDs[testImg]; !slices.Equal(got, []string{"sha:a"}) {
		t.Fatalf("layers = %v, want only the known cached layer", got)
	}
	if p.tasks["sha:c"] != nil || p.tasks["sha:d"] != nil || p.tasks["sha:e"] != nil {
		t.Errorf("a download was placed on a layer it can't be matched to")
	}
	if got := len(p.layersForImage(testImg)); got != 3 {
		t.Errorf("layersForImage = %d rows, want the cached layer plus both downloads", got)
	}
	if got := semstyle.ToPlain(p.buildSummaryLine()); !strings.Contains(got, "3 layers") {
		t.Errorf("summary = %q, want 3 layers", got)
	}
}

func TestApplyInspectResultKeepsDownloadsOnContainerdStore(t *testing.T) {
	// The counts match, but the containerd store's downloads have no fixed order.
	p := newLayerTestProcessor([]string{"sha:a"}, []string{"r1", "r2"})
	p.containerdStore = true
	p.applyInspectResult(testImg, []string{"sha:a", "sha:c", "sha:d"})

	if got := p.imageLayerDiffIDs[testImg]; !slices.Equal(got, []string{"sha:a"}) {
		t.Fatalf("layers = %v, want only the known cached layer", got)
	}
	if p.tasks["r1"] == nil || p.tasks["r2"] == nil {
		t.Errorf("downloads not kept as their own rows")
	}
}

func TestApplyInspectResultReplacesLayersWithNoDownloads(t *testing.T) {
	// An update whose new layers were all on disk already.
	p := newLayerTestProcessor([]string{"sha:a", "sha:b"}, nil)
	p.localDiffIDs["sha:e"] = true
	p.applyInspectResult(testImg, []string{"sha:a", "sha:e"})

	if got := p.imageLayerDiffIDs[testImg]; !slices.Equal(got, []string{"sha:a", "sha:e"}) {
		t.Fatalf("layers = %v, want the updated image's", got)
	}
	if tk := p.tasks["sha:e"]; tk == nil || tk.text != "Already exists" {
		t.Errorf("sha:e = %+v, want Already exists", tk)
	}
}

func TestApplyInspectResultSkipsDownloadsRemappedByAnotherImage(t *testing.T) {
	// r1 was shared with another image, which already moved it onto sha:c.
	p := newLayerTestProcessor([]string{"sha:a"}, []string{"r1", "r2"})
	p.tasks["sha:c"] = p.tasks["r1"]
	p.tasks["sha:c"].id = "sha:c"
	delete(p.tasks, "r1")
	p.applyInspectResult(testImg, []string{"sha:a", "sha:c", "sha:d"})

	if got := p.imageLayerDiffIDs[testImg]; !slices.Equal(got, []string{"sha:a", "sha:c", "sha:d"}) {
		t.Fatalf("layers = %v", got)
	}
	wantDownload(t, p, "sha:c", rawSizeR1)
	wantDownload(t, p, "sha:d", rawSizeR2)
	if !slices.Contains(p.layersForImage(testImg), p.tasks["sha:c"]) {
		t.Errorf("layersForImage lost the shared layer sha:c")
	}
}

func TestApplyInspectResultKeepsSharedDownloadForOtherImage(t *testing.T) {
	// otherImg keeps r1 as its own row; testImg then places r1 on sha:c.
	p := newLayerTestProcessor([]string{"sha:a"}, []string{"r1"})
	p.imageOrder = append(p.imageOrder, otherImg)
	addRawDownload(p, otherImg, "r1", rawSizeR1)
	p.applyInspectResult(testImg, []string{"sha:a", "sha:c"})

	wantDownload(t, p, "sha:c", rawSizeR1)
	if !slices.Contains(p.layersForImage(otherImg), p.tasks["sha:c"]) {
		t.Errorf("otherImg lost its row for the shared download")
	}
	if got := p.layerCount(); got != 2 {
		t.Errorf("layerCount = %d, want sha:a and sha:c once each", got)
	}
}

func TestApplyInspectResultRepeatedLayerIsOneSlot(t *testing.T) {
	// sha:c appears twice in RootFS but downloads once.
	p := newLayerTestProcessor([]string{"sha:a"}, []string{"r1"})
	p.applyInspectResult(testImg, []string{"sha:a", "sha:c", "sha:c"})

	wantDownload(t, p, "sha:c", rawSizeR1)
	if p.tasks["r1"] != nil {
		t.Errorf("r1 left unplaced")
	}

	// Two downloads for the one repeated layer can't share its task.
	p = newLayerTestProcessor([]string{"sha:a"}, []string{"r1", "r2"})
	p.applyInspectResult(testImg, []string{"sha:a", "sha:c", "sha:c"})
	if p.tasks["r1"] == nil || p.tasks["r2"] == nil {
		t.Errorf("a download was overwritten on the repeated layer")
	}
}

func TestStatusIncludesUnmappedDownloads(t *testing.T) {
	p := newLayerTestProcessor([]string{"sha:a", "sha:b"}, []string{"r1"})
	p.tasks["r1"].text = "Downloading"
	p.tasks["r1"].status = api.Working

	if p.allLayersAlreadyExist(testImg) {
		t.Errorf("allLayersAlreadyExist = true while a download is running")
	}
	if got := p.worstChildStatus(testImg); got != api.Working {
		t.Errorf("worstChildStatus = %v, want Working", got)
	}
	p.tasks["r1"].status = api.Error
	if got := p.worstChildStatus(testImg); got != api.Error {
		t.Errorf("worstChildStatus = %v, want Error from the download", got)
	}
}

func TestUsesContainerdStore(t *testing.T) {
	if !usesContainerdStore([][2]string{{"driver-type", "io.containerd.snapshotter.v1"}}) {
		t.Errorf("containerd snapshotter not detected")
	}
	if usesContainerdStore([][2]string{{"Backing Filesystem", "extfs"}, {"Supports d_type", "true"}}) {
		t.Errorf("overlay2 graphdriver detected as containerd")
	}
}
