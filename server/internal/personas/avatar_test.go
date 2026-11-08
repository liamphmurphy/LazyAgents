package personas

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"net/http"
	"testing"

	"github.com/liamphmurphy/vesper/server/internal/apperr"
)

func testImage(w, h int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for x := range w {
		for y := range h {
			img.Set(x, y, color.RGBA{uint8(x * 7), uint8(y * 13), 90, 255})
		}
	}
	return img
}

// pngFixture and jpegFixture are small, valid images.
func pngFixture(t testing.TB, w, h int) []byte {
	var b bytes.Buffer
	if err := png.Encode(&b, testImage(w, h)); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func jpegFixture(t testing.TB) []byte {
	var b bytes.Buffer
	if err := jpeg.Encode(&b, testImage(16, 16), nil); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// A 1×1 lossless WebP.
var testWebP, _ = base64.StdEncoding.DecodeString("UklGRhoAAABXRUJQVlA4TA0AAAAvAAAAEAcQERGIiP4HAA==")

// withPNGChunk inserts a valid chunk right after IHDR.
func withPNGChunk(t *testing.T, img []byte, typ string, data []byte) []byte {
	t.Helper()
	ihdrEnd := 8 + 12 + int(binary.BigEndian.Uint32(img[8:]))
	var c bytes.Buffer
	binary.Write(&c, binary.BigEndian, uint32(len(data)))
	c.WriteString(typ)
	c.Write(data)
	binary.Write(&c, binary.BigEndian, crc32.ChecksumIEEE(append([]byte(typ), data...)))
	out := append([]byte{}, img[:ihdrEnd]...)
	out = append(out, c.Bytes()...)
	return append(out, img[ihdrEnd:]...)
}

// withJPEGComment inserts a COM segment right after SOI.
func withJPEGComment(img []byte, text string) []byte {
	seg := []byte{0xFF, 0xFE, 0, 0}
	binary.BigEndian.PutUint16(seg[2:], uint16(len(text)+2))
	out := append([]byte{}, img[:2]...)
	out = append(out, seg...)
	out = append(out, text...)
	return append(out, img[2:]...)
}

func status(err error) int {
	var ae *AvatarError
	var ve *apperr.ValidationError
	switch {
	case err == nil:
		return http.StatusOK
	case errors.As(err, &ae):
		return ae.Status
	case errors.As(err, &ve):
		return http.StatusUnprocessableEntity
	}
	return 0
}

func TestCheckAvatar(t *testing.T) {
	pngImg, jpegImg := pngFixture(t, 8, 8), jpegFixture(t)
	var gifBuf bytes.Buffer
	if err := gif.Encode(&gifBuf, testImage(4, 4), nil); err != nil {
		t.Fatal(err)
	}
	const html = "<html><body><script>alert(document.cookie)</script></body></html>"
	// A WebP with a metadata chunk carrying script.
	webpXSS := append([]byte{}, testWebP...)
	meta := []byte("<svg onload=alert(1)>")
	chunk := append([]byte("XMP "), 0, 0, 0, 0)
	binary.LittleEndian.PutUint32(chunk[4:], uint32(len(meta)))
	chunk = append(chunk, meta...)
	if len(meta)%2 == 1 {
		chunk = append(chunk, 0)
	}
	webpXSS = append(webpXSS, chunk...)
	binary.LittleEndian.PutUint32(webpXSS[4:], uint32(len(webpXSS)-8))
	// A big PNG past MaxAvatarSide that compresses tiny.
	var bomb bytes.Buffer
	if err := png.Encode(&bomb, image.NewGray(image.Rect(0, 0, MaxAvatarSide+1, 1))); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name string
		b    []byte
		code int
		ct   string
	}{
		{"png", pngImg, 200, "image/png"},
		{"jpeg", jpegImg, 200, "image/jpeg"},
		{"webp", testWebP, 200, "image/webp"},
		{"empty", nil, 422, ""},
		{"over 1 MB", make([]byte, MaxAvatarBytes+1), 413, ""},
		{"svg", []byte(`<svg xmlns="http://www.w3.org/2000/svg" onload="alert(1)"><circle r="4"/></svg>`), 415, ""},
		{"svg with xml prolog", []byte(`<?xml version="1.0"?><svg xmlns="http://www.w3.org/2000/svg"/>`), 415, ""},
		{"html", []byte(html), 415, ""},
		{"gif", gifBuf.Bytes(), 415, ""},
		{"text", []byte("just some text"), 415, ""},
		{"png then html", append(append([]byte{}, pngImg...), html...), 415, ""},
		{"png with script in a text chunk", withPNGChunk(t, pngImg, "tEXt", []byte("Comment\x00"+html)), 415, ""},
		{"png with a bad chunk crc", func() []byte { b := append([]byte{}, pngImg...); b[30] ^= 0xFF; return b }(), 415, ""},
		{"truncated png", pngImg[:len(pngImg)-12], 415, ""},
		{"png header only", pngImg[:40], 415, ""},
		{"jpeg then html", append(append([]byte{}, jpegImg...), html...), 415, ""},
		{"jpeg then html then eoi", append(append(append([]byte{}, jpegImg...), html...), 0xFF, 0xD9), 415, ""},
		{"jpeg with script in a comment", withJPEGComment(jpegImg, html), 415, ""},
		{"truncated jpeg", jpegImg[:len(jpegImg)/2], 415, ""},
		{"jpeg with an appended jpeg (ultra hdr)", append(append([]byte{}, jpegImg...), jpegImg...), 200, "image/jpeg"},
		{"jpeg, jpeg, then html", append(append(append([]byte{}, jpegImg...), jpegImg...), html...), 415, ""},
		{"jpeg then a truncated jpeg", append(append([]byte{}, jpegImg...), jpegImg[:len(jpegImg)/2]...), 415, ""},
		{"png with an img onerror chunk", withPNGChunk(t, pngImg, "tEXt", []byte("c\x00<img src=x onerror=alert(1)>")), 415, ""},
		{"webp then html", append(append([]byte{}, testWebP...), html...), 415, ""},
		{"webp with script in a chunk", webpXSS, 415, ""},
		{"png over the size cap", bomb.Bytes(), 422, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ct, err := CheckAvatar(c.b)
			if got := status(err); got != c.code || ct != c.ct {
				t.Fatalf("CheckAvatar = %q, %v (status %d), want %q, %d", ct, err, got, c.ct, c.code)
			}
		})
	}
	// A plain JPEG comment (no markup) is fine.
	if _, err := CheckAvatar(withJPEGComment(jpegImg, "taken on holiday")); err != nil {
		t.Fatalf("benign comment: %v", err)
	}
}
