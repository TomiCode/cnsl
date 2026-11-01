// Package firmware serves the device listener: the endpoints TRMNL firmware calls and the
// rendered images it downloads. Paths and headers are dictated by the firmware, so these
// handlers are hand-written rather than generated from the OpenAPI spec.
package firmware

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/TomiCode/cnsl/internal/device"
	"github.com/TomiCode/cnsl/internal/store"
)

type Options struct {
	Devices     *device.Service
	ImagesDir   string
	FirmwareDir string // OTA images, served at /firmware/<sha256>.bin
	// PublicURL is the base URL devices use to reach this listener; image_url is built from it.
	PublicURL string
	Log       *slog.Logger
}

type handler struct {
	Options
}

func Handler(opts Options) http.Handler {
	h := &handler{opts}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/setup", h.setup)
	mux.HandleFunc("GET /api/display", h.display)
	mux.HandleFunc("POST /api/log", h.logs)

	images := http.StripPrefix("/images/", http.FileServer(http.Dir(opts.ImagesDir)))
	mux.Handle("GET /images/", immutable(images))
	if opts.FirmwareDir != "" {
		mux.Handle("GET /firmware/", immutable(http.StripPrefix("/firmware/", http.FileServer(http.Dir(opts.FirmwareDir)))))
	}
	return mux
}

func (h *handler) setup(w http.ResponseWriter, r *http.Request) {
	d, err := h.Devices.Setup(r.Context(), identity(r))
	if err != nil {
		if errors.Is(err, device.ErrInvalid) {
			writeJSON(w, map[string]any{"status": 404, "message": err.Error()})
			return
		}
		h.fail(w, r, err)
		return
	}
	if d.Status == store.DeviceDisabled {
		writeJSON(w, map[string]any{"status": 403, "message": "This device is disabled in cnsl"})
		return
	}
	// The firmware stores the key only when status is 200 and draws friendly_id + message itself.
	writeJSON(w, map[string]any{
		"status":      200,
		"api_key":     d.APIKey,
		"friendly_id": d.FriendlyID,
		"image_url":   "",
		"message":     "Approve this device in cnsl",
	})
}

func (h *handler) display(w http.ResponseWriter, r *http.Request) {
	res, err := h.Devices.Display(r.Context(), device.DisplayRequest{
		APIKey:    header(r, "Access-Token"),
		Identity:  identity(r),
		Telemetry: telemetry(r),
	})
	if err != nil && !errors.Is(err, device.ErrInvalid) {
		h.fail(w, r, err)
		return
	}
	if err != nil || res.Outcome == device.NotRegistered {
		// 202 = not registered: the firmware fast-polls until the device is approved.
		writeJSON(w, map[string]any{"status": 202})
		return
	}

	firmwareURL := ""
	if res.Firmware != "" {
		firmwareURL = h.PublicURL + "/firmware/" + res.Firmware
	}
	writeJSON(w, map[string]any{
		"status":                0,
		"filename":              res.Filename,
		"image_url":             h.PublicURL + "/images/" + res.Filename,
		"image_url_timeout":     0,
		"refresh_rate":          res.RefreshRate,
		"update_firmware":       firmwareURL != "",
		"firmware_url":          firmwareURL,
		"reset_firmware":        false,
		"special_function":      "none",
		"temperature_profile":   "default",
		"maximum_compatibility": false,
	})
}

func (h *handler) logs(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Logs []map[string]any `json:"logs"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&body); err != nil {
		http.Error(w, "invalid log payload", http.StatusBadRequest)
		return
	}
	if _, err := h.Devices.IngestLogs(r.Context(), header(r, "Access-Token"), body.Logs); err != nil {
		if errors.Is(err, device.ErrInvalid) {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	h.Log.ErrorContext(r.Context(), "device request failed", "path", r.URL.Path, "err", err)
	http.Error(w, "internal error", http.StatusInternalServerError)
}

// header reads a firmware header. Current firmware sends hyphenated names (Access-Token);
// older firmware and docs use underscores (ACCESS_TOKEN), so both are accepted.
func header(r *http.Request, name string) string {
	if v := r.Header.Get(name); v != "" {
		return strings.TrimSpace(v)
	}
	return strings.TrimSpace(r.Header.Get(strings.ReplaceAll(name, "-", "_")))
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// immutable marks responses as cacheable forever: image filenames are content hashes.
func immutable(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		next.ServeHTTP(w, r)
	})
}
