package main

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Small stand-ins for what a renderer writes: the sniffed signatures are all
// Vesper checks.
var (
	fakePNG  = append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 64)...)
	fakeJPEG = append([]byte("\xff\xd8\xff\xe0"), make([]byte, 64)...)
)

// fakeVideo walks a video through its stages as a renderer would, by step:
// ok, fail, unfinished or empty.
func fakeVideo(call func(string, map[string]any) map[string]any, title, step string) string {
	start := call("start_video", map[string]any{"title": title, "width": 640, "height": 360, "fps": 30, "duration_seconds": 2, "script": "render.py"})
	id, _ := start["video_id"].(string)
	if id == "" {
		return fmt.Sprintf("start_video failed: %v", start["error"])
	}
	dir := filepath.Join(".lazyagents", "videos", id)
	must := func(err error) {
		if err != nil {
			panic(err)
		}
	}
	write := func(rel string, b []byte) {
		p := filepath.Join(dir, rel)
		must(os.MkdirAll(filepath.Dir(p), 0o755))
		must(os.WriteFile(p, b, 0o644))
	}
	must(os.WriteFile("render.py", []byte("import numpy as np\n\ndef render_frame(t):\n    return t\n\nif __name__ == '__main__':\n    main()\n"), 0o644))
	if step == "fail" {
		call("update_video", map[string]any{"video_id": id, "stage": "render"})
		out := call("fail_video", map[string]any{"video_id": id, "error": "ffmpeg isn't installed in this workspace.", "log": "sh: 1: ffmpeg: not found"})
		return fmt.Sprintf("Video failed (%v)", out["state"])
	}
	write("stills/0.png", fakePNG)
	write("stills/30.png", fakePNG)
	call("update_video", map[string]any{"video_id": id, "stage": "stills", "checks": []map[string]any{{"label": "Paws stay planted"}, {"label": "Rain falls along its slant"}}})
	call("update_video", map[string]any{"video_id": id, "stage": "stills", "checks": []map[string]any{{"label": "Paws stay planted", "passed": true}, {"label": "Rain falls along its slant", "passed": true}}})
	call("update_video", map[string]any{"video_id": id, "stage": "render"})
	progress, _ := json.Marshal(map[string]any{"frames_done": 60, "frames_total": 60, "stage": "encode", "note": "H.264 (libx264, CRF 18)"})
	write("progress.json", progress)
	write("preview.jpg", fakeJPEG)
	write("strip/0.jpg", fakeJPEG)
	write("strip/29.jpg", fakeJPEG)
	write("poster.jpg", fakeJPEG)
	if step == "unfinished" {
		return "Rendering…"
	}
	if step != "empty" {
		write("video.mp4", fakeMP4(640, 360, 2000))
	}
	out := call("finish_video", map[string]any{"video_id": id})
	return fmt.Sprintf("Here's the render (%v).", out["stage"])
}

func mp4Box(typ string, payload ...[]byte) []byte {
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

// fakeMP4 is the box skeleton of an H.264 MP4: enough for Vesper to read its
// length, size and codec, with no media data.
func fakeMP4(width, height, ms int) []byte {
	u32 := func(v int) []byte { b := make([]byte, 4); binary.BigEndian.PutUint32(b, uint32(v)); return b }
	mvhd := append(make([]byte, 12), append(u32(1000), u32(ms)...)...)
	mvhd = append(mvhd, make([]byte, 80)...)
	tkhd := append(make([]byte, 76), append(u32(width<<16), u32(height<<16)...)...)
	hdlr := append(make([]byte, 8), []byte("vide")...)
	hdlr = append(hdlr, make([]byte, 13)...)
	stsd := append(append(make([]byte, 4), u32(1)...), mp4Box("avc1", make([]byte, 78))...)
	stbl := mp4Box("stbl", mp4Box("stsd", stsd))
	trak := mp4Box("trak", mp4Box("tkhd", tkhd), mp4Box("mdia", mp4Box("hdlr", hdlr), mp4Box("minf", stbl)))
	return append(mp4Box("ftyp", []byte("isom"), u32(512), []byte("isomavc1")), mp4Box("moov", mp4Box("mvhd", mvhd), trak)...)
}
