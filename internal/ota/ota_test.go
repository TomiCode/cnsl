package ota

import (
	"crypto/sha256"
	"encoding/binary"
	"os"
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
	bootloader := append(make([]byte, 0x1000), fakeImage(9, true)...)
	bootloader[0] = imageMagic // a bootloader image has no app descriptor where the app has one
	if _, err := Parse(bootloader); err == nil || !strings.Contains(err.Error(), "app descriptor") {
		t.Errorf("bootloader image: %v", err)
	}
	if _, err := Parse([]byte(strings.Repeat("x", 1000))); err == nil {
		t.Error("random data accepted")
	}
}

// fakeFlash lays out a full-flash image: bootloader at 0, partition table at 0x8000 with an
// empty ota_0 slot first and the app in ota_1, filesystem padding after it.
func fakeFlash(app []byte) []byte {
	flash := make([]byte, 0x40000)
	for i := range flash {
		flash[i] = 0xFF
	}
	boot := fakeImage(9, false)
	binary.LittleEndian.PutUint32(boot[headerLen+segHeaderLen:], 0) // bootloaders carry no app descriptor
	copy(flash, boot)
	entry := func(i int, typ, sub byte, off, size uint32) {
		e := flash[partitionTableOffset+i*partitionEntryLen:]
		binary.LittleEndian.PutUint16(e, partitionMagic)
		e[2], e[3] = typ, sub
		binary.LittleEndian.PutUint32(e[4:], off)
		binary.LittleEndian.PutUint32(e[8:], size)
	}
	entry(0, 1, 0x02, 0x9000, 0x5000)   // nvs
	entry(1, 0, 0x10, 0x10000, 0x10000) // ota_0, empty
	entry(2, 0, 0x11, 0x20000, 0x10000) // ota_1, the app
	entry(3, 1, 0x82, 0x30000, 0x10000) // spiffs
	copy(flash[0x20000:], app)
	return flash
}

func TestApp(t *testing.T) {
	app := fakeImage(9, true)
	got, full, err := App(fakeFlash(app))
	if err != nil || !full {
		t.Fatalf("full=%v err=%v", full, err)
	}
	if string(got) != string(app) {
		t.Fatalf("extracted %d bytes, want the %d-byte app", len(got), len(app))
	}
	if _, err := Parse(got); err != nil {
		t.Fatal(err)
	}
	if got, full, _ := App(app); full || len(got) != len(app) {
		t.Error("an app image should pass through")
	}
	empty := fakeFlash(nil)
	if _, _, err := App(empty); err == nil {
		t.Error("flash image without an app accepted")
	}
}

// TestRealFlashImage checks a downloaded full-flash image: CNSL_FLASH_IMAGE=path go test ./internal/ota
func TestRealFlashImage(t *testing.T) {
	path := os.Getenv("CNSL_FLASH_IMAGE")
	if path == "" {
		t.Skip("set CNSL_FLASH_IMAGE to a full-flash .bin")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	app, full, err := App(data)
	if err != nil {
		t.Fatal(err)
	}
	info, err := Parse(app)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("full=%v app=%d bytes chip=%s project=%s version=%s built=%s idf=%s guess=%s",
		full, len(app), info.Chip, info.Project, info.Version, info.Built, info.IDF, GuessVersion(info, path))
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
