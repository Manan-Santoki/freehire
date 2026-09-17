package headshot

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"

	"github.com/disintegration/imaging"
)

// blurSigma is the Gaussian kernel's standard deviation applied to a stored headshot
// before it is ever served publicly (imaging.Blur's sigma parameter — not a pixel
// radius; the kernel it drives reaches roughly 3*sigma, ~90px on the 512 px square
// Normalize produces). Large enough that no facial feature survives as a distinct shape
// — chosen by looking at the result, not derived from a formula, since "unrecognizable"
// is a visual judgment. Fixed, not configurable: there is no deployment reason to tune
// how anonymous a public photo is.
const blurSigma = 30

// Blur returns a strongly, irreversibly blurred rendering of a stored headshot. It never
// returns the input unchanged and never partially blurs — every public caller gets the
// same fixed transformation. Used only on the read path a public route serves; the
// stored original is never touched.
func Blur(data []byte) ([]byte, error) {
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnsupportedImage, err)
	}

	blurred := imaging.Blur(src, blurSigma)

	var out bytes.Buffer
	if err := jpeg.Encode(&out, blurred, &jpeg.Options{Quality: jpegQuality}); err != nil {
		return nil, fmt.Errorf("headshot: encode blurred: %w", err)
	}
	return out.Bytes(), nil
}
