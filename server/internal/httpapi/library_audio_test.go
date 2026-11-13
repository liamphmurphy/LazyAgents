package httpapi

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/liamphmurphy/vesper/server/internal/runs"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

// tinyWAV is a mono 16-bit tone, quiet in its first half and loud in its second.
func tinyWAV(rate, seconds int) []byte {
	n := rate * seconds
	data := make([]byte, n*2)
	for i := 0; i < n; i++ {
		amp := 0.05
		if i >= n/2 {
			amp = 0.9
		}
		v := int16(amp * math.Sin(2*math.Pi*440*float64(i)/float64(rate)) * 32767)
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

// An uploaded WAV is read for its length and waveform when it's stored, shows
// them in the listing, seeks with byte ranges, can be renamed, and is
// private to its owner.
func TestLibraryAudioUploadPlayRename(t *testing.T) {
	e, _ := newAvatarEnv(t)
	wav := tinyWAV(8000, 2)
	code, item := e.uploadLibrary("Voice memo.wav", wav)
	if code != http.StatusCreated || item["kind"] != "audio" || item["content_type"] != "audio/wav" || item["source"] != "upload" || item["duration_ms"] != float64(2000) {
		t.Fatalf("upload %d %v", code, item)
	}
	if item["method"] != nil {
		t.Fatalf("an upload has no method: %v", item["method"])
	}
	peaks, _ := item["peaks"].([]any)
	if len(peaks) != runs.AudioPeakBuckets || peaks[2].(float64) > 40 || peaks[len(peaks)-3].(float64) < 250 {
		t.Fatalf("waveform doesn't follow the quiet-then-loud signal: %v", peaks)
	}
	id := item["id"].(string)

	listed := e.want("GET", "/library?kind=audio", nil, 200)["items"].([]any)
	if len(listed) != 1 || listed[0].(map[string]any)["id"] != id || listed[0].(map[string]any)["title"] != "Voice memo" {
		t.Fatalf("listed %v", listed)
	}

	res, body := fetch(t, e.srv.URL+"/api/v1/library/"+id+"/content", map[string]string{"Range": "bytes=0-3"})
	if res.StatusCode != http.StatusPartialContent || body != "RIFF" || res.Header.Get("Accept-Ranges") != "bytes" || res.Header.Get("Content-Type") != "audio/wav" {
		t.Fatalf("range %d %q %s", res.StatusCode, body, res.Header.Get("Content-Type"))
	}

	renamed := e.want("PATCH", "/library/"+id, map[string]any{"title": "App ideas"}, 200)
	if renamed["title"] != "App ideas" || renamed["kind"] != "audio" {
		t.Fatalf("renamed %v", renamed)
	}
	e.want("PATCH", "/library/"+id, map[string]any{"title": "   "}, http.StatusUnprocessableEntity)
	e.want("PATCH", "/library/"+uuid.NewString(), map[string]any{"title": "x"}, http.StatusNotFound)

	e.want("DELETE", "/library/"+id, nil, http.StatusNoContent)
	e.want("GET", "/library?kind=audio", nil, 200)
	if items := e.want("GET", "/library?kind=audio", nil, 200)["items"].([]any); len(items) != 0 {
		t.Fatalf("after delete %v", items)
	}
}

// The upload takes what it says it takes: a file must be the audio its name
// claims, and an undecodable format is stored without a waveform.
func TestLibraryAudioUploadChecksTheFile(t *testing.T) {
	e, _ := newAvatarEnv(t)
	if code, out := e.uploadLibrary("song.mp3", avatarPNG(t)); code != http.StatusUnprocessableEntity || !strings.Contains(fmt.Sprint(out), "isn't the mp3 audio file") {
		t.Fatalf("PNG named .mp3: %d %v", code, out)
	}
	if code, _ := e.uploadLibrary("song.wav", append([]byte("OggS"), make([]byte, 64)...)); code != http.StatusUnprocessableEntity {
		t.Fatalf("Ogg named .wav: %d", code)
	}
	if code, _ := e.uploadLibrary("empty.wav", nil); code != http.StatusUnprocessableEntity {
		t.Fatalf("empty file: %d", code)
	}
	if code, _ := e.uploadLibrary("notes.txt", []byte("hello")); code != http.StatusUnprocessableEntity {
		t.Fatalf("text file: %d", code)
	}
	code, item := e.uploadLibrary("field.ogg", append([]byte("OggS"), make([]byte, 64)...))
	if code != http.StatusCreated || item["content_type"] != "audio/ogg" || item["peaks"] != nil || item["duration_ms"] != nil {
		t.Fatalf("ogg upload %d %v", code, item)
	}
}

// A chat's audio is listed with its stages and checks, plays by byte range from
// the chat, and a stranger sees none of it.
func TestChatAudioAPI(t *testing.T) {
	e, s3 := newAvatarEnv(t)
	ctx := t.Context()
	chat := e.newChat(nil)
	chatID := uuid.MustParse(chat["id"].(string))
	job, err := e.q.CreateChatAudioJob(ctx, store.CreateChatAudioJobParams{WorkspaceID: e.me.WorkspaceID, UserID: e.me.ID, ChatID: &chatID, Title: "Riff", DurationMs: 24000, Method: "MIDI → FluidSynth", ScriptPath: "riff.py"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.q.SetChatAudioStage(ctx, store.SetChatAudioStageParams{ID: job.ID, Stage: "check", Checks: []byte(`[{"label":"Peak under -1 dBFS","passed":true},{"label":"No silence","passed":null}]`)}); err != nil {
		t.Fatal(err)
	}
	list := e.want("GET", "/chats/"+chatID.String()+"/audio", nil, 200)["audio"].([]any)
	if len(list) != 1 {
		t.Fatalf("list %v", list)
	}
	a := list[0].(map[string]any)
	checks := a["checks"].([]any)
	if a["stage"] != "check" || a["state"] != "running" || a["method"] != "MIDI → FluidSynth" || len(checks) != 2 || checks[0].(map[string]any)["passed"] != true || checks[1].(map[string]any)["passed"] != nil || a["output"] != nil {
		t.Fatalf("running audio %v", a)
	}
	e.want("GET", "/chats/"+chatID.String()+"/audio/"+job.ID.String()+"/content", nil, http.StatusNotFound) // not finished

	wav := tinyWAV(8000, 1)
	key := "chat-audio/" + uuid.NewString()
	s3.put(avatarBucket+"/"+key, wav, "audio/wav")
	size := int64(len(wav))
	name, ct, dur := "riff.wav", "audio/wav", int32(1000)
	if _, err := e.q.FinishChatAudioJob(ctx, store.FinishChatAudioJobParams{ID: job.ID, OutputBlobKey: &key, OutputFilename: &name, OutputContentType: &ct, OutputSizeBytes: &size, OutputDurationMs: &dur}); err != nil {
		t.Fatal(err)
	}
	a = e.want("GET", "/chats/"+chatID.String()+"/audio", nil, 200)["audio"].([]any)[0].(map[string]any)
	out := a["output"].(map[string]any)
	if a["state"] != "done" || a["percent"] != float64(100) || out["filename"] != "riff.wav" || out["duration_ms"] != float64(1000) {
		t.Fatalf("finished audio %v", a)
	}
	res, body := fetch(t, e.srv.URL+"/api/v1/chats/"+chatID.String()+"/audio/"+job.ID.String()+"/content", map[string]string{"Range": "bytes=8-11"})
	if res.StatusCode != http.StatusPartialContent || body != "WAVE" {
		t.Fatalf("range %d %q", res.StatusCode, body)
	}
	if res, _ := fetch(t, e.srv.URL+"/api/v1/chats/"+chatID.String()+"/audio/"+job.ID.String()+"/content", map[string]string{"Range": "bytes=99999-"}); res.StatusCode != http.StatusRequestedRangeNotSatisfiable {
		t.Fatalf("unsatisfiable range %d", res.StatusCode)
	}

	// Another chat of the same person can't reach it by its id.
	other := e.newChat(nil)
	e.want("GET", "/chats/"+other["id"].(string)+"/audio/"+job.ID.String()+"/content", nil, http.StatusNotFound)

	// Deleting the chat detaches the job; the sweep takes it and its file.
	e.want("DELETE", "/chats/"+chatID.String(), nil, http.StatusNoContent)
	keys, err := e.q.DeleteDetachedChatAudioJobs(ctx, 10)
	if err != nil || len(keys) != 1 || keys[0] != key {
		t.Fatalf("swept %v %v", keys, err)
	}
}
