// Package ota handles firmware images for over-the-air updates: it reads and checks ESP-IDF app
// images before they are offered to devices, and stores them by content hash.
//
// TRMNL firmware installs whatever app image it is pointed at (no signature check, no rollback
// on the TRMNL X), so cnsl refuses anything that is not a complete app image for the right chip.
package ota

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// MaxSize bounds app images; app partitions of the supported boards are 3 MiB.
const MaxSize = 8 << 20

// MaxUpload bounds uploaded files, which may be full-flash images (16 MiB on the TRMNL X).
const MaxUpload = 32 << 20

// Info is what an app image says about itself.
type Info struct {
	Chip    string // esp32, esp32s3, …
	Project string // project name from the app descriptor
	Version string // app descriptor version (often not the version the firmware reports)
	Built   string // "Oct 30 2026 12:00:00"
	IDF     string
	Size    int
	SHA256  string // hex of the whole file
}

// Chip ids from the image header (esp_chip_id_t).
var chips = map[uint16]string{
	0: "esp32", 2: "esp32s2", 5: "esp32c3", 9: "esp32s3", 12: "esp32c2", 13: "esp32c6",
	16: "esp32h2", 18: "esp32p4", 20: "esp32c61", 23: "esp32c5",
}

// ChipName is a display name: esp32s3 → ESP32-S3.
func ChipName(chip string) string {
	if c, ok := strings.CutPrefix(chip, "esp32"); ok && c != "" {
		return "ESP32-" + strings.ToUpper(c)
	}
	return strings.ToUpper(chip)
}

const (
	imageMagic   = 0xE9
	appDescMagic = 0xABCD5432
	headerLen    = 24 // esp_image_header_t
	segHeaderLen = 8
)

// Parse checks an app image and reads its header and app descriptor. Merged/factory binaries
// (bootloader + partition table + app) are refused: OTA writes only the app partition.
func Parse(data []byte) (Info, error) {
	if len(data) < headerLen+segHeaderLen+256 {
		return Info{}, errors.New("too small to be a firmware image")
	}
	if len(data) > MaxSize {
		return Info{}, fmt.Errorf("larger than %d MiB", MaxSize>>20)
	}
	if data[0] != imageMagic {
		return Info{}, errors.New("not an ESP firmware image (use the app .bin, e.g. firmware.bin)")
	}
	chipID := binary.LittleEndian.Uint16(data[12:14])
	chip, ok := chips[chipID]
	if !ok {
		return Info{}, fmt.Errorf("unknown chip id %d", chipID)
	}
	hashAppended := data[23] == 1

	desc := data[headerLen+segHeaderLen:]
	if binary.LittleEndian.Uint32(desc[0:4]) != appDescMagic {
		return Info{}, errors.New("no app descriptor: neither an app image nor a full-flash image with an app partition")
	}

	info := Info{
		Chip:    chip,
		Version: cstr(desc[16:48]),
		Project: cstr(desc[48:80]),
		Built:   strings.TrimSpace(cstr(desc[96:112]) + " " + cstr(desc[80:96])),
		IDF:     cstr(desc[112:144]),
		Size:    len(data),
	}
	sum := sha256.Sum256(data)
	info.SHA256 = hex.EncodeToString(sum[:])

	if hashAppended {
		end, err := imageEnd(data)
		if err != nil {
			return Info{}, err
		}
		if end+32 > len(data) {
			return Info{}, errors.New("image is truncated")
		}
		got := sha256.Sum256(data[:end])
		if !bytes.Equal(got[:], data[end:end+32]) {
			return Info{}, errors.New("image checksum does not match (corrupt or incomplete download?)")
		}
	}
	return info, nil
}

// imageEnd walks the segments to find where the image (including its checksum byte and
// padding) ends, i.e. where the appended SHA-256 starts.
func imageEnd(data []byte) (int, error) {
	segments := int(data[1])
	off := headerLen
	for range segments {
		if off+segHeaderLen > len(data) {
			return 0, errors.New("image is truncated")
		}
		size := int(binary.LittleEndian.Uint32(data[off+4 : off+8]))
		off += segHeaderLen + size
		if off > len(data) {
			return 0, errors.New("image is truncated")
		}
	}
	// One checksum byte, padded so the image length is a multiple of 16.
	off++
	if rem := off % 16; rem != 0 {
		off += 16 - rem
	}
	return off, nil
}

// Partition table location and entry layout (esp_partition_info_t).
const (
	partitionTableOffset = 0x8000
	partitionMagic       = 0x50AA // bytes AA 50
	partitionEntryLen    = 32
	partitionTypeApp     = 0
)

// App returns the app image in data: data itself when it is an app image, or the image in the
// first app partition of a full-flash image (bootloader, partition table, apps, filesystem — what
// web flashers write). full reports the second case.
func App(data []byte) (app []byte, full bool, err error) {
	if len(data) < headerLen+segHeaderLen+256 || data[0] != imageMagic {
		return data, false, nil // Parse explains what is wrong
	}
	if binary.LittleEndian.Uint32(data[headerLen+segHeaderLen:]) == appDescMagic {
		return data, false, nil
	}
	// Starts with an image but not an app: a bootloader, so look for the partition table.
	if len(data) < partitionTableOffset+partitionEntryLen {
		return data, false, nil
	}
	for off := partitionTableOffset; off+partitionEntryLen <= len(data); off += partitionEntryLen {
		e := data[off : off+partitionEntryLen]
		if binary.LittleEndian.Uint16(e[0:2]) != partitionMagic {
			break // end of the table (FF FF) or its MD5 entry (EB EB)
		}
		if e[2] != partitionTypeApp {
			continue
		}
		start := int(binary.LittleEndian.Uint32(e[4:8]))
		size := int(binary.LittleEndian.Uint32(e[8:12]))
		if start <= 0 || start >= len(data) {
			continue
		}
		part := data[start:min(start+size, len(data))]
		if len(part) < headerLen+segHeaderLen+256 || part[0] != imageMagic ||
			binary.LittleEndian.Uint32(part[headerLen+segHeaderLen:]) != appDescMagic {
			continue // empty slot (e.g. ota_1 in a fresh image)
		}
		end, err := imageEnd(part)
		if err != nil {
			return nil, true, fmt.Errorf("app partition at %#x: %w", start, err)
		}
		if part[23] == 1 { // appended SHA-256
			end += 32
		}
		if end > len(part) {
			return nil, true, fmt.Errorf("app partition at %#x is truncated", start)
		}
		return part[:end], true, nil
	}
	return nil, true, errors.New("a full-flash image without an app partition with firmware in it")
}

func cstr(b []byte) string {
	if i := bytes.IndexByte(b, 0); i >= 0 {
		b = b[:i]
	}
	return strings.TrimSpace(string(b))
}

var semver = regexp.MustCompile(`\d+\.\d+\.\d+`)

// GuessVersion suggests the version a device will report after installing the image: TRMNL
// firmware reports its own FW_VERSION_STRING (e.g. 1.8.19), which usually appears in the file
// name of release binaries rather than in the app descriptor.
func GuessVersion(info Info, filename string) string {
	if v := semver.FindString(filename); v != "" {
		return v
	}
	if v := semver.FindString(info.Version); v != "" {
		return v
	}
	return ""
}

// SameVersion compares firmware version strings, ignoring a leading "v" and spaces.
func SameVersion(a, b string) bool {
	norm := func(s string) string { return strings.TrimPrefix(strings.ToLower(strings.TrimSpace(s)), "v") }
	return norm(a) != "" && norm(a) == norm(b)
}

// Files stores images under their SHA-256 (<dir>/<sha>.bin).
type Files struct{ Dir string }

func (f Files) Name(sha string) string { return sha + ".bin" }

func (f Files) Save(info Info, data []byte) error {
	if err := os.MkdirAll(f.Dir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(f.Dir, f.Name(info.SHA256))
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	tmp, err := os.CreateTemp(f.Dir, "tmp-*")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), path)
}

func (f Files) Remove(sha string) error {
	err := os.Remove(filepath.Join(f.Dir, f.Name(sha)))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
