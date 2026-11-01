// Package imaging prepares images for e-ink panels: grayscale conversion, resizing,
// quantization to the panel's gray levels (optionally dithered) and low-bit-depth PNG encoding.
package imaging

import (
	"image"
	"image/color"
	"image/draw"
)

type Fit string

const (
	FitContain Fit = "contain" // whole image visible, white bars
	FitCover   Fit = "cover"   // fill the screen, crop the overflow (centered)
)

type Dither string

const (
	DitherNone           Dither = "none"
	DitherFloydSteinberg Dither = "floyd-steinberg"
	DitherAtkinson       Dither = "atkinson" // lighter, higher-contrast; good for 1–2 bit
	DitherOrdered        Dither = "ordered"  // Bayer 8×8; stable between renders (slideshows)
)

// ValidDither reports whether d is a known dithering method.
func ValidDither(d Dither) bool {
	switch d {
	case DitherNone, DitherFloydSteinberg, DitherAtkinson, DitherOrdered:
		return true
	}
	return false
}

// Gray converts any image to 8-bit grayscale (luma). Transparent areas become white.
func Gray(src image.Image) *image.Gray {
	b := src.Bounds()
	flat := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(flat, flat.Bounds(), image.White, image.Point{}, draw.Src)
	draw.Draw(flat, flat.Bounds(), src, b.Min, draw.Over)

	out := image.NewGray(flat.Bounds())
	for y := range b.Dy() {
		for x := range b.Dx() {
			out.SetGray(x, y, color.GrayModel.Convert(flat.RGBAAt(x, y)).(color.Gray))
		}
	}
	return out
}

// Resize scales src into a w×h canvas using fit.
func Resize(src *image.Gray, w, h int, fit Fit) *image.Gray {
	sw, sh := src.Bounds().Dx(), src.Bounds().Dy()
	dst := image.NewGray(image.Rect(0, 0, w, h))
	for i := range dst.Pix {
		dst.Pix[i] = 0xff
	}
	if sw == 0 || sh == 0 {
		return dst
	}

	scaleX, scaleY := float64(w)/float64(sw), float64(h)/float64(sh)
	scale := min(scaleX, scaleY)
	if fit == FitCover {
		scale = max(scaleX, scaleY)
	}
	tw, th := max(1, int(float64(sw)*scale+0.5)), max(1, int(float64(sh)*scale+0.5))
	scaled := scaleGray(src, tw, th)

	// Center the scaled image; for cover this crops equally on both sides.
	offset := image.Pt((w-tw)/2, (h-th)/2)
	draw.Draw(dst, dst.Bounds(), scaled, image.Point{}.Sub(offset), draw.Src)
	return dst
}

// scaleGray resamples src to w×h: area averaging when shrinking, bilinear when enlarging.
func scaleGray(src *image.Gray, w, h int) *image.Gray {
	sw, sh := src.Bounds().Dx(), src.Bounds().Dy()
	if w <= sw && h <= sh {
		return shrink(src, w, h)
	}
	return enlarge(src, w, h)
}

func shrink(src *image.Gray, w, h int) *image.Gray {
	sw, sh := src.Bounds().Dx(), src.Bounds().Dy()
	dst := image.NewGray(image.Rect(0, 0, w, h))
	for y := range h {
		y0, y1 := y*sh/h, max((y+1)*sh/h, y*sh/h+1)
		for x := range w {
			x0, x1 := x*sw/w, max((x+1)*sw/w, x*sw/w+1)
			sum, n := 0, 0
			for sy := y0; sy < y1; sy++ {
				row := src.Pix[sy*src.Stride:]
				for sx := x0; sx < x1; sx++ {
					sum += int(row[sx])
					n++
				}
			}
			dst.Pix[y*dst.Stride+x] = uint8((sum + n/2) / n)
		}
	}
	return dst
}

func enlarge(src *image.Gray, w, h int) *image.Gray {
	sw, sh := src.Bounds().Dx(), src.Bounds().Dy()
	dst := image.NewGray(image.Rect(0, 0, w, h))
	at := func(x, y int) float64 {
		x, y = min(max(x, 0), sw-1), min(max(y, 0), sh-1)
		return float64(src.Pix[y*src.Stride+x])
	}
	for y := range h {
		fy := (float64(y)+0.5)*float64(sh)/float64(h) - 0.5
		y0 := int(fy)
		if fy < 0 {
			y0 = -1
		}
		dy := fy - float64(y0)
		for x := range w {
			fx := (float64(x)+0.5)*float64(sw)/float64(w) - 0.5
			x0 := int(fx)
			if fx < 0 {
				x0 = -1
			}
			dx := fx - float64(x0)
			top := at(x0, y0)*(1-dx) + at(x0+1, y0)*dx
			bottom := at(x0, y0+1)*(1-dx) + at(x0+1, y0+1)*dx
			dst.Pix[y*dst.Stride+x] = uint8(top*(1-dy) + bottom*dy + 0.5)
		}
	}
	return dst
}

// Quantize maps every pixel to one of 2^bitDepth evenly spaced gray levels (see QuantizeTo for
// measured, uneven levels). The returned image holds level indexes, ready for EncodePNG.
func Quantize(src *image.Gray, bitDepth int, dither Dither) *image.Gray {
	return QuantizeTo(src, LinearPalette(bitDepth), dither)
}

// kernel lists error-diffusion targets as (dx, dy, weight).
type kernel struct {
	taps    [][3]int
	divisor float64
}

var (
	floydSteinberg = kernel{taps: [][3]int{{1, 0, 7}, {-1, 1, 3}, {0, 1, 5}, {1, 1, 1}}, divisor: 16}
	// Atkinson spreads only 6/8 of the error, which keeps highlights and shadows clean.
	atkinson = kernel{taps: [][3]int{{1, 0, 1}, {2, 0, 1}, {-1, 1, 1}, {0, 1, 1}, {1, 1, 1}, {0, 2, 1}}, divisor: 8}
)

func diffuse(out *image.Gray, w, h int, at func(x, y int) float64, p Palette, k kernel) {
	const pad = 2
	rows := [3][]float64{make([]float64, w+2*pad), make([]float64, w+2*pad), make([]float64, w+2*pad)}
	for y := range h {
		for x := range w {
			v := at(x, y) + rows[0][x+pad]
			level := p.nearest(v)
			out.Pix[y*out.Stride+x] = uint8(level)
			e := v - p[level] // error against what the panel really shows
			for _, t := range k.taps {
				rows[t[1]][x+pad+t[0]] += e * float64(t[2]) / k.divisor
			}
		}
		rows[0], rows[1], rows[2] = rows[1], rows[2], rows[0]
		clear(rows[2])
	}
}

var bayer8 = [8][8]int{
	{0, 32, 8, 40, 2, 34, 10, 42},
	{48, 16, 56, 24, 50, 18, 58, 26},
	{12, 44, 4, 36, 14, 46, 6, 38},
	{60, 28, 52, 20, 62, 30, 54, 22},
	{3, 35, 11, 43, 1, 33, 9, 41},
	{51, 19, 59, 27, 49, 17, 57, 25},
	{15, 47, 7, 39, 13, 45, 5, 37},
	{63, 31, 55, 23, 61, 29, 53, 21},
}

// Expand turns level indexes back into 8-bit gray, e.g. for previews.
func Expand(levels *image.Gray, bitDepth int) *image.Gray {
	maxLevel := (1 << bitDepth) - 1
	out := image.NewGray(levels.Bounds())
	for i, l := range levels.Pix {
		out.Pix[i] = uint8(int(l) * 255 / maxLevel)
	}
	return out
}
