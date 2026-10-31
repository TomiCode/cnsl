// Package config loads cnsl settings from CNSL_* environment variables.
package config

import (
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Config struct {
	Addr            string // dashboard listener
	DeviceAddr      string // device (firmware) listener
	DevicePublicURL string // base URL of the device listener as devices see it
	DataDir         string
	ChromeURL       string // empty = launch a local Chrome
	RenderBaseURL   string // URL Chrome uses to reach the dashboard listener
	Timezone        *time.Location
	LogLevel        slog.Level
	LogFormat       string // "text" or "json"
}

// DatabasePath is the SQLite file inside the data directory.
func (c Config) DatabasePath() string {
	return filepath.Join(c.DataDir, "cnsl.db")
}

// ImagesDir holds rendered device images.
func (c Config) ImagesDir() string {
	return filepath.Join(c.DataDir, "images")
}

func Load() (Config, error) {
	c := Config{
		Addr:            env("CNSL_ADDR", ":8080"),
		DeviceAddr:      env("CNSL_DEVICE_ADDR", ":8081"),
		DevicePublicURL: strings.TrimRight(env("CNSL_DEVICE_PUBLIC_URL", ""), "/"),
		DataDir:         env("CNSL_DATA_DIR", "./data"),
		ChromeURL:       env("CNSL_CHROME_URL", ""),
		RenderBaseURL:   strings.TrimRight(env("CNSL_RENDER_BASE_URL", ""), "/"),
		LogFormat:       env("CNSL_LOG_FORMAT", "text"),
	}

	tzName := env("CNSL_TIMEZONE", env("TZ", "UTC"))
	tz, err := time.LoadLocation(tzName)
	if err != nil {
		return c, fmt.Errorf("CNSL_TIMEZONE %q: %w", tzName, err)
	}
	c.Timezone = tz

	if err := c.LogLevel.UnmarshalText([]byte(env("CNSL_LOG_LEVEL", "info"))); err != nil {
		return c, fmt.Errorf("CNSL_LOG_LEVEL: %w", err)
	}
	if c.LogFormat != "text" && c.LogFormat != "json" {
		return c, fmt.Errorf("CNSL_LOG_FORMAT must be text or json, got %q", c.LogFormat)
	}

	if c.DevicePublicURL != "" {
		if _, err := url.ParseRequestURI(c.DevicePublicURL); err != nil {
			return c, fmt.Errorf("CNSL_DEVICE_PUBLIC_URL: %w", err)
		}
	}
	if c.RenderBaseURL == "" {
		c.RenderBaseURL = "http://" + localHost(c.Addr)
	}
	return c, nil
}

func env(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

// localHost turns a listen address like ":8080" into "localhost:8080".
func localHost(addr string) string {
	if strings.HasPrefix(addr, ":") {
		return "localhost" + addr
	}
	return addr
}
