package compose

import (
	"slices"
	"testing"

	"github.com/docker/compose/v5/pkg/api"
)

const testImg = "example/app:latest"

// newLayerTestProcessor returns a processor holding testImg's pre-flight layers as
// "Already exists" tasks and its downloads as rawID-keyed tasks.
func newLayerTestProcessor(preflight, rawIDs []string) *consoleEventProcessor {
	p := &consoleEventProcessor{
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
	for _, id := range rawIDs {
		p.tasks[id] = &consoleTask{id: id, parentID: testImg, text: "Pull complete", status: api.Done}
		p.ids = append(p.ids, id)
		p.imagePullOrder[testImg] = append(p.imagePullOrder[testImg], id)
	}
	return p
}

func TestApplyInspectResultMapsDownloadsInOrder(t *testing.T) {
	p := newLayerTestProcessor([]string{"sha:a", "sha:b"}, []string{"r1", "r2"})
	p.applyInspectResult(testImg, []string{"sha:a", "sha:c", "sha:d"})

	if got := p.imageLayerDiffIDs[testImg]; !slices.Equal(got, []string{"sha:a", "sha:c", "sha:d"}) {
		t.Fatalf("layers = %v", got)
	}
	for _, sha := range []string{"sha:c", "sha:d"} {
		if tk := p.tasks[sha]; tk == nil || tk.text != "Pull complete" {
			t.Errorf("%s not mapped from a download: %+v", sha, tk)
		}
	}
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
	if got := p.layerCount(); got != 3 {
		t.Errorf("layerCount = %d, want the cached layer plus both downloads", got)
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
	delete(p.tasks, "r1")
	p.applyInspectResult(testImg, []string{"sha:a", "sha:c", "sha:d"})

	if got := p.imageLayerDiffIDs[testImg]; !slices.Equal(got, []string{"sha:a", "sha:c", "sha:d"}) {
		t.Fatalf("layers = %v", got)
	}
	if tk := p.tasks["sha:d"]; tk == nil || tk.text != "Pull complete" {
		t.Errorf("sha:d not mapped from r2: %+v", tk)
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
