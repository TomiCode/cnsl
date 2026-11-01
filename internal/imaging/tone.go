package imaging

import (
	"encoding/json"
	"fmt"
	"image"
	"math"
)

// ToneCurve corrects for e-ink gray levels not being evenly spaced to the eye. It maps input
// luma before quantization:
//
//	x = clamp((in/255 - Black) / (White - Black)),  out = 255 · x^Gamma
//
// Inputs at or below Black become black, at or above White become white. Gamma > 1 darkens the
// mid-tones, < 1 lightens them. The zero value is treated as linear.
//
// Levels, when set, is the measured brightness (0 = black … 1 = white) of each device gray level
// as it looks on the panel; quantization and dithering then target those real levels (Palette).
type ToneCurve struct {
	Gamma  float64   `json:"gamma"`
	Black  float64   `json:"black"`
	White  float64   `json:"white"`
	Levels []float64 `json:"levels,omitempty"`
}

var LinearCurve = ToneCurve{Gamma: 1, Black: 0, White: 1}

func (c ToneCurve) normalized() ToneCurve {
	if c.Gamma == 0 && c.Black == 0 && c.White == 0 {
		c.Gamma, c.White = 1, 1
	}
	return c
}

// Validate checks the curve for a panel with the given bit depth.
func (c ToneCurve) Validate(bitDepth int) error {
	c = c.normalized()
	if c.Levels != nil {
		if len(c.Levels) != 1<<bitDepth {
			return fmt.Errorf("levels must list %d brightness values, got %d", 1<<bitDepth, len(c.Levels))
		}
		for i, v := range c.Levels {
			if math.IsNaN(v) || v < 0 || v > 1 {
				return fmt.Errorf("level %d brightness must be between 0 and 1", i)
			}
		}
	}
	if c.Gamma < 0.2 || c.Gamma > 5 {
		return fmt.Errorf("gamma must be between 0.2 and 5")
	}
	if c.Black < 0 || c.White > 1 || c.White-c.Black < 0.2 {
		return fmt.Errorf("black and white points must satisfy 0 ≤ black, white ≤ 1, white − black ≥ 0.2")
	}
	return nil
}

// Palette returns the brightness of each level for quantization: the measured levels when they
// match the bit depth, evenly spaced otherwise.
func (c ToneCurve) Palette(bitDepth int) Palette {
	if len(c.Levels) != 1<<bitDepth {
		return LinearPalette(bitDepth)
	}
	p := make(Palette, len(c.Levels))
	for i, v := range c.Levels {
		p[i] = v * 255
	}
	return p
}

// LUT returns the 256-entry lookup table for the curve.
func (c ToneCurve) LUT() [256]uint8 {
	c = c.normalized()
	var lut [256]uint8
	for i := range lut {
		x := (float64(i)/255 - c.Black) / (c.White - c.Black)
		x = math.Min(math.Max(x, 0), 1)
		lut[i] = uint8(math.Round(255 * math.Pow(x, c.Gamma)))
	}
	return lut
}

// Apply returns a copy of src with the curve applied.
func (c ToneCurve) Apply(src *image.Gray) *image.Gray {
	lut := c.LUT()
	out := image.NewGray(src.Bounds())
	for i, v := range src.Pix {
		out.Pix[i] = lut[v]
	}
	return out
}

// ParseToneCurve reads a curve stored as JSON; empty means linear.
func ParseToneCurve(s string) (ToneCurve, error) {
	if s == "" {
		return LinearCurve, nil
	}
	var c ToneCurve
	if err := json.Unmarshal([]byte(s), &c); err != nil {
		return LinearCurve, err
	}
	return c.normalized(), nil
}

// LevelValues returns, for each of the 2^bitDepth levels, an 8-bit gray value that the render
// pipeline (curve, then nearest level) maps back to exactly that level. Pre-dithered images
// placed in a page are stored with these values so the final pass doesn't shift them. A level
// the curve can't produce (clipped by the black/white point) uses the closest one it can.
func (c ToneCurve) LevelValues(bitDepth int) []uint8 {
	levels := 1 << bitDepth
	pal := c.Palette(bitDepth)
	lut := c.LUT()
	first, last := make([]int, levels), make([]int, levels)
	for i := range first {
		first[i] = -1
	}
	for v := range 256 {
		l := pal.nearest(float64(lut[v]))
		if first[l] < 0 {
			first[l] = v
		}
		last[l] = v
	}
	out := make([]uint8, levels)
	for l := range levels {
		best := -1
		for d := 0; d < levels && best < 0; d++ {
			for _, cand := range []int{l - d, l + d} {
				if cand >= 0 && cand < levels && first[cand] >= 0 {
					best = cand
					break
				}
			}
		}
		out[l] = uint8((first[best] + last[best]) / 2)
	}
	return out
}

// Prepare turns any image into level indexes for a w×h area: grayscale, fit, tone curve, dither.
func Prepare(src image.Image, w, h int, fit Fit, dither Dither, curve ToneCurve, bitDepth int) *image.Gray {
	return QuantizeTo(curve.Apply(Resize(Gray(src), w, h, fit)), curve.Palette(bitDepth), dither)
}

// ForPage converts level indexes into 8-bit gray for embedding in a rendered page, using
// LevelValues so the pipeline's final curve + quantization reproduces the same levels.
func ForPage(levels *image.Gray, curve ToneCurve, bitDepth int) *image.Gray {
	values := curve.LevelValues(bitDepth)
	out := image.NewGray(levels.Bounds())
	for i, l := range levels.Pix {
		out.Pix[i] = values[min(int(l), len(values)-1)]
	}
	return out
}
