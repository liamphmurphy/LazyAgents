package runs

import (
	"bytes"
	"encoding/binary"
	"errors"
	"testing"
)

func box(typ string, payload ...[]byte) []byte {
	n := 8
	for _, p := range payload {
		n += len(p)
	}
	b := make([]byte, 8, n)
	binary.BigEndian.PutUint32(b, uint32(n))
	copy(b[4:], typ)
	for _, p := range payload {
		b = append(b, p...)
	}
	return b
}

func u32(v int) []byte { b := make([]byte, 4); binary.BigEndian.PutUint32(b, uint32(v)); return b }

// testMP4 is an MP4's box skeleton: an audio track first, then the video
// track, with mdat before moov (no faststart) when late is set.
func testMP4(codec string, mvhdV1, late bool) []byte {
	var mvhd []byte
	if mvhdV1 {
		mvhd = append([]byte{1, 0, 0, 0}, make([]byte, 16)...)
		mvhd = append(append(mvhd, u32(600)...), append(make([]byte, 4), u32(9000)...)...) // 15 s
	} else {
		mvhd = append(append(make([]byte, 12), u32(1000)...), u32(15000)...)
	}
	mvhd = append(mvhd, make([]byte, 80)...)
	trak := func(handler string, w, h int, fourcc string) []byte {
		tkhd := append(make([]byte, 76), append(u32(w<<16), u32(h<<16)...)...)
		hdlr := append(append(make([]byte, 8), handler...), make([]byte, 13)...)
		stsd := append(append(make([]byte, 4), u32(1)...), box(fourcc, make([]byte, 78))...)
		return box("trak", box("tkhd", tkhd), box("mdia", box("hdlr", hdlr), box("minf", box("stbl", box("stsd", stsd)))))
	}
	moov := box("moov", box("mvhd", mvhd), trak("soun", 0, 0, "mp4a"), trak("vide", 1920, 1080, codec))
	ftyp := box("ftyp", []byte("isom"), u32(512), []byte("isomavc1"))
	mdat := box("mdat", make([]byte, 100))
	if late {
		return append(append(ftyp, mdat...), moov...)
	}
	return append(append(ftyp, moov...), mdat...)
}

func TestReadVideoInfo(t *testing.T) {
	for _, tc := range []struct {
		name  string
		file  []byte
		codec string
		durMS int
	}{
		{"faststart h264", testMP4("avc1", false, false), "H.264", 15000},
		{"moov at the end, mvhd v1", testMP4("hvc1", true, true), "H.265", 15000},
		{"unknown codec keeps its code", testMP4("abcd", false, false), "abcd", 15000},
	} {
		info, err := ReadVideoInfo(bytes.NewReader(tc.file), int64(len(tc.file)))
		if err != nil || info.Codec != tc.codec || info.DurationMS != tc.durMS || info.Width != 1920 || info.Height != 1080 {
			t.Errorf("%s: %+v %v", tc.name, info, err)
		}
	}
	if _, err := ReadVideoInfo(bytes.NewReader([]byte("\x1aE\xdf\xa3 webm")), 9); !errors.Is(err, ErrNotMP4) {
		t.Errorf("webm: %v", err)
	}
	// A box claiming more than the file holds is an error, not a panic.
	bad := testMP4("avc1", false, false)
	binary.BigEndian.PutUint32(bad[len(bad)-108:], 1<<30)
	if _, err := ReadVideoInfo(bytes.NewReader(bad), int64(len(bad))); err == nil {
		t.Error("truncated file read cleanly")
	}
}
