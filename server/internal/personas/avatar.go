package personas

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"image"
	"image/jpeg"
	"image/png"
	"net/http"
	"regexp"

	"github.com/google/uuid"

	"github.com/liamphmurphy/vesper/server/internal/apperr"
)

// Avatar uploads (ADR Decision 4). The type comes from the bytes, never from
// the declared Content-Type or filename, and the whole file must be exactly
// one well-formed PNG, JPEG or WebP (a JPEG may carry appended JPEGs, such as
// an Ultra HDR gain map): trailing data, a container that doesn't
// parse, or HTML/SVG markup anywhere in the file is refused, so a polyglot
// can't pass as an image.
const (
	MaxAvatarBytes = 1 << 20 // 1 MB
	// MaxAvatarSide bounds the decoded size, so a small but highly compressed
	// PNG or JPEG can't make the server allocate gigabytes.
	MaxAvatarSide = 2048
)

// AvatarTypes are the content types an avatar may have.
var AvatarTypes = []string{"image/png", "image/jpeg", "image/webp"}

// AvatarError is an upload refused with a specific HTTP status (413 or 415).
type AvatarError struct {
	Status int
	Msg    string
}

func (e *AvatarError) Error() string { return e.Msg }

// AvatarTooLarge is the 413 for an upload over MaxAvatarBytes.
func AvatarTooLarge() error {
	return &AvatarError{Status: http.StatusRequestEntityTooLarge, Msg: "avatars are limited to 1 MB"}
}

func unsupported(format string, args ...any) error {
	return &AvatarError{Status: http.StatusUnsupportedMediaType, Msg: fmt.Sprintf(format, args...)}
}

// AvatarKey is where an avatar's bytes live in the blob bucket. Every upload
// gets a new key, so a replaced avatar is never served from a stale cache.
func AvatarKey(o Owner, persona uuid.UUID) string {
	return "personas/" + o.WorkspaceID.String() + "/" + o.UserID.String() + "/" + persona.String() + "/" + uuid.NewString()
}

// markup catches HTML, SVG and script in a file that otherwise parses as an
// image (in a PNG text chunk, a JPEG comment or a WebP metadata chunk). Each
// token is long enough that compressed pixel data won't match it by chance.
// It's defence in depth: the real control is that avatars are only ever
// served with an image Content-Type, nosniff and a sandbox CSP, so any new
// path that serves these blobs must send the same headers.
var markup = regexp.MustCompile(`(?i)<(?:script|html|svg|iframe|body|head|object|embed|style|meta|link|form|base|img|video|audio|math)[\s/>]|<!doctype\s+html|<\?xml-stylesheet|javascript:`)

// CheckAvatar validates an uploaded avatar and returns its sniffed content
// type. Empty is a 422 on file, over 1 MB a 413, anything that isn't a
// clean PNG, JPEG or WebP a 415 (*AvatarError).
func CheckAvatar(b []byte) (string, error) {
	if len(b) == 0 {
		return "", apperr.Invalid("file", "the file is empty")
	}
	if len(b) > MaxAvatarBytes {
		return "", AvatarTooLarge()
	}
	ct := http.DetectContentType(b)
	var err error
	switch ct {
	case "image/png":
		err = checkPNG(b)
	case "image/jpeg":
		err = checkJPEG(b)
	case "image/webp":
		err = checkWebP(b)
	default:
		return "", unsupported("avatars must be PNG, JPEG or WebP images")
	}
	if err != nil {
		return "", err
	}
	if markup.Match(b) {
		return "", unsupported("the image contains markup or script; export it again as a plain PNG, JPEG or WebP")
	}
	return ct, nil
}

func malformed(kind string) error {
	return unsupported("the file isn't a valid %s image", kind)
}

func trailing(kind string) error {
	return unsupported("the file has extra data after the %s image", kind)
}

// checkDecoded decodes the pixels after checking the dimensions, so the image
// is real and its decode is bounded.
func checkDecoded(kind string, b []byte, config func([]byte) (image.Config, error), decode func([]byte) error) error {
	cfg, err := config(b)
	if err != nil {
		return malformed(kind)
	}
	if cfg.Width < 1 || cfg.Height < 1 || cfg.Width > MaxAvatarSide || cfg.Height > MaxAvatarSide {
		return apperr.Invalid("file", "avatars are limited to %d×%d pixels", MaxAvatarSide, MaxAvatarSide)
	}
	if decode(b) != nil {
		return malformed(kind)
	}
	return nil
}

// checkPNG walks the chunks (lengths and CRCs) from IHDR to an IEND that ends
// the file, then decodes the image.
func checkPNG(b []byte) error {
	const sig = "\x89PNG\r\n\x1a\n"
	if !bytes.HasPrefix(b, []byte(sig)) {
		return malformed("PNG")
	}
	p := len(sig)
	for first := true; ; first = false {
		if len(b)-p < 12 {
			return malformed("PNG")
		}
		n := binary.BigEndian.Uint32(b[p:])
		if uint64(n) > uint64(len(b)-p-12) {
			return malformed("PNG")
		}
		typ := string(b[p+4 : p+8])
		data := p + 8 + int(n)
		if crc32.ChecksumIEEE(b[p+4:data]) != binary.BigEndian.Uint32(b[data:]) {
			return malformed("PNG")
		}
		if first && typ != "IHDR" {
			return malformed("PNG")
		}
		p = data + 4
		if typ == "IEND" {
			break
		}
	}
	if p != len(b) {
		return trailing("PNG")
	}
	return checkDecoded("PNG", b,
		func(b []byte) (image.Config, error) { return png.DecodeConfig(bytes.NewReader(b)) },
		func(b []byte) error { _, err := png.Decode(bytes.NewReader(b)); return err })
}

// checkJPEG walks the marker segments and entropy-coded data from SOI to EOI,
// then decodes the image. Anything after that EOI must itself be one or more
// complete JPEGs: Ultra HDR and gain-map photos append a second image there.
func checkJPEG(b []byte) error {
	end, err := walkJPEG(b)
	if err != nil {
		return err
	}
	for p := end; p < len(b); {
		if len(b)-p < 2 || b[p] != 0xFF || b[p+1] != 0xD8 {
			return trailing("JPEG")
		}
		n, err := walkJPEG(b[p:])
		if err != nil {
			return trailing("JPEG")
		}
		p += n
	}
	return checkDecoded("JPEG", b,
		func(b []byte) (image.Config, error) { return jpeg.DecodeConfig(bytes.NewReader(b)) },
		func(b []byte) error { _, err := jpeg.Decode(bytes.NewReader(b)); return err })
}

// walkJPEG returns the offset just past the EOI of the JPEG that starts b.
func walkJPEG(b []byte) (int, error) {
	if len(b) < 4 || b[0] != 0xFF || b[1] != 0xD8 {
		return 0, malformed("JPEG")
	}
	p := 2
	for {
		if p >= len(b) || b[p] != 0xFF {
			return 0, malformed("JPEG")
		}
		for p < len(b) && b[p] == 0xFF { // fill bytes
			p++
		}
		if p >= len(b) {
			return 0, malformed("JPEG")
		}
		m := b[p]
		p++
		switch {
		case m == 0xD9: // EOI
			return p, nil
		case m == 0x01 || (m >= 0xD0 && m <= 0xD7): // TEM, RSTn: no length
			continue
		case m == 0x00 || m == 0xD8:
			return 0, malformed("JPEG")
		}
		if len(b)-p < 2 {
			return 0, malformed("JPEG")
		}
		n := int(binary.BigEndian.Uint16(b[p:]))
		if n < 2 || n > len(b)-p {
			return 0, malformed("JPEG")
		}
		p += n
		if m == 0xDA { // SOS: skip entropy-coded data up to the next real marker
			for p < len(b)-1 && (b[p] != 0xFF || b[p+1] == 0x00 || (b[p+1] >= 0xD0 && b[p+1] <= 0xD7)) {
				p++
			}
			if p >= len(b)-1 {
				return 0, malformed("JPEG")
			}
		}
	}
}

// checkWebP walks the RIFF chunks: the RIFF size must match the file and the
// chunks must tile it exactly, starting with an image chunk. The standard
// library has no WebP decoder; the server never decodes it, the browser does.
func checkWebP(b []byte) error {
	if len(b) < 20 || string(b[0:4]) != "RIFF" || string(b[8:12]) != "WEBP" {
		return malformed("WebP")
	}
	if size := binary.LittleEndian.Uint32(b[4:]); uint64(size)+8 != uint64(len(b)) {
		if uint64(size)+8 < uint64(len(b)) {
			return trailing("WebP")
		}
		return malformed("WebP")
	}
	p := 12
	for first := true; p < len(b); first = false {
		if len(b)-p < 8 {
			return malformed("WebP")
		}
		fourcc := string(b[p : p+4])
		n := uint64(binary.LittleEndian.Uint32(b[p+4:]))
		if first && fourcc != "VP8 " && fourcc != "VP8L" && fourcc != "VP8X" {
			return malformed("WebP")
		}
		end := uint64(p) + 8 + n + n&1
		if end > uint64(len(b)) {
			return malformed("WebP")
		}
		p = int(end)
	}
	return nil
}
