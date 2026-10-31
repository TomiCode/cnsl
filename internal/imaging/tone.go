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
type ToneCurve struct {
	Gamma float64 `json:"gamma"`
	Black float64 `json:"black"`
	White float64 `json:"white"`
}

var LinearCurve = ToneCurve{Gamma: 1, Black: 0, White: 1}

func (c ToneCurve) normalized() ToneCurve {
	if c == (ToneCurve{}) {
		return LinearCurve
	}
	return c
}

func (c ToneCurve) Validate() error {
	c = c.normalized()
	if c.Gamma < 0.2 || c.Gamma > 5 {
		return fmt.Errorf("gamma must be between 0.2 and 5")
	}
	if c.Black < 0 || c.White > 1 || c.White-c.Black < 0.2 {
		return fmt.Errorf("black and white points must satisfy 0 ≤ black, white ≤ 1, white − black ≥ 0.2")
	}
	return nil
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
