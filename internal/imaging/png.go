package imaging

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"image"
	"io"
)

var pngSignature = []byte("\x89PNG\r\n\x1a\n")

// EncodePNG writes level indexes (see Quantize) as a grayscale PNG with the given bit depth
// (1, 2 or 4). The standard library only writes 8-bit gray, but the TRMNL firmware decodes
// low-bit-depth gray PNGs directly, which keeps files small and avoids palette conversion.
func EncodePNG(w io.Writer, levels *image.Gray, bitDepth int) error {
	if bitDepth != 1 && bitDepth != 2 && bitDepth != 4 && bitDepth != 8 {
		return fmt.Errorf("unsupported bit depth %d", bitDepth)
	}
	width, height := levels.Bounds().Dx(), levels.Bounds().Dy()

	var ihdr [13]byte
	binary.BigEndian.PutUint32(ihdr[0:], uint32(width))
	binary.BigEndian.PutUint32(ihdr[4:], uint32(height))
	ihdr[8] = byte(bitDepth)
	ihdr[9] = 0 // color type: grayscale
	// compression, filter and interlace methods are all 0

	var idat bytes.Buffer
	zw, err := zlib.NewWriterLevel(&idat, zlib.BestCompression)
	if err != nil {
		return err
	}
	perByte := 8 / bitDepth
	row := make([]byte, 1+(width+perByte-1)/perByte) // leading filter byte 0 (None)
	for y := range height {
		clear(row)
		src := levels.Pix[y*levels.Stride : y*levels.Stride+width]
		for x, v := range src {
			shift := 8 - bitDepth*(x%perByte+1)
			row[1+x/perByte] |= v << shift
		}
		if _, err := zw.Write(row); err != nil {
			return err
		}
	}
	if err := zw.Close(); err != nil {
		return err
	}

	if _, err := w.Write(pngSignature); err != nil {
		return err
	}
	for _, c := range []struct {
		kind string
		data []byte
	}{{"IHDR", ihdr[:]}, {"IDAT", idat.Bytes()}, {"IEND", nil}} {
		if err := writeChunk(w, c.kind, c.data); err != nil {
			return err
		}
	}
	return nil
}

func writeChunk(w io.Writer, kind string, data []byte) error {
	var header [8]byte
	binary.BigEndian.PutUint32(header[:4], uint32(len(data)))
	copy(header[4:], kind)
	crc := crc32.NewIEEE()
	crc.Write(header[4:])
	crc.Write(data)

	var trailer [4]byte
	binary.BigEndian.PutUint32(trailer[:], crc.Sum32())
	for _, b := range [][]byte{header[:], data, trailer[:]} {
		if _, err := w.Write(b); err != nil {
			return err
		}
	}
	return nil
}
