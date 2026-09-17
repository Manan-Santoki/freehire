package headshot

import (
	"bytes"
	"testing"
)

func TestBlur_ProducesADifferentValidJPEGOfTheSameSize(t *testing.T) {
	input := encode(t, bands(outputEdge, outputEdge, false, red, green, blue), "jpeg")

	out, err := Blur(input)
	if err != nil {
		t.Fatalf("Blur: %v", err)
	}
	if bytes.Equal(out, input) {
		t.Fatal("Blur returned the input unchanged")
	}

	img := decodeResult(t, out)
	b := img.Bounds()
	if b.Dx() != outputEdge || b.Dy() != outputEdge {
		t.Errorf("blurred size = %dx%d, want %dx%d", b.Dx(), b.Dy(), outputEdge, outputEdge)
	}
}

// TestBlur_SoftensASharpEdge is the actual evidence the transform blurs rather than
// merely re-encoding. A pixel far enough from a hard band boundary to be outside plain
// JPEG's own DCT-ringing radius (an ordinary re-encode with no blur at all already
// blends the few pixels right AT a hard edge — that alone would not catch a regression
// that quietly turned Blur into a no-op) must still carry a visible blend of both
// neighbouring colours, because the transform's kernel reaches that far.
func TestBlur_SoftensASharpEdge(t *testing.T) {
	input := encode(t, bands(outputEdge, outputEdge, false, red, green, blue), "jpeg")

	out, err := Blur(input)
	if err != nil {
		t.Fatalf("Blur: %v", err)
	}
	img := decodeResult(t, out)

	// The red/green boundary sits at outputEdge/3 on the x axis (see bands()). 40px past
	// it is well clear of JPEG's own re-encode artifacts (a handful of pixels at most)
	// but well inside a sigma=30 Gaussian kernel's reach (~3*sigma, so ~90px) — the
	// distance a strong blur must visibly cross and a no-op transform cannot.
	farFromBoundary := outputEdge/3 + 40
	c := img.At(farFromBoundary, outputEdge/2)
	r, g, _, _ := c.RGBA()
	if r == 0 || g == 0 {
		t.Errorf("pixel 40px past the band boundary = %v, want a visible red/green blend (strong blur reaching this far)", c)
	}
}

func TestBlur_RejectsUndecodableInput(t *testing.T) {
	if _, err := Blur([]byte("not an image")); err == nil {
		t.Fatal("Blur accepted undecodable input")
	}
}
