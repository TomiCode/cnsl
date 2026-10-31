// Package tmpl executes screen region templates (Go html/template) with cnsl's data context
// and helper functions.
package tmpl

import (
	"bytes"
	"errors"
	"fmt"
	"html/template"
	"regexp"
	"strconv"
	"time"
)

// Context is what a region template sees as ".".
type Context struct {
	Now    time.Time      // in the household timezone
	Device DeviceInfo     // filled only when the screen is rendered for exactly one device
	Screen ScreenInfo     //
	Region RegionInfo     //
	Data   map[string]any // binding alias → connector data (phase 4)
	Stale  map[string]bool
}

type DeviceInfo struct {
	Name           string
	BatteryPercent float64
	RSSI           int
}

type ScreenInfo struct {
	Name          string
	Width, Height int // device pixels
	BitDepth      int
}

type RegionInfo struct {
	Key           string
	Width, Height int
}

// Error is a template problem with a position, for editor markers.
type Error struct {
	Region  string `json:"region"`
	Line    int    `json:"line"`
	Column  int    `json:"column"`
	Message string `json:"message"`
}

func (e *Error) Error() string {
	if e.Line > 0 {
		return fmt.Sprintf("%s:%d:%d: %s", e.Region, e.Line, e.Column, e.Message)
	}
	return fmt.Sprintf("%s: %s", e.Region, e.Message)
}

const maxOutput = 2 << 20 // a region's HTML may not exceed 2 MiB

// Render parses and executes a region template. Errors are returned as *Error.
func Render(region, src string, ctx Context) (template.HTML, error) {
	t, err := template.New(region).Option("missingkey=zero").Funcs(funcs).Parse(src)
	if err != nil {
		return "", toError(region, err)
	}
	var buf bytes.Buffer
	if err := t.Execute(&limitWriter{w: &buf, n: maxOutput}, ctx); err != nil {
		return "", toError(region, err)
	}
	return template.HTML(buf.String()), nil
}

// Check parses a template without executing it.
func Check(region, src string) error {
	if _, err := template.New(region).Funcs(funcs).Parse(src); err != nil {
		return toError(region, err)
	}
	return nil
}

// Go reports positions as "template: name:LINE:COL: msg" (parse) or
// "template: name:LINE:COL: executing "name" at <...>: msg" (exec).
var positionPattern = regexp.MustCompile(`^template: [^:]+:(\d+)(?::(\d+))?: (.*)$`)

func toError(region string, err error) error {
	var lim errLimit
	if errors.As(err, &lim) {
		return &Error{Region: region, Message: "output too large (over 2 MiB)"}
	}
	e := &Error{Region: region, Message: err.Error()}
	if m := positionPattern.FindStringSubmatch(err.Error()); m != nil {
		e.Line, _ = strconv.Atoi(m[1])
		e.Column, _ = strconv.Atoi(m[2])
		e.Message = m[3]
	}
	return e
}

type errLimit struct{}

func (errLimit) Error() string { return "output limit exceeded" }

type limitWriter struct {
	w *bytes.Buffer
	n int
}

func (l *limitWriter) Write(p []byte) (int, error) {
	if l.w.Len()+len(p) > l.n {
		return 0, errLimit{}
	}
	return l.w.Write(p)
}
