// Command vendor-icons packs icon sets into internal/icons/data as gzipped JSON:
//
//	go run ./cmd/vendor-icons -lucide <lucide-static package dir> -wi <weather-icons checkout> -wi-source "<commit>"
//
// lucide-static comes from npm (`npm pack lucide-static`), Weather Icons from
// github.com/erikflowers/weather-icons (its svg/ directory). Only the shapes inside each <svg> are
// kept; the icons package adds the outer element when rendering.
package main

import (
	"compress/gzip"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type icon struct {
	Name    string   `json:"name"`
	ViewBox string   `json:"viewBox"`
	Body    string   `json:"body"`
	Tags    []string `json:"tags,omitempty"`
}

type set struct {
	Source string `json:"source"`
	Icons  []icon `json:"icons"`
}

var (
	viewBoxAttr = regexp.MustCompile(`viewBox="([^"]+)"`)
	svgOpen     = regexp.MustCompile(`(?s)<svg\b[^>]*>`)
	comment     = regexp.MustCompile(`(?s)<!--.*?-->`)
	space       = regexp.MustCompile(`\s+`)
	// Only plain shapes are accepted, so nothing active (scripts, links, styles) gets inlined.
	allowed = regexp.MustCompile(`^<(/?)(path|circle|ellipse|line|polygon|polyline|rect|g)\b`)
	tag     = regexp.MustCompile(`<[^>]+>`)
)

func main() {
	lucideDir := flag.String("lucide", "", "lucide-static package directory (contains icons/ and tags.json)")
	wiDir := flag.String("wi", "", "weather-icons checkout (contains svg/)")
	wiSource := flag.String("wi-source", "", "weather-icons version/commit for the SOURCE note")
	out := flag.String("out", "internal/icons/data", "output directory")
	flag.Parse()
	if *lucideDir == "" || *wiDir == "" {
		flag.Usage()
		os.Exit(2)
	}

	var version struct{ Version string }
	readJSON(filepath.Join(*lucideDir, "package.json"), &version)
	var tags map[string][]string
	readJSON(filepath.Join(*lucideDir, "tags.json"), &tags)

	lucide := set{Source: "lucide-static " + version.Version}
	for _, path := range glob(filepath.Join(*lucideDir, "icons", "*.svg")) {
		name := strings.TrimSuffix(filepath.Base(path), ".svg")
		ic := parse(path, name)
		ic.Tags = tags[name]
		lucide.Icons = append(lucide.Icons, ic)
	}
	wi := set{Source: "weather-icons " + *wiSource}
	for _, path := range glob(filepath.Join(*wiDir, "svg", "wi-*.svg")) {
		wi.Icons = append(wi.Icons, parse(path, strings.TrimSuffix(filepath.Base(path), ".svg")))
	}

	write(filepath.Join(*out, "lucide.json.gz"), lucide)
	write(filepath.Join(*out, "wi.json.gz"), wi)
	copyFile(filepath.Join(*lucideDir, "LICENSE"), filepath.Join(*out, "LICENSE-lucide"))
	fmt.Printf("vendored %d lucide and %d weather icons into %s\n", len(lucide.Icons), len(wi.Icons), *out)
}

func parse(path, name string) icon {
	raw, err := os.ReadFile(path)
	if err != nil {
		log.Fatal(err)
	}
	s := comment.ReplaceAllString(string(raw), "")
	open := svgOpen.FindStringIndex(s)
	end := strings.LastIndex(s, "</svg>")
	if open == nil || end < open[1] {
		log.Fatalf("%s: no <svg> element", path)
	}
	vb := viewBoxAttr.FindStringSubmatch(s[open[0]:open[1]])
	if vb == nil {
		log.Fatalf("%s: no viewBox", path)
	}
	body := strings.TrimSpace(space.ReplaceAllString(s[open[1]:end], " "))
	for _, t := range tag.FindAllString(body, -1) {
		if !allowed.MatchString(t) || strings.Contains(t, "href") || strings.Contains(t, " on") {
			log.Fatalf("%s: unexpected element %s", path, t)
		}
	}
	return icon{Name: name, ViewBox: vb[1], Body: body}
}

func glob(pattern string) []string {
	paths, err := filepath.Glob(pattern)
	if err != nil || len(paths) == 0 {
		log.Fatalf("no files match %s", pattern)
	}
	sort.Strings(paths)
	return paths
}

func readJSON(path string, v any) {
	raw, err := os.ReadFile(path)
	if err != nil {
		log.Fatal(err)
	}
	if err := json.Unmarshal(raw, v); err != nil {
		log.Fatalf("%s: %v", path, err)
	}
}

func write(path string, v any) {
	f, err := os.Create(path)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	zw, _ := gzip.NewWriterLevel(f, gzip.BestCompression)
	if err := json.NewEncoder(zw).Encode(v); err != nil {
		log.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		log.Fatal(err)
	}
}

func copyFile(from, to string) {
	raw, err := os.ReadFile(from)
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(to, raw, 0o644); err != nil {
		log.Fatal(err)
	}
}
