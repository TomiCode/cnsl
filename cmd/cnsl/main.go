// Command cnsl runs the TRMNL server: the device listener, the dashboard and its API.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
	_ "time/tzdata" // timezones work in minimal container images

	"github.com/TomiCode/cnsl/internal/assets"
	"github.com/TomiCode/cnsl/internal/config"
	"github.com/TomiCode/cnsl/internal/connector"
	"github.com/TomiCode/cnsl/internal/dashboard"
	"github.com/TomiCode/cnsl/internal/device"
	"github.com/TomiCode/cnsl/internal/events"
	"github.com/TomiCode/cnsl/internal/firmware"
	"github.com/TomiCode/cnsl/internal/httpx"
	"github.com/TomiCode/cnsl/internal/icons"
	"github.com/TomiCode/cnsl/internal/imaging"
	"github.com/TomiCode/cnsl/internal/library"
	"github.com/TomiCode/cnsl/internal/render"
	"github.com/TomiCode/cnsl/internal/secret"
	"github.com/TomiCode/cnsl/internal/store"
	"github.com/TomiCode/cnsl/web"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	if err := run(); err != nil {
		slog.Error("cnsl stopped", "err", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	logger := newLogger(cfg)
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := os.MkdirAll(cfg.ImagesDir(), 0o755); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}
	st, err := store.Open(ctx, cfg.DatabasePath())
	if err != nil {
		return err
	}
	defer st.Close()
	if err := library.Seed(ctx, st); err != nil {
		return fmt.Errorf("seed component library: %w", err)
	}

	if cfg.DevicePublicURL == "" {
		logger.Warn("CNSL_DEVICE_PUBLIC_URL is not set; devices will not be able to download images")
	}

	broker := events.NewBroker()
	files := imaging.Files{Dir: cfg.ImagesDir()}
	devices := device.NewService(st, broker, files, cfg.Timezone, logger)

	box, keySource, err := secret.Load(cfg.SecretKey, cfg.DataDir)
	if err != nil {
		return err
	}
	logger.Info("secret key loaded", "source", keySource)
	connectors := connector.NewService(st, logger, box)
	if n, err := connectors.EncryptStored(ctx); err != nil {
		return fmt.Errorf("encrypt stored secrets: %w", err)
	} else if n > 0 {
		logger.Info("encrypted connector secrets stored in plaintext", "connectors", n)
	}

	browser := render.NewBrowser(cfg.ChromeURL, 2, logger)
	defer browser.Close()
	renderer := render.NewRenderer(render.Options{
		Store:    st,
		Files:    files,
		Browser:  browser,
		BaseURL:  cfg.RenderBaseURL,
		Timezone: cfg.Timezone,
		Log:      logger,

		UploadsDir: cfg.UploadsDir(),
		CacheDir:   cfg.CacheDir(),
		Data:       connectors,
	})
	scheduler := render.NewScheduler(renderer, st, broker, logger)
	devices.SetScreens(scheduler)
	go scheduler.Run(ctx)
	go renderer.RunCleanup(ctx, 6*time.Hour)
	connectors.OnChange = func(screenIDs []int64) {
		for _, id := range screenIDs {
			scheduler.Request(id, "data")
		}
	}
	go connectors.Run(ctx)
	go st.RunRetention(ctx, 30*24*time.Hour, 90*24*time.Hour, logger)

	api, err := dashboard.NewServer(dashboard.Options{
		Version:         version,
		Store:           st,
		Devices:         devices,
		Events:          broker,
		Timezone:        cfg.Timezone,
		DevicePublicURL: cfg.DevicePublicURL,
		Renderer:        renderer,
		Scheduler:       scheduler,
		UploadsDir:      cfg.UploadsDir(),
		FirmwareDir:     cfg.FirmwareDir(),
		Connectors:      connectors,
	}).Handler()
	if err != nil {
		return err
	}

	deviceHandler := firmware.Handler(firmware.Options{
		Devices:     devices,
		ImagesDir:   cfg.ImagesDir(),
		FirmwareDir: cfg.FirmwareDir(),
		PublicURL:   cfg.DevicePublicURL,
		Log:         logger,
	})

	dashMux := http.NewServeMux()
	dashMux.Handle(dashboard.BasePath+"/", api)
	// The device API is mirrored on the dashboard listener so the browser-based virtual device
	// (and image previews) work same-origin, without CORS.
	dashMux.Handle("/device/", http.StripPrefix("/device", deviceHandler))
	// What the headless browser loads while rendering. The framework CSS hard-codes /fonts/.
	dashMux.Handle("/internal/render/", renderer.Handler())
	dashMux.Handle("/fonts/", http.StripPrefix("/fonts/", assets.FontsHandler()))
	dashMux.Handle("GET /icons/", http.StripPrefix("/icons", icons.Handler()))
	// Original uploads (content-hash names) for thumbnails in the dashboard.
	dashMux.Handle("GET /uploads/", http.StripPrefix("/uploads/", httpx.Immutable(http.FileServer(http.Dir(cfg.UploadsDir())))))
	dashMux.Handle("/", web.Handler())

	servers := []*http.Server{
		newServer(ctx, cfg.Addr, httpx.LogRequests(logger.With("listener", "dashboard"), dashMux)),
		newServer(ctx, cfg.DeviceAddr, httpx.LogRequests(logger.With("listener", "device"), deviceHandler)),
	}

	errc := make(chan error, len(servers))
	for _, srv := range servers {
		go func() {
			if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				errc <- fmt.Errorf("listen %s: %w", srv.Addr, err)
			}
		}()
	}
	logger.Info("cnsl started",
		"version", version,
		"dashboard", cfg.Addr,
		"device", cfg.DeviceAddr,
		"data_dir", cfg.DataDir,
		"timezone", cfg.Timezone.String(),
	)

	var runErr error
	select {
	case <-ctx.Done():
		logger.Info("shutting down")
	case runErr = <-errc:
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for _, srv := range servers {
		if err := srv.Shutdown(shutdownCtx); err != nil {
			logger.Warn("shutdown", "addr", srv.Addr, "err", err)
		}
	}
	return runErr
}

func newServer(ctx context.Context, addr string, h http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           h,
		ReadHeaderTimeout: 10 * time.Second,
		BaseContext:       func(net.Listener) context.Context { return ctx },
	}
}

func newLogger(cfg config.Config) *slog.Logger {
	opts := &slog.HandlerOptions{Level: cfg.LogLevel}
	if cfg.LogFormat == "json" {
		return slog.New(slog.NewJSONHandler(os.Stderr, opts))
	}
	return slog.New(slog.NewTextHandler(os.Stderr, opts))
}
