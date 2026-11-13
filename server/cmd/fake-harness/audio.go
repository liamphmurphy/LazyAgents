package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
)

// fakeAudio walks an audio file through its stages as a renderer would, by
// step: ok, ogg (a format the server can't decode), fail, unfinished or empty.
func fakeAudio(call func(string, map[string]any) map[string]any, title, step string) string {
	start := call("start_audio", map[string]any{"title": title, "duration_seconds": 2, "method": "numpy sine sweep", "script": "tone.py"})
	id, _ := start["audio_id"].(string)
	if id == "" {
		return fmt.Sprintf("start_audio failed: %v", start["error"])
	}
	dir := filepath.Join(".lazyagents", "audio", id)
	must := func(err error) {
		if err != nil {
			panic(err)
		}
	}
	write := func(name string, b []byte) {
		p := filepath.Join(dir, name)
		must(os.MkdirAll(filepath.Dir(p), 0o755))
		must(os.WriteFile(p, b, 0o644))
	}
	must(os.WriteFile("tone.py", []byte("import numpy as np\n"), 0o644))
	call("update_audio", map[string]any{"audio_id": id, "stage": "render"})
	if step == "fail" {
		out := call("fail_audio", map[string]any{"audio_id": id, "error": "fluidsynth isn't installed in this workspace.", "log": "sh: 1: fluidsynth: not found"})
		return fmt.Sprintf("Audio failed (%v)", out["state"])
	}
	progress, _ := json.Marshal(map[string]any{"percent": 80, "note": "Rendering MIDI with FluidSynth"})
	write("progress.json", progress)
	call("update_audio", map[string]any{"audio_id": id, "stage": "check", "checks": []map[string]any{{"label": "Peak stays under -1 dBFS", "passed": true}, {"label": "No silence over 1 s", "passed": true}}})
	if step == "unfinished" {
		return "Rendering…"
	}
	switch step {
	case "empty":
	case "ogg":
		write("audio.ogg", append([]byte("OggS"), make([]byte, 64)...))
	default:
		write("audio.wav", fakeWAV(8000, 2))
	}
	out := call("finish_audio", map[string]any{"audio_id": id})
	return fmt.Sprintf("Here's the audio (%v).", out["stage"])
}

// fakeWAV is a mono 16-bit tone that fades in over its length, so its
// waveform rises from left to right.
func fakeWAV(rate, seconds int) []byte {
	n := rate * seconds
	data := make([]byte, n*2)
	for i := 0; i < n; i++ {
		t := float64(i) / float64(rate)
		v := int16(0.9 * float64(i) / float64(n) * math.Sin(2*math.Pi*440*t) * 32767)
		binary.LittleEndian.PutUint16(data[i*2:], uint16(v))
	}
	var b bytes.Buffer
	b.WriteString("RIFF")
	binary.Write(&b, binary.LittleEndian, uint32(36+len(data)))
	b.WriteString("WAVEfmt ")
	for _, v := range []any{uint32(16), uint16(1), uint16(1), uint32(rate), uint32(rate * 2), uint16(2), uint16(16)} {
		binary.Write(&b, binary.LittleEndian, v)
	}
	b.WriteString("data")
	binary.Write(&b, binary.LittleEndian, uint32(len(data)))
	b.Write(data)
	return b.Bytes()
}
