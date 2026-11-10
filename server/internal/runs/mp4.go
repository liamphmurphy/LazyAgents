package runs

import (
	"encoding/binary"
	"errors"
	"io"
)

// VideoInfo is what Vesper reads from a finished video's MP4 boxes, for the
// card and the Library: no decoder or ffprobe needed.
type VideoInfo struct {
	DurationMS int
	Width      int
	Height     int
	// Codec is the video track's sample entry, named for people (H.264,
	// H.265, AV1, VP9, MPEG-4), or its four-character code.
	Codec string
}

// ErrNotMP4 means the file doesn't start with an ftyp box.
var ErrNotMP4 = errors.New("not an MP4 file")

var codecNames = map[string]string{"avc1": "H.264", "avc3": "H.264", "hvc1": "H.265", "hev1": "H.265", "av01": "AV1", "vp09": "VP9", "mp4v": "MPEG-4"}

type mp4Box struct {
	typ        string
	start, end int64 // the payload, after the header
}

// mp4Boxes lists the boxes between start and end.
func mp4Boxes(r io.ReaderAt, start, end int64) ([]mp4Box, error) {
	var out []mp4Box
	for off := start; off+8 <= end; {
		var h [16]byte
		if _, err := r.ReadAt(h[:8], off); err != nil {
			return out, err
		}
		size, hdr := int64(binary.BigEndian.Uint32(h[:4])), int64(8)
		switch size {
		case 0:
			size = end - off
		case 1:
			if _, err := r.ReadAt(h[8:16], off+8); err != nil {
				return out, err
			}
			size, hdr = int64(binary.BigEndian.Uint64(h[8:16])), 16
		}
		if size < hdr || off+size > end {
			return out, errors.New("mp4: box runs past its parent")
		}
		out = append(out, mp4Box{typ: string(h[4:8]), start: off + hdr, end: off + size})
		off += size
		if len(out) > 4096 {
			return out, errors.New("mp4: too many boxes")
		}
	}
	return out, nil
}

func findBox(boxes []mp4Box, typ string) (mp4Box, bool) {
	for _, b := range boxes {
		if b.typ == typ {
			return b, true
		}
	}
	return mp4Box{}, false
}

func readAt(r io.ReaderAt, off int64, n int) ([]byte, error) {
	b := make([]byte, n)
	_, err := r.ReadAt(b, off)
	return b, err
}

// ReadVideoInfo reads an MP4 (or QuickTime) file's duration, the first video
// track's size and its codec. A file without ftyp is ErrNotMP4; one whose
// moov can't be read returns what was found with an error.
func ReadVideoInfo(r io.ReaderAt, size int64) (VideoInfo, error) {
	var info VideoInfo
	top, err := mp4Boxes(r, 0, size)
	if len(top) == 0 || top[0].typ != "ftyp" {
		return info, ErrNotMP4
	}
	if err != nil {
		return info, err
	}
	moov, ok := findBox(top, "moov")
	if !ok {
		return info, errors.New("mp4: no moov box")
	}
	kids, err := mp4Boxes(r, moov.start, moov.end)
	if err != nil {
		return info, err
	}
	if mvhd, ok := findBox(kids, "mvhd"); ok {
		v, err := readAt(r, mvhd.start, 32)
		if err == nil {
			var scale, dur uint64
			if v[0] == 1 {
				scale, dur = uint64(binary.BigEndian.Uint32(v[20:24])), binary.BigEndian.Uint64(v[24:32])
			} else {
				scale, dur = uint64(binary.BigEndian.Uint32(v[12:16])), uint64(binary.BigEndian.Uint32(v[16:20]))
			}
			if scale > 0 {
				info.DurationMS = int(dur * 1000 / scale)
			}
		}
	}
	for _, t := range kids {
		if t.typ != "trak" {
			continue
		}
		parts, err := mp4Boxes(r, t.start, t.end)
		if err != nil {
			continue
		}
		mdia, ok := findBox(parts, "mdia")
		if !ok {
			continue
		}
		m, err := mp4Boxes(r, mdia.start, mdia.end)
		if err != nil {
			continue
		}
		hdlr, ok := findBox(m, "hdlr")
		if !ok {
			continue
		}
		if h, err := readAt(r, hdlr.start+8, 4); err != nil || string(h) != "vide" {
			continue
		}
		if tkhd, ok := findBox(parts, "tkhd"); ok && tkhd.end-tkhd.start >= 8 {
			if wh, err := readAt(r, tkhd.end-8, 8); err == nil {
				info.Width, info.Height = int(binary.BigEndian.Uint32(wh[:4])>>16), int(binary.BigEndian.Uint32(wh[4:])>>16)
			}
		}
		if minf, ok := findBox(m, "minf"); ok {
			if mi, err := mp4Boxes(r, minf.start, minf.end); err == nil {
				if stbl, ok := findBox(mi, "stbl"); ok {
					if st, err := mp4Boxes(r, stbl.start, stbl.end); err == nil {
						if stsd, ok := findBox(st, "stsd"); ok {
							// version/flags (4), entry count (4), then the first entry's size (4) and format (4).
							if f, err := readAt(r, stsd.start+12, 4); err == nil {
								info.Codec = string(f)
								if n, ok := codecNames[info.Codec]; ok {
									info.Codec = n
								}
							}
						}
					}
				}
			}
		}
		break
	}
	return info, nil
}
