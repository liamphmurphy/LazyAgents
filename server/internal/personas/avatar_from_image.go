package personas

import (
	"bytes"
	"image"
	"image/color"
	_ "image/gif" // generated images may be GIFs
	"image/jpeg"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp" // generated images may be WebP

	"github.com/liamphmurphy/vesper/server/internal/apperr"
)

const (
	// AvatarSide is the largest edge of an avatar made from an image.
	AvatarSide = 512
	// MaxAvatarSourceBytes bounds the image AvatarFromImage is given; generated
	// images are well under it.
	MaxAvatarSourceBytes = 20 << 20
	// maxAvatarSourceSide bounds a source image's decoded size, so a small but
	// highly compressed file can't make the server allocate gigabytes.
	maxAvatarSourceSide = 4096
)

// AvatarFromImage makes an avatar from an image the server already holds, such
// as one generated in a chat: the centre square, scaled down to at most
// AvatarSide and re-encoded as JPEG on white. A generated image is often over
// MaxAvatarBytes, which an avatar made this way never is; re-encoding also
// drops any metadata or trailing data the source carried. The result passes
// CheckAvatar, which is checked again as defence in depth.
func AvatarFromImage(src []byte) ([]byte, string, error) {
	if len(src) == 0 || len(src) > MaxAvatarSourceBytes {
		return nil, "", apperr.Invalid("image", "must be 1 byte to %d MB", MaxAvatarSourceBytes>>20)
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(src))
	if err != nil {
		return nil, "", apperr.Invalid("image", "not a PNG, JPEG, WebP or GIF image")
	}
	if cfg.Width < 1 || cfg.Height < 1 || cfg.Width > maxAvatarSourceSide || cfg.Height > maxAvatarSourceSide {
		return nil, "", apperr.Invalid("image", "must be at most %d×%d pixels", maxAvatarSourceSide, maxAvatarSourceSide)
	}
	img, _, err := image.Decode(bytes.NewReader(src))
	if err != nil {
		return nil, "", apperr.Invalid("image", "not a valid image")
	}
	b := img.Bounds()
	edge := min(b.Dx(), b.Dy())
	x0, y0 := b.Min.X+(b.Dx()-edge)/2, b.Min.Y+(b.Dy()-edge)/2
	crop := image.Rect(x0, y0, x0+edge, y0+edge)
	side := min(edge, AvatarSide)
	dst := image.NewRGBA(image.Rect(0, 0, side, side))
	draw.Draw(dst, dst.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	draw.CatmullRom.Scale(dst, dst.Bounds(), img, crop, draw.Over, nil)
	var out bytes.Buffer
	if err := jpeg.Encode(&out, dst, &jpeg.Options{Quality: 90}); err != nil {
		return nil, "", err
	}
	ct, err := CheckAvatar(out.Bytes())
	if err != nil {
		return nil, "", err
	}
	return out.Bytes(), ct, nil
}
