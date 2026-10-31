package imaging

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"regexp"
)

// Files stores device images under content-hash names, so identical images share one file
// and the firmware can skip downloads and refreshes when the name doesn't change.
type Files struct {
	Dir string
}

var filenamePattern = regexp.MustCompile(`^[0-9a-f]{32}\.png$`)

// ValidName reports whether name looks like a file produced by Save.
func ValidName(name string) bool {
	return filenamePattern.MatchString(name)
}

// Save encodes levels as a PNG and stores it, returning its filename.
func (f Files) Save(levels *image.Gray, bitDepth int) (string, error) {
	var buf bytes.Buffer
	if err := EncodePNG(&buf, levels, bitDepth); err != nil {
		return "", err
	}
	sum := sha256.Sum256(buf.Bytes())
	name := hex.EncodeToString(sum[:16]) + ".png"
	path := filepath.Join(f.Dir, name)
	if _, err := os.Stat(path); err == nil {
		return name, nil
	}

	tmp, err := os.CreateTemp(f.Dir, "tmp-*.png")
	if err != nil {
		return "", err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(buf.Bytes()); err != nil {
		tmp.Close()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return "", fmt.Errorf("store image: %w", err)
	}
	return name, nil
}

func (f Files) Remove(name string) error {
	if !ValidName(name) {
		return fmt.Errorf("invalid image name %q", name)
	}
	err := os.Remove(filepath.Join(f.Dir, name))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// Placeholder renders the "nothing assigned yet" image for a panel: white with a black frame.
func Placeholder(width, height, bitDepth int) *image.Gray {
	levels := image.NewGray(image.Rect(0, 0, width, height))
	white := uint8(1<<bitDepth - 1)
	const frame = 12
	for y := range height {
		for x := range width {
			v := white
			if x < frame || y < frame || x >= width-frame || y >= height-frame {
				v = 0
			}
			levels.Pix[y*levels.Stride+x] = v
		}
	}
	return levels
}
