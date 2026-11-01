package imaging

import (
	"image"
	"math"
)

// Palette is how bright (0–255) each device gray level actually looks. An ideal panel has evenly
// spaced levels (LinearPalette); real e-ink panels don't — the TRMNL X drives its 16 grays with a
// hand-tuned waveform table, so some neighbours look alike. Quantizing and dithering against the
// measured palette spreads detail over the levels that really differ, which reduces banding.
type Palette []float64

func LinearPalette(bitDepth int) Palette {
	n := 1 << bitDepth
	p := make(Palette, n)
	for i := range p {
		p[i] = float64(i) * 255 / float64(n-1)
	}
	return p
}

// nearest returns the level whose brightness is closest to v (levels need not be monotonic).
func (p Palette) nearest(v float64) int {
	best, bestD := 0, math.Inf(1)
	for i, b := range p {
		if d := math.Abs(v - b); d < bestD {
			best, bestD = i, d
		}
	}
	return best
}

// Emulate shows level indexes the way the panel displays them (for previews).
func (p Palette) Emulate(levels *image.Gray) *image.Gray {
	out := image.NewGray(levels.Bounds())
	for i, l := range levels.Pix {
		out.Pix[i] = uint8(math.Round(p[min(int(l), len(p)-1)]))
	}
	return out
}

// QuantizeTo maps every pixel to the palette level that looks closest, optionally dithering
// with the error measured against the palette's real brightnesses. The result holds level
// indexes, ready for EncodePNG.
func QuantizeTo(src *image.Gray, p Palette, dither Dither) *image.Gray {
	w, h := src.Bounds().Dx(), src.Bounds().Dy()
	out := image.NewGray(image.Rect(0, 0, w, h))
	at := func(x, y int) float64 { return float64(src.Pix[y*src.Stride+x]) }

	// Exact lookups for undithered input (every value is an integer 0–255).
	var lut [256]uint8
	for v := range lut {
		lut[v] = uint8(p.nearest(float64(v)))
	}

	switch dither {
	case DitherFloydSteinberg:
		diffuse(out, w, h, at, p, floydSteinberg)
	case DitherAtkinson:
		diffuse(out, w, h, at, p, atkinson)
	case DitherOrdered:
		step := 255.0 / float64(len(p)-1) // average spacing; good enough for the threshold offset
		for y := range h {
			for x := range w {
				t := (float64(bayer8[y%8][x%8])+0.5)/64 - 0.5
				out.Pix[y*out.Stride+x] = uint8(p.nearest(at(x, y) + t*step))
			}
		}
	default:
		for y := range h {
			row := src.Pix[y*src.Stride : y*src.Stride+w]
			for x, v := range row {
				out.Pix[y*out.Stride+x] = lut[v]
			}
		}
	}
	return out
}
