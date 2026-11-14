package runs

import (
	"bytes"
	"encoding/binary"
	"errors"
	"math"
	"testing"
)

// wavBytes is a mono 16-bit PCM file of seconds at rate, its amplitude set by amp(t).
func wavBytes(rate, seconds int, amp func(t float64) float64) []byte {
	n := rate * seconds
	data := make([]byte, n*2)
	for i := 0; i < n; i++ {
		t := float64(i) / float64(rate)
		v := int16(amp(t) * math.Sin(2*math.Pi*440*t) * 32767)
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

func TestReadAudioInfoWAVLengthAndWaveform(t *testing.T) {
	// Silent first half, loud second half: the waveform must show it.
	wav := wavBytes(8000, 4, func(t float64) float64 {
		if t < 2 {
			return 0
		}
		return 0.9
	})
	info, err := ReadAudioInfo("tone.wav", bytes.NewReader(wav), int64(len(wav)))
	if err != nil {
		t.Fatal(err)
	}
	if info.ContentType != "audio/wav" || info.DurationMS != 4000 {
		t.Fatalf("got %q %d ms, want audio/wav 4000 ms", info.ContentType, info.DurationMS)
	}
	if len(info.Peaks) != AudioPeakBuckets {
		t.Fatalf("got %d peaks, want %d", len(info.Peaks), AudioPeakBuckets)
	}
	if info.Peaks[2] != 0 || info.Peaks[AudioPeakBuckets-3] < 250 {
		t.Fatalf("waveform doesn't follow the signal: quiet half %d, loud half %d", info.Peaks[2], info.Peaks[AudioPeakBuckets-3])
	}
}

func TestReadAudioInfoRefusesTheWrongFile(t *testing.T) {
	png := append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 64)...)
	if _, err := ReadAudioInfo("song.wav", bytes.NewReader(png), int64(len(png))); !errors.Is(err, ErrNotAudio) {
		t.Fatalf("a PNG named .wav: got %v, want ErrNotAudio", err)
	}
	wav := wavBytes(8000, 1, func(float64) float64 { return 0.5 })
	if _, err := ReadAudioInfo("song.mp3", bytes.NewReader(wav), int64(len(wav))); !errors.Is(err, ErrNotAudio) {
		t.Fatalf("WAV named .mp3: got %v, want ErrNotAudio", err)
	}
}

func TestReadAudioInfoKeepsAFileItCannotDecode(t *testing.T) {
	ogg := append([]byte("OggS"), make([]byte, 64)...)
	info, err := ReadAudioInfo("voice.ogg", bytes.NewReader(ogg), int64(len(ogg)))
	if err != nil || info.ContentType != "audio/ogg" || info.Peaks != nil || info.DurationMS != 0 {
		t.Fatalf("OGG: got %+v, %v; want stored with no waveform", info, err)
	}
	// Garbage after an MP3 tag is still an MP3 upload, just without a waveform.
	mp3 := append([]byte("ID3"), make([]byte, 64)...)
	info, err = ReadAudioInfo("x.mp3", bytes.NewReader(mp3), int64(len(mp3)))
	if err != nil || info.ContentType != "audio/mpeg" || info.Peaks != nil {
		t.Fatalf("undecodable MP3: got %+v, %v; want stored with no waveform", info, err)
	}
}

func TestReadAudioInfoTruncatedWAVStillReportsWhatItHolds(t *testing.T) {
	wav := wavBytes(8000, 2, func(float64) float64 { return 0.5 })
	cut := wav[:len(wav)/2] // the data chunk claims more than the file holds
	info, err := ReadAudioInfo("cut.wav", bytes.NewReader(cut), int64(len(cut)))
	if err != nil || info.DurationMS < 900 || info.DurationMS > 1000 {
		t.Fatalf("truncated WAV: got %d ms, %v; want about 1 s", info.DurationMS, err)
	}
}

func TestReadAudioInfoReadsFromTheStartWhateverThePosition(t *testing.T) {
	wav := wavBytes(8000, 1, func(float64) float64 { return 0.5 })
	r := bytes.NewReader(wav)
	if _, err := r.Seek(0, 2); err != nil { // a file just copied into place sits at its end
		t.Fatal(err)
	}
	info, err := ReadAudioInfo("tone.wav", r, int64(len(wav)))
	if err != nil || info.DurationMS != 1000 {
		t.Fatalf("got %d ms, %v; want 1000 ms", info.DurationMS, err)
	}
}
