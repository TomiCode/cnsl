package imaging

import (
	"bytes"
	"image"
	"image/png"
	"testing"
)

func gradient(w, h int) *image.Gray {
	img := image.NewGray(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Pix[y*img.Stride+x] = uint8(x * 255 / (w - 1))
		}
	}
	return img
}

func TestEncodePNGRoundTrip(t *testing.T) {
	for _, depth := range []int{1, 2, 4} {
		levels := Quantize(gradient(37, 5), depth, DitherNone) // odd width exercises partial bytes
		var buf bytes.Buffer
		if err := EncodePNG(&buf, levels, depth); err != nil {
			t.Fatalf("depth %d: encode: %v", depth, err)
		}
		decoded, err := png.Decode(&buf)
		if err != nil {
			t.Fatalf("depth %d: decode: %v", depth, err)
		}
		want := Expand(levels, depth)
		for y := range 5 {
			for x := range 37 {
				got := decoded.(*image.Gray).GrayAt(x, y).Y
				if got != want.GrayAt(x, y).Y {
					t.Fatalf("depth %d: pixel (%d,%d) = %d, want %d", depth, x, y, got, want.GrayAt(x, y).Y)
				}
			}
		}
	}
}

func TestQuantizeLevels(t *testing.T) {
	levels := Quantize(gradient(256, 1), 4, DitherNone)
	if levels.Pix[0] != 0 || levels.Pix[255] != 15 {
		t.Fatalf("ends = %d, %d; want 0, 15", levels.Pix[0], levels.Pix[255])
	}
}

func TestDitherPreservesAverage(t *testing.T) {
	src := image.NewGray(image.Rect(0, 0, 64, 64))
	for i := range src.Pix {
		src.Pix[i] = 100
	}
	out := Expand(Quantize(src, 1, DitherFloydSteinberg), 1)
	sum := 0
	for _, v := range out.Pix {
		sum += int(v)
	}
	if avg := sum / len(out.Pix); avg < 90 || avg > 110 {
		t.Fatalf("dithered average = %d, want ≈100", avg)
	}
}

func TestResizeFit(t *testing.T) {
	src := image.NewGray(image.Rect(0, 0, 200, 100)) // black, 2:1
	contain := Resize(src, 100, 100, FitContain)
	if contain.GrayAt(50, 10).Y != 0xff || contain.GrayAt(50, 50).Y != 0 {
		t.Fatal("contain: expected white bars top/bottom and black center")
	}
	cover := Resize(src, 100, 100, FitCover)
	if cover.GrayAt(50, 2).Y != 0 {
		t.Fatal("cover: expected the image to fill the canvas")
	}
}
