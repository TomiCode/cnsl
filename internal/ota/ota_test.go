package ota

import (
	"crypto/sha256"
	"encoding/binary"
	"strings"
	"testing"
)

// fakeImage builds a minimal app image: header, one segment starting with an app descriptor,
// checksum byte + padding, and (optionally) the appended SHA-256.
func fakeImage(chipID uint16, hash bool) []byte {
	desc := make([]byte, 256)
	binary.LittleEndian.PutUint32(desc[0:], appDescMagic)
	copy(desc[16:], "1.8.19-dirty")
	copy(desc[48:], "trmnl")
	copy(desc[80:], "12:00:00")
	copy(desc[96:], "Oct 30 2026")
	copy(desc[112:], "v5.5")

	img := make([]byte, headerLen)
	img[0] = imageMagic
	img[1] = 1 // segments
	binary.LittleEndian.PutUint16(img[12:], chipID)
	if hash {
		img[23] = 1
	}
	seg := make([]byte, segHeaderLen)
	binary.LittleEndian.PutUint32(seg[4:], uint32(len(desc)))
	img = append(img, seg...)
	img = append(img, desc...)
	img = append(img, 0xEF) // checksum byte
	for len(img)%16 != 0 {
		img = append(img, 0)
	}
	if hash {
		sum := sha256.Sum256(img)
		img = append(img, sum[:]...)
	}
	return img
}

func TestParse(t *testing.T) {
	info, err := Parse(fakeImage(9, true))
	if err != nil {
		t.Fatal(err)
	}
	if info.Chip != "esp32s3" || info.Project != "trmnl" || info.Version != "1.8.19-dirty" || info.Built != "Oct 30 2026 12:00:00" || info.IDF != "v5.5" {
		t.Fatalf("info = %+v", info)
	}
	if ChipName(info.Chip) != "ESP32-S3" {
		t.Errorf("ChipName = %q", ChipName(info.Chip))
	}
	if _, err := Parse(fakeImage(0, false)); err != nil {
		t.Errorf("image without appended hash: %v", err)
	}

	corrupt := fakeImage(9, true)
	corrupt[100] ^= 0xFF
	if _, err := Parse(corrupt); err == nil || !strings.Contains(err.Error(), "checksum") {
		t.Errorf("corrupt image: %v", err)
	}
	truncated := fakeImage(9, true)
	if _, err := Parse(truncated[:len(truncated)-20]); err == nil {
		t.Error("truncated image accepted")
	}
	merged := append(make([]byte, 0x1000), fakeImage(9, true)...)
	merged[0] = imageMagic // a bootloader image has no app descriptor where the app has one
	if _, err := Parse(merged); err == nil || !strings.Contains(err.Error(), "app descriptor") {
		t.Errorf("merged image: %v", err)
	}
	if _, err := Parse([]byte(strings.Repeat("x", 1000))); err == nil {
		t.Error("random data accepted")
	}
}

func TestVersions(t *testing.T) {
	info := Info{Version: "abc123"}
	if v := GuessVersion(info, "trmnl-x-FW1.8.19.bin"); v != "1.8.19" {
		t.Errorf("from filename: %q", v)
	}
	if v := GuessVersion(Info{Version: "v1.9.0"}, "firmware.bin"); v != "1.9.0" {
		t.Errorf("from descriptor: %q", v)
	}
	if v := GuessVersion(info, "firmware.bin"); v != "" {
		t.Errorf("no guess expected, got %q", v)
	}
	if !SameVersion("v1.8.19", "1.8.19 ") || SameVersion("", "") || SameVersion("1.8.19", "1.8.2") {
		t.Error("SameVersion")
	}
}
