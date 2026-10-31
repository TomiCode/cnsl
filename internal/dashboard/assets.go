package dashboard

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	_ "image/gif" // decoders for uploads
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/TomiCode/cnsl/internal/dashboard/gen"
	"github.com/TomiCode/cnsl/internal/imaging"
	"github.com/TomiCode/cnsl/internal/store"
)

const (
	maxUploadBytes = 25 << 20
	maxImagePixels = 40_000_000 // refuse decompression bombs
)

func (s *Server) ListAssets(ctx context.Context, _ gen.ListAssetsRequestObject) (gen.ListAssetsResponseObject, error) {
	assets, err := s.store.Assets(ctx)
	if err != nil {
		return nil, err
	}
	out := make(gen.ListAssets200JSONResponse, 0, len(assets))
	for _, a := range assets {
		out = append(out, toAsset(a))
	}
	return out, nil
}

func (s *Server) UploadAsset(ctx context.Context, req gen.UploadAssetRequestObject) (gen.UploadAssetResponseObject, error) {
	invalid := func(format string, args ...any) gen.UploadAssetResponseObject {
		return gen.UploadAsset422ApplicationProblemPlusJSONResponse{ProblemApplicationProblemPlusJSONResponse: invalidProblem(fmt.Errorf(format, args...))}
	}
	var (
		data     []byte
		name     string
		filename string
	)
	for {
		part, err := req.Body.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return invalid("read upload: %v", err), nil
		}
		switch part.FormName() {
		case "file":
			filename = part.FileName()
			data, err = io.ReadAll(io.LimitReader(part, maxUploadBytes+1))
			if err != nil {
				return invalid("read upload: %v", err), nil
			}
			if len(data) > maxUploadBytes {
				return invalid("the image is larger than %d MB", maxUploadBytes>>20), nil
			}
		case "name":
			v, _ := io.ReadAll(io.LimitReader(part, 200))
			name = strings.TrimSpace(string(v))
		}
		part.Close()
	}
	if data == nil {
		return invalid("missing file"), nil
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return invalid("unsupported or broken image (PNG, JPEG or GIF): %v", err), nil
	}
	if cfg.Width*cfg.Height > maxImagePixels {
		return invalid("the image is too large (%d×%d)", cfg.Width, cfg.Height), nil
	}
	if name == "" {
		name = strings.TrimSuffix(path.Base(filepath.ToSlash(filename)), path.Ext(filename))
	}
	if name == "" {
		name = "image"
	}

	sum := sha256.Sum256(data)
	stored := hex.EncodeToString(sum[:16]) + "." + format
	if existing, err := s.store.AssetByFilename(ctx, stored); err == nil {
		return gen.UploadAsset201JSONResponse(toAsset(existing)), nil // same file uploaded before
	} else if !errors.Is(err, store.ErrNotFound) {
		return nil, err
	}
	if err := os.MkdirAll(s.uploadsDir, 0o755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(s.uploadsDir, stored), data, 0o644); err != nil {
		return nil, err
	}
	a := store.Asset{Name: name, Filename: stored, Mime: "image/" + format, Width: cfg.Width, Height: cfg.Height, Size: int64(len(data))}
	if err := s.store.CreateAsset(ctx, &a); err != nil {
		return nil, err
	}
	return gen.UploadAsset201JSONResponse(toAsset(a)), nil
}

func (s *Server) DeleteAsset(ctx context.Context, req gen.DeleteAssetRequestObject) (gen.DeleteAssetResponseObject, error) {
	a, err := s.store.Asset(ctx, req.Id)
	if errors.Is(err, store.ErrNotFound) {
		return gen.DeleteAsset404ApplicationProblemPlusJSONResponse{ProblemApplicationProblemPlusJSONResponse: notFoundProblem("image")}, nil
	}
	if err != nil {
		return nil, err
	}
	users, err := s.store.ScreensUsingAsset(ctx, a.ID)
	if err != nil {
		return nil, err
	}
	if len(users) > 0 {
		detail := "still used by: " + strings.Join(users, ", ")
		return gen.DeleteAsset409ApplicationProblemPlusJSONResponse{Title: http.StatusText(http.StatusConflict), Status: http.StatusConflict, Detail: &detail}, nil
	}
	if err := s.store.DeleteAsset(ctx, a.ID); err != nil {
		return nil, err
	}
	if err := os.Remove(filepath.Join(s.uploadsDir, a.Filename)); err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	return gen.DeleteAsset204Response{}, nil
}

func (s *Server) PreviewAsset(ctx context.Context, req gen.PreviewAssetRequestObject) (gen.PreviewAssetResponseObject, error) {
	fit, dither := imaging.FitCover, imaging.DitherFloydSteinberg
	if req.Body.Fit != nil {
		fit = imaging.Fit(*req.Body.Fit)
	}
	if req.Body.Dither != nil {
		dither = imaging.Dither(*req.Body.Dither)
	}
	png, err := s.renderer.AssetPreview(ctx, req.Id, req.Body.ModelId, fit, dither)
	if errors.Is(err, store.ErrNotFound) {
		return gen.PreviewAsset404ApplicationProblemPlusJSONResponse{ProblemApplicationProblemPlusJSONResponse: notFoundProblem("image or model")}, nil
	}
	if err != nil {
		return nil, err
	}
	return gen.PreviewAsset200JSONResponse{Device: "data:image/png;base64," + base64.StdEncoding.EncodeToString(png)}, nil
}

func toAsset(a store.Asset) gen.Asset {
	return gen.Asset{Id: a.ID, Name: a.Name, Url: "/uploads/" + a.Filename, Width: a.Width, Height: a.Height, Size: a.Size, CreatedAt: a.CreatedAt.Time}
}
