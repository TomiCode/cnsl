package imaging

import "image"

// Rotate turns an image clockwise by 0, 90, 180 or 270 degrees (other values return it as is).
func Rotate(src *image.Gray, degrees int) *image.Gray {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	at := func(x, y int) uint8 { return src.Pix[y*src.Stride+x] }
	var dst *image.Gray
	switch degrees {
	case 90: // src (x, y) → dst (h-1-y, x)
		dst = image.NewGray(image.Rect(0, 0, h, w))
		for dy := range w {
			row := dst.Pix[dy*dst.Stride:]
			for dx := range h {
				row[dx] = at(dy, h-1-dx)
			}
		}
	case 180:
		dst = image.NewGray(image.Rect(0, 0, w, h))
		for dy := range h {
			row := dst.Pix[dy*dst.Stride:]
			for dx := range w {
				row[dx] = at(w-1-dx, h-1-dy)
			}
		}
	case 270: // src (x, y) → dst (y, w-1-x)
		dst = image.NewGray(image.Rect(0, 0, h, w))
		for dy := range w {
			row := dst.Pix[dy*dst.Stride:]
			for dx := range h {
				row[dx] = at(w-1-dy, dx)
			}
		}
	default:
		return src
	}
	return dst
}
