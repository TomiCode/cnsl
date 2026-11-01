package imaging

import (
	"bytes"
	"image"
	"image/png"
	"math"
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

func TestToneCurve(t *testing.T) {
	lut := LinearCurve.LUT()
	if lut[0] != 0 || lut[128] != 128 || lut[255] != 255 {
		t.Fatalf("linear lut wrong: %d %d %d", lut[0], lut[128], lut[255])
	}
	clipped := ToneCurve{Gamma: 1, Black: 0.1, White: 0.9}.LUT()
	if clipped[20] != 0 || clipped[240] != 255 {
		t.Fatalf("clipping wrong: %d %d", clipped[20], clipped[240])
	}
	dark := ToneCurve{Gamma: 2, Black: 0, White: 1}.LUT()
	if dark[128] >= 128 {
		t.Fatalf("gamma 2 should darken mid-tones, got %d", dark[128])
	}
	if (ToneCurve{Gamma: 1, Black: 0.5, White: 0.6}).Validate(4) == nil {
		t.Fatal("narrow range should be invalid")
	}
}

func TestDitherMethodsPreserveAverage(t *testing.T) {
	src := image.NewGray(image.Rect(0, 0, 64, 64))
	for i := range src.Pix {
		src.Pix[i] = 90
	}
	for _, d := range []Dither{DitherFloydSteinberg, DitherAtkinson, DitherOrdered} {
		out := Expand(Quantize(src, 1, d), 1)
		sum := 0
		for _, v := range out.Pix {
			sum += int(v)
		}
		// Atkinson drops 1/4 of the error, so it may drift a little more.
		if avg := sum / len(out.Pix); avg < 70 || avg > 110 {
			t.Errorf("%s: average %d, want ≈90", d, avg)
		}
	}
}

// Pre-dithered pixels must survive the page pipeline (curve, then nearest level) unchanged.
func TestForPageRoundTrip(t *testing.T) {
	for _, curve := range []ToneCurve{LinearCurve, {Gamma: 1.8, Black: 0.1, White: 0.92}, {Gamma: 0.6, Black: 0, White: 1}} {
		levels := Quantize(gradient(256, 4), 4, DitherFloydSteinberg)
		page := ForPage(levels, curve, 4)
		back := Quantize(curve.Apply(page), 4, DitherNone)
		for i := range levels.Pix {
			if back.Pix[i] != levels.Pix[i] {
				// A level the curve clips away can't round-trip; it must land on a neighbor.
				if d := int(back.Pix[i]) - int(levels.Pix[i]); d < -2 || d > 2 {
					t.Fatalf("curve %+v: level %d came back as %d", curve, levels.Pix[i], back.Pix[i])
				}
			}
		}
	}
	// With a linear curve every level is reachable, so the round trip is exact.
	levels := Quantize(gradient(256, 1), 4, DitherNone)
	back := Quantize(LinearCurve.Apply(ForPage(levels, LinearCurve, 4)), 4, DitherNone)
	for i := range levels.Pix {
		if back.Pix[i] != levels.Pix[i] {
			t.Fatalf("linear: level %d came back as %d", levels.Pix[i], back.Pix[i])
		}
	}
}

// A panel whose levels 1–3 all look nearly black and whose mid-grays bunch together.
func unevenLevels() []float64 {
	return []float64{0, .03, .05, .06, .2, .3, .38, .45, .52, .6, .68, .75, .82, .9, .96, 1}
}

func TestQuantizeToMeasuredPalette(t *testing.T) {
	curve := ToneCurve{Gamma: 1, White: 1, Levels: unevenLevels()}
	if err := curve.Validate(4); err != nil {
		t.Fatal(err)
	}
	if (ToneCurve{Gamma: 1, White: 1, Levels: []float64{0, 1}}).Validate(4) == nil {
		t.Fatal("wrong number of levels should be invalid")
	}
	p := curve.Palette(4)
	src := image.NewGray(image.Rect(0, 0, 1, 1))
	src.Pix[0] = 51 // 20% gray: closest real level is 4 (.2), not level 3 as on an ideal panel
	if got := QuantizeTo(src, p, DitherNone).Pix[0]; got != 4 {
		t.Fatalf("20%% gray → level %d, want 4", got)
	}

	// Dithering against real brightnesses keeps the perceived average.
	flat := image.NewGray(image.Rect(0, 0, 64, 64))
	for i := range flat.Pix {
		flat.Pix[i] = 30
	}
	emulated := p.Emulate(QuantizeTo(flat, p, DitherFloydSteinberg))
	sum := 0
	for _, v := range emulated.Pix {
		sum += int(v)
	}
	if avg := sum / len(emulated.Pix); avg < 26 || avg > 34 {
		t.Fatalf("perceived average %d, want ≈30", avg)
	}

	// Pre-dithered backgrounds still round-trip through the page pipeline.
	levels := QuantizeTo(curve.Apply(gradient(256, 2)), p, DitherFloydSteinberg)
	back := QuantizeTo(curve.Apply(ForPage(levels, curve, 4)), p, DitherNone)
	for i := range levels.Pix {
		if back.Pix[i] != levels.Pix[i] && math.Abs(p[back.Pix[i]]-p[levels.Pix[i]]) > 8 {
			t.Fatalf("level %d came back as %d", levels.Pix[i], back.Pix[i])
		}
	}
}
