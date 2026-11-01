package imaging

import (
	"image"
	"testing"
)

func TestRotate(t *testing.T) {
	// 3×2:  0 1 2
	//       3 4 5
	src := image.NewGray(image.Rect(0, 0, 3, 2))
	copy(src.Pix, []uint8{0, 1, 2, 3, 4, 5})
	cases := map[int][]uint8{
		90:  {3, 0, 4, 1, 5, 2}, // 2×3
		180: {5, 4, 3, 2, 1, 0},
		270: {2, 5, 1, 4, 0, 3}, // 2×3
	}
	for deg, want := range cases {
		got := Rotate(src, deg)
		if string(got.Pix) != string(want) {
			t.Errorf("%d°: %v, want %v", deg, got.Pix, want)
		}
		if deg != 180 && (got.Bounds().Dx() != 2 || got.Bounds().Dy() != 3) {
			t.Errorf("%d°: size %v", deg, got.Bounds())
		}
	}
	if Rotate(src, 0) != src {
		t.Error("0° should return the source")
	}
}
