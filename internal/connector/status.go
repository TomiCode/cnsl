package connector

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Status: is a server up? HTTP(S) requests or TCP connects, checked in parallel.
type statusChecks struct{}

func init() { register(statusChecks{}) }

func (statusChecks) Key() string  { return "status" }
func (statusChecks) Name() string { return "Server status" }
func (statusChecks) Description() string {
	return "Up/down and latency of HTTP(S) endpoints and TCP ports. In templates: range .Data.<alias>.checks (name, up, latency_ms), .Data.<alias>.up / .total"
}

func (statusChecks) ConfigSchema() Schema {
	return Schema{
		{Key: "timeout", Label: "Timeout (seconds)", Kind: "int", Default: float64(5)},
	}
}

func (statusChecks) ParamsSchema() Schema {
	return Schema{
		{Key: "checks", Label: "Checks", Kind: "list", Required: true,
			Help: "One per line: Name = https://host/health or Name = tcp://host:22"},
	}
}

type check struct {
	name, target string
}

func parseChecks(lines []string) ([]check, error) {
	out := make([]check, 0, len(lines))
	for _, l := range lines {
		name, target, ok := strings.Cut(l, "=")
		if !ok {
			name, target = l, l
		}
		name, target = strings.TrimSpace(name), strings.TrimSpace(target)
		u, err := url.Parse(target)
		if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https" && u.Scheme != "tcp") {
			return nil, fmt.Errorf("check %q: target must be http(s)://… or tcp://host:port", name)
		}
		out = append(out, check{name: name, target: target})
	}
	return out, nil
}

func (statusChecks) Fetch(ctx context.Context, env Env, config, params Values) (any, error) {
	checks, err := parseChecks(params.List("checks"))
	if err != nil {
		return nil, err
	}
	timeout := time.Duration(min(max(config.Int("timeout", 5), 1), 30)) * time.Second
	client := &http.Client{Timeout: timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

	results := make([]any, len(checks))
	var wg sync.WaitGroup
	for i, c := range checks {
		wg.Go(func() { results[i] = runCheck(ctx, client, timeout, c) })
	}
	wg.Wait()

	up := 0
	for _, r := range results {
		if r.(map[string]any)["up"] == true {
			up++
		}
	}
	return map[string]any{"checks": results, "up": up, "total": len(results), "all_up": up == len(results)}, nil
}

func runCheck(ctx context.Context, client *http.Client, timeout time.Duration, c check) map[string]any {
	r := map[string]any{"name": c.name, "target": c.target, "up": false}
	start := time.Now()
	u, _ := url.Parse(c.target)
	if u.Scheme == "tcp" {
		conn, err := (&net.Dialer{Timeout: timeout}).DialContext(ctx, "tcp", u.Host)
		if err != nil {
			r["error"] = err.Error()
			return r
		}
		conn.Close()
	} else {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, c.target, nil)
		req.Header.Set("User-Agent", "cnsl status check")
		resp, err := client.Do(req)
		if err != nil {
			r["error"] = err.Error()
			return r
		}
		resp.Body.Close()
		r["status"] = resp.StatusCode
		if resp.StatusCode >= 400 {
			r["error"] = resp.Status
			r["latency_ms"] = time.Since(start).Milliseconds()
			return r
		}
	}
	r["up"] = true
	r["latency_ms"] = time.Since(start).Milliseconds()
	return r
}

func (statusChecks) Sample(params Values) any {
	checks, _ := parseChecks(params.List("checks"))
	if len(checks) == 0 {
		checks = []check{{"nas", "tcp://nas.local:22"}, {"web", "https://example.com"}, {"backup", "tcp://backup.local:22"}}
	}
	results := make([]any, len(checks))
	for i, c := range checks {
		up := i%3 != 2
		r := map[string]any{"name": c.name, "target": c.target, "up": up, "latency_ms": 12 + 7*i}
		if !up {
			r["error"] = "connection refused"
			delete(r, "latency_ms")
		}
		results[i] = r
	}
	upCount := len(results) - len(results)/3
	return map[string]any{"checks": results, "up": upCount, "total": len(results), "all_up": upCount == len(results)}
}
