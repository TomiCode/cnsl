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
)

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

// Quantize maps every pixel to one of the 2^bitDepth gray levels. The returned image holds
// level indexes (0 = black … levels-1 = white), ready for EncodePNG.
func Quantize(src *image.Gray, bitDepth int, dither Dither) *image.Gray {
	levels := 1 << bitDepth
	step := 255.0 / float64(levels-1)
	w, h := src.Bounds().Dx(), src.Bounds().Dy()
	out := image.NewGray(image.Rect(0, 0, w, h))

	nearest := func(v float64) int {
		return min(max(int(v/step+0.5), 0), levels-1)
	}

	if dither != DitherFloydSteinberg {
		for y := range h {
			for x := range w {
				out.Pix[y*out.Stride+x] = uint8(nearest(float64(src.Pix[y*src.Stride+x])))
			}
		}
		return out
	}

	// Floyd–Steinberg error diffusion with two rolling rows of accumulated error.
	cur, next := make([]float64, w+2), make([]float64, w+2)
	for y := range h {
		for x := range w {
			v := float64(src.Pix[y*src.Stride+x]) + cur[x+1]
			level := nearest(v)
			out.Pix[y*out.Stride+x] = uint8(level)
			e := v - float64(level)*step
			cur[x+2] += e * 7 / 16
			next[x] += e * 3 / 16
			next[x+1] += e * 5 / 16
			next[x+2] += e * 1 / 16
		}
		cur, next = next, cur
		clear(next)
	}
	return out
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
