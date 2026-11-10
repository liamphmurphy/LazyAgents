package personas

import (
	"bytes"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"math/rand/v2"
	"testing"
)

// noisyPNG is a w×h PNG of random pixels: it barely compresses, like the large
// images a generator returns, so it is far over MaxAvatarBytes.
func noisyPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	r := rand.New(rand.NewPCG(1, 2))
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := range img.Pix {
		img.Pix[i] = uint8(r.IntN(256))
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestAvatarFromImage(t *testing.T) {
	big := noisyPNG(t, 1024, 1536)
	if len(big) <= MaxAvatarBytes {
		t.Fatalf("fixture is %d bytes; it must be over the avatar limit to prove the shrink", len(big))
	}
	var gifBuf bytes.Buffer
	if err := gif.Encode(&gifBuf, testImage(40, 30), nil); err != nil {
		t.Fatal(err)
	}
	// A transparent PNG lands on white, not black.
	clear := image.NewNRGBA(image.Rect(0, 0, 8, 8))
	var clearBuf bytes.Buffer
	if err := png.Encode(&clearBuf, clear); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		in   []byte
		side int
	}{
		{"large portrait png is cropped and shrunk", big, AvatarSide},
		{"small png keeps its size", pngFixture(t, 100, 60), 60},
		{"gif", gifBuf.Bytes(), 30},
		{"webp", testWebP, 1},
		{"transparent png", clearBuf.Bytes(), 8},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, ct, err := AvatarFromImage(tc.in)
			if err != nil {
				t.Fatal(err)
			}
			if ct != "image/jpeg" {
				t.Errorf("content type %q", ct)
			}
			if got, err := CheckAvatar(out); err != nil || got != ct {
				t.Errorf("CheckAvatar = %q, %v", got, err)
			}
			img, err := jpeg.Decode(bytes.NewReader(out))
			if err != nil {
				t.Fatal(err)
			}
			if b := img.Bounds(); b.Dx() != tc.side || b.Dy() != tc.side {
				t.Errorf("size %v, want %d×%d", b.Size(), tc.side, tc.side)
			}
			if tc.name == "transparent png" {
				if r, g, b, _ := img.At(4, 4).RGBA(); r>>8 < 250 || g>>8 < 250 || b>>8 < 250 {
					t.Errorf("transparent pixel became %v, want white", color.RGBA{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8), 255})
				}
			}
		})
	}
}

func TestAvatarFromImageRefuses(t *testing.T) {
	// A header claiming 5000×10: refused before any pixels are decoded.
	wide := pngFixture(t, 5000, 1)
	cases := map[string][]byte{
		"empty":     nil,
		"not image": []byte("<html><script>alert(1)</script></html>"),
		"too wide":  wide,
		"truncated": pngFixture(t, 64, 64)[:60],
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			if _, _, err := AvatarFromImage(in); err == nil {
				t.Fatal("accepted")
			}
		})
	}
}
