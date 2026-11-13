package runs

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"math"
	"strings"

	"github.com/hajimehoshi/go-mp3"
)

// What the Library knows about an audio file: its type, and for the formats
// the server can decode, its length and the waveform its tiles draw. The
// server image carries no ffmpeg, so only WAV and MP3 are decoded; an OGG or
// M4A file is stored and played but has no waveform (AudioInfo.Peaks is nil).

// AudioPeakBuckets is the waveform's resolution: one byte per bucket.
const AudioPeakBuckets = 64

// MaxAudioDecodeSeconds bounds how much audio is decoded for its waveform, so
// a hostile file can't hold a request open.
const MaxAudioDecodeSeconds = 3 * 3600

// ErrNotAudio is returned for a file that isn't one of the audio formats.
var ErrNotAudio = errors.New("not an audio file")

// AudioInfo is what ReadAudioInfo learned about a file.
type AudioInfo struct {
	ContentType string
	// DurationMS is 0 when the format isn't decoded.
	DurationMS int
	// Peaks has AudioPeakBuckets bytes (0-255), or is nil when not decoded.
	Peaks []byte
	Codec string
}

// audioExt maps a filename extension to the content type it must sniff as.
var audioExt = map[string]string{
	".mp3": "audio/mpeg", ".wav": "audio/wav", ".ogg": "audio/ogg", ".oga": "audio/ogg", ".m4a": "audio/mp4",
}

// AudioTypeFromHead names the audio format head starts like, by its magic
// bytes, or "" when it isn't one.
func AudioTypeFromHead(head []byte) string {
	switch {
	case len(head) >= 12 && bytes.Equal(head[:4], []byte("RIFF")) && bytes.Equal(head[8:12], []byte("WAVE")):
		return "audio/wav"
	case bytes.HasPrefix(head, []byte("OggS")):
		return "audio/ogg"
	case len(head) >= 12 && bytes.Equal(head[4:8], []byte("ftyp")):
		return "audio/mp4"
	case bytes.HasPrefix(head, []byte("ID3")):
		return "audio/mpeg"
	case len(head) >= 2 && head[0] == 0xFF && head[1]&0xE0 == 0xE0 && head[1]&0x18 != 0x08 && head[1]&0x06 != 0:
		return "audio/mpeg" // an MPEG audio frame header without a tag
	}
	return ""
}

// AudioNameType is the content type a filename claims, or "" for a name that
// isn't an audio file.
func AudioNameType(name string) string {
	i := strings.LastIndex(name, ".")
	if i < 0 {
		return ""
	}
	return audioExt[strings.ToLower(name[i:])]
}

// ReadAudioInfo checks that the file is the audio its name says and reads
// what it can of its length and waveform.
func ReadAudioInfo(name string, r io.ReadSeeker, size int64) (AudioInfo, error) {
	// Callers hand over a file they have just written or copied.
	if _, err := r.Seek(0, io.SeekStart); err != nil {
		return AudioInfo{}, err
	}
	head := make([]byte, 16)
	n, _ := io.ReadFull(r, head)
	if _, err := r.Seek(0, io.SeekStart); err != nil {
		return AudioInfo{}, err
	}
	ct := AudioTypeFromHead(head[:n])
	if want := AudioNameType(name); ct == "" || (want != "" && want != ct) {
		return AudioInfo{}, ErrNotAudio
	}
	info := AudioInfo{ContentType: ct}
	var err error
	switch ct {
	case "audio/wav":
		info.Codec = "PCM"
		err = readWAV(r, size, &info)
	case "audio/mpeg":
		info.Codec = "MP3"
		err = readMP3(r, &info)
	case "audio/ogg":
		info.Codec = "Ogg"
	case "audio/mp4":
		info.Codec = "AAC"
	}
	if err != nil {
		// A file we recognise but can't decode is still stored: no waveform.
		info.DurationMS, info.Peaks = 0, nil
	}
	return info, nil
}

// peakBuilder folds samples into AudioPeakBuckets maxima once the total is known.
type peakBuilder struct {
	total int64 // frames expected
	max   [AudioPeakBuckets]float64
	seen  int64
}

func (p *peakBuilder) add(level float64) {
	if p.total <= 0 {
		return
	}
	b := int(p.seen * AudioPeakBuckets / p.total)
	if b >= AudioPeakBuckets {
		b = AudioPeakBuckets - 1
	}
	if level > p.max[b] {
		p.max[b] = level
	}
	p.seen++
}

// bytes scales the buckets so the loudest fills the range: a quiet file still
// draws a readable waveform.
func (p *peakBuilder) bytes() []byte {
	top := 0.0
	for _, v := range p.max {
		top = math.Max(top, v)
	}
	out := make([]byte, AudioPeakBuckets)
	if top == 0 {
		return out
	}
	for i, v := range p.max {
		out[i] = byte(math.Round(v / top * 255))
	}
	return out
}

func readWAV(r io.ReadSeeker, size int64, info *AudioInfo) error {
	var riff [12]byte
	if _, err := io.ReadFull(r, riff[:]); err != nil {
		return err
	}
	var (
		format, channels, bits uint16
		rate                   uint32
		haveFmt                bool
	)
	for {
		var ch [8]byte
		if _, err := io.ReadFull(r, ch[:]); err != nil {
			return errors.New("wav: no data chunk")
		}
		id, n := string(ch[:4]), int64(binary.LittleEndian.Uint32(ch[4:]))
		switch id {
		case "fmt ":
			if n < 16 {
				return errors.New("wav: short fmt chunk")
			}
			buf := make([]byte, n)
			if _, err := io.ReadFull(r, buf); err != nil {
				return err
			}
			format, channels = binary.LittleEndian.Uint16(buf[0:]), binary.LittleEndian.Uint16(buf[2:])
			rate, bits = binary.LittleEndian.Uint32(buf[4:]), binary.LittleEndian.Uint16(buf[14:])
			if format == 0xFFFE && n >= 26 { // extensible: the real format leads the sub-format GUID
				format = binary.LittleEndian.Uint16(buf[24:])
			}
			haveFmt = true
		case "data":
			if !haveFmt || channels == 0 || rate == 0 || bits == 0 || bits%8 != 0 {
				return errors.New("wav: data before a usable fmt chunk")
			}
			if (format != 1 && format != 3) || (format == 3 && bits != 32 && bits != 64) {
				return errors.New("wav: unsupported encoding")
			}
			pos, err := r.Seek(0, io.SeekCurrent)
			if err != nil {
				return err
			}
			return wavData(r, n, size-pos, int(channels), int(bits/8), int64(rate), format == 3, info)
		default:
			if _, err := r.Seek(n+n&1, io.SeekCurrent); err != nil {
				return err
			}
		}
	}
}

func wavData(r io.Reader, n, avail int64, channels, width int, rate int64, float bool, info *AudioInfo) error {
	frame := int64(channels * width)
	if n <= 0 || n > avail { // a streamed or truncated file claims more than it holds
		n = avail
	}
	frames := n / frame
	if frames == 0 || frames/rate > MaxAudioDecodeSeconds {
		return errors.New("wav: empty or too long")
	}
	info.DurationMS = int(frames * 1000 / rate)
	pk := &peakBuilder{total: frames}
	buf := make([]byte, frame*4096)
	for left := frames; left > 0; {
		want := min(int64(len(buf)), left*frame)
		got, err := io.ReadFull(r, buf[:want])
		for off := 0; off+int(frame) <= got; off += int(frame) {
			level := 0.0
			for c := 0; c < channels; c++ {
				level = math.Max(level, math.Abs(sampleAt(buf[off+c*width:], width, float)))
			}
			pk.add(level)
		}
		left -= int64(got) / frame
		if err != nil {
			break
		}
	}
	info.Peaks = pk.bytes()
	return nil
}

// sampleAt reads one little-endian sample as -1…1.
func sampleAt(b []byte, width int, float bool) float64 {
	switch {
	case float && width == 4:
		return float64(math.Float32frombits(binary.LittleEndian.Uint32(b)))
	case float:
		return math.Float64frombits(binary.LittleEndian.Uint64(b))
	case width == 1:
		return (float64(b[0]) - 128) / 128 // 8-bit WAV is unsigned
	case width == 2:
		return float64(int16(binary.LittleEndian.Uint16(b))) / 32768
	case width == 3:
		v := int32(b[0]) | int32(b[1])<<8 | int32(int8(b[2]))<<16
		return float64(v) / 8388608
	default:
		return float64(int32(binary.LittleEndian.Uint32(b))) / 2147483648
	}
}

func readMP3(r io.Reader, info *AudioInfo) error {
	d, err := mp3.NewDecoder(r)
	if err != nil {
		return err
	}
	rate := int64(d.SampleRate())
	// go-mp3 always decodes to 16-bit stereo: 4 bytes a frame.
	frames := d.Length() / 4
	if rate <= 0 || frames <= 0 || frames/rate > MaxAudioDecodeSeconds {
		return errors.New("mp3: unknown or too long")
	}
	info.DurationMS = int(frames * 1000 / rate)
	pk := &peakBuilder{total: frames}
	buf := make([]byte, 4*4096)
	for {
		n, err := d.Read(buf)
		for off := 0; off+4 <= n; off += 4 {
			l := math.Abs(float64(int16(binary.LittleEndian.Uint16(buf[off:]))) / 32768)
			rr := math.Abs(float64(int16(binary.LittleEndian.Uint16(buf[off+2:]))) / 32768)
			pk.add(math.Max(l, rr))
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return err
		}
	}
	info.Peaks = pk.bytes()
	return nil
}
