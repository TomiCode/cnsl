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

	"github.com/TomiCode/cnsl/internal/config"
	"github.com/TomiCode/cnsl/internal/dashboard"
	"github.com/TomiCode/cnsl/internal/firmware"
	"github.com/TomiCode/cnsl/internal/httpx"
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

	if cfg.DevicePublicURL == "" {
		logger.Warn("CNSL_DEVICE_PUBLIC_URL is not set; devices will not be able to download images")
	}

	api, err := dashboard.NewServer(version, st).Handler()
	if err != nil {
		return err
	}
	dashMux := http.NewServeMux()
	dashMux.Handle(dashboard.BasePath+"/", api)
	dashMux.Handle("/", web.Handler())

	deviceHandler := firmware.Handler(firmware.Options{
		ImagesDir:   cfg.ImagesDir(),
		AllowOrigin: cfg.DashboardOrigin,
	})

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
