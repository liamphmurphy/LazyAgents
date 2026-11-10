package runner

import "testing"

func TestFrameSlot(t *testing.T) {
	for rel, want := range map[string]struct {
		kind string
		idx  int
		ok   bool
	}{
		"preview.jpg": {"preview", 0, true}, "poster.jpg": {"poster", 0, true},
		"stills/0120.png": {"still", 120, true}, "stills/3.jpeg": {"still", 3, true},
		"strip/07.jpg": {"strip", 7, true}, "strip/30.jpg": {"strip", 30, false},
		"stills/a.png": {}, "strip/1.gif": {}, "notes.txt": {},
	} {
		kind, idx, ok := frameSlot(rel)
		if ok != want.ok || (ok && (kind != want.kind || idx != want.idx)) {
			t.Errorf("%s: %s %d %v", rel, kind, idx, ok)
		}
	}
}

func TestCleanProgress(t *testing.T) {
	p := cleanProgress(videoProgress{FramesDone: 900, FramesTotal: 450, Note: " H.264\nCRF 18 "})
	if p.FramesDone != 450 || p.FramesTotal != 450 || p.Note != "H.264 CRF 18" {
		t.Fatalf("%+v", p)
	}
	if p := cleanProgress(videoProgress{FramesDone: 5, FramesTotal: -1}); p.FramesDone != 0 || p.FramesTotal != 0 {
		t.Fatalf("%+v", p)
	}
}

func TestScriptTail(t *testing.T) {
	n, tail := scriptTail("a\nb\n\n\nc\nd\ne\nf\ng\n\n")
	if n != 9 || tail != "b\nc\nd\ne\nf\ng" {
		t.Fatalf("%d %q", n, tail)
	}
	if n, tail := scriptTail(""); n != 0 || tail != "" {
		t.Fatalf("%d %q", n, tail)
	}
}

func TestVideoFilename(t *testing.T) {
	if got := videoFilename("Black cat — on a rainy street!"); got != "black-cat-on-a-rainy-street.mp4" {
		t.Fatal(got)
	}
	if got := videoFilename("猫"); got != "video.mp4" {
		t.Fatal(got)
	}
}
