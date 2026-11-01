package render

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/gif" // decoders for uploaded assets
	_ "image/jpeg"
	"image/png"
	"math/rand/v2"
	"os"
	"path/filepath"

	"github.com/TomiCode/cnsl/internal/imaging"
	"github.com/TomiCode/cnsl/internal/store"
)

// Background is a screen's image layer (screens.background JSON). Template screens draw their
// regions on top of it; image screens consist of nothing else.
type Background struct {
	Kind     string  `json:"kind"` // "none" | "image" | "slideshow"
	AssetIDs []int64 `json:"asset_ids,omitempty"`
	Fit      string  `json:"fit,omitempty"`    // imaging.Fit, default cover
	Dither   string  `json:"dither,omitempty"` // imaging.Dither, default floyd-steinberg
	Order    string  `json:"order,omitempty"`  // slideshow: "sequential" (default) | "shuffle"
}

func ParseBackground(s string) (Background, error) {
	var b Background
	if s == "" {
		return Background{Kind: "none"}, nil
	}
	if err := json.Unmarshal([]byte(s), &b); err != nil {
		return Background{Kind: "none"}, err
	}
	return b.normalized(), nil
}

func (b Background) normalized() Background {
	if b.Kind == "" {
		b.Kind = "none"
	}
	if b.Fit == "" {
		b.Fit = string(imaging.FitCover)
	}
	if b.Dither == "" {
		b.Dither = string(imaging.DitherFloydSteinberg)
	}
	if b.Order == "" {
		b.Order = "sequential"
	}
	return b
}

func (b Background) Validate() error {
	switch b.Kind {
	case "none":
		return nil
	case "image":
		if len(b.AssetIDs) != 1 {
			return errors.New("an image background needs exactly one image")
		}
	case "slideshow":
		if len(b.AssetIDs) < 1 {
			return errors.New("a slideshow needs at least one image")
		}
	default:
		return fmt.Errorf("unknown background kind %q", b.Kind)
	}
	if b.Fit != string(imaging.FitCover) && b.Fit != string(imaging.FitContain) {
		return fmt.Errorf("unknown fit %q", b.Fit)
	}
	if !imaging.ValidDither(imaging.Dither(b.Dither)) {
		return fmt.Errorf("unknown dither %q", b.Dither)
	}
	if b.Order != "sequential" && b.Order != "shuffle" {
		return fmt.Errorf("unknown slideshow order %q", b.Order)
	}
	return nil
}

func (b Background) JSON() string {
	data, _ := json.Marshal(b)
	return string(data)
}

// Pick returns the asset to show for a slideshow position.
func (b Background) Pick(index int) (int64, bool) {
	n := len(b.AssetIDs)
	if b.Kind == "none" || n == 0 {
		return 0, false
	}
	if b.Kind == "image" || n == 1 {
		return b.AssetIDs[0], true
	}
	i := index % n
	if b.Order == "shuffle" {
		// A fresh permutation per round, so every image appears once before any repeats.
		round := uint64(index / n)
		perm := rand.New(rand.NewPCG(round, 0x636e736c)).Perm(n)
		i = perm[i]
	}
	return b.AssetIDs[i], true
}

// assetLevels loads an asset and prepares it for the model (fit, tone curve, dither) as level
// indexes. Results are cached on disk, keyed by the inputs.
func (r *Renderer) assetLevels(ctx context.Context, assetID int64, model store.DeviceModel, fit imaging.Fit, dither imaging.Dither) (*image.Gray, error) {
	a, err := r.store.Asset(ctx, assetID)
	if err != nil {
		return nil, fmt.Errorf("background image %d: %w", assetID, err)
	}
	curve, _ := imaging.ParseToneCurve(model.ToneCurve.String)
	sum := sha256.Sum256(fmt.Appendf(nil, "%s|%dx%d|%s|%s|%v|%d", a.Filename, model.Width, model.Height, fit, dither, curve, model.BitDepth))
	cached := filepath.Join(r.cacheDir, hex.EncodeToString(sum[:12])+".png")

	if f, err := os.Open(cached); err == nil {
		defer f.Close()
		if img, err := png.Decode(f); err == nil {
			if g, ok := img.(*image.Gray); ok {
				touch(cached) // keeps it from being cleaned up while in use
				return g, nil
			}
		}
	}

	src, err := r.openAsset(a)
	if err != nil {
		return nil, err
	}
	levels := imaging.Prepare(src, model.Width, model.Height, fit, dither, curve, model.BitDepth)

	// Cache the level indexes as an 8-bit gray PNG (pixel value = level).
	var buf bytes.Buffer
	if err := png.Encode(&buf, levels); err == nil {
		if err := os.MkdirAll(r.cacheDir, 0o755); err == nil {
			_ = os.WriteFile(cached, buf.Bytes(), 0o644)
		}
	}
	return levels, nil
}

func (r *Renderer) openAsset(a store.Asset) (image.Image, error) {
	f, err := os.Open(filepath.Join(r.uploadsDir, a.Filename))
	if err != nil {
		return nil, fmt.Errorf("open image %q: %w", a.Name, err)
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("decode image %q: %w", a.Name, err)
	}
	return img, nil
}

// AssetPreview shows how an asset looks on a model's panel with the given fit and dithering
// (emulated with the model's measured gray levels).
func (r *Renderer) AssetPreview(ctx context.Context, assetID, modelID int64, fit imaging.Fit, dither imaging.Dither) ([]byte, error) {
	model, err := r.store.DeviceModel(ctx, modelID)
	if err != nil {
		return nil, err
	}
	levels, err := r.assetLevels(ctx, assetID, model, fit, dither)
	if err != nil {
		return nil, err
	}
	curve, _ := imaging.ParseToneCurve(model.ToneCurve.String)
	var buf bytes.Buffer
	err = png.Encode(&buf, curve.Palette(model.BitDepth).Emulate(levels))
	return buf.Bytes(), err
}
