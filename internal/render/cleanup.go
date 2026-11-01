package render

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/TomiCode/cnsl/internal/imaging"
)

const (
	cacheMaxAge  = 14 * 24 * time.Hour // processed asset variants unused this long are removed
	orphanMaxAge = 7 * 24 * time.Hour  // rendered images no render references any more
	tmpMaxAge    = 24 * time.Hour      // leftovers of interrupted writes
)

// RunCleanup removes stale files from the cache and images directories, once at start and then
// every interval, until ctx ends.
func (r *Renderer) RunCleanup(ctx context.Context, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		cache, images := r.Cleanup(ctx, time.Now())
		if cache+images > 0 {
			r.log.Info("cleanup: removed stale files", "cache", cache, "images", images)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// Cleanup removes processed asset variants not used for cacheMaxAge (cache hits refresh the file's
// modification time) and rendered images that no stored render references and that are older
// than orphanMaxAge. Placeholders and calibration patterns are recreated on demand if needed.
func (r *Renderer) Cleanup(ctx context.Context, now time.Time) (cacheRemoved, imagesRemoved int) {
	cacheRemoved = removeOlder(r.cacheDir, now.Add(-cacheMaxAge), func(string) bool { return true })

	referenced, err := r.store.RenderFilenames(ctx)
	if err != nil {
		r.log.WarnContext(ctx, "cleanup: render filenames", "err", err)
		return cacheRemoved, 0
	}
	imagesRemoved = removeOlder(r.files.Dir, now.Add(-orphanMaxAge), func(name string) bool {
		return imaging.ValidName(name) && !referenced[name]
	})
	imagesRemoved += removeOlder(r.files.Dir, now.Add(-tmpMaxAge), func(name string) bool {
		return strings.HasPrefix(name, "tmp-")
	})
	return cacheRemoved, imagesRemoved
}

// removeOlder deletes regular files in dir modified before cutoff for which match returns true.
func removeOlder(dir string, cutoff time.Time, match func(name string) bool) int {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	n := 0
	for _, e := range entries {
		if !e.Type().IsRegular() || !match(e.Name()) {
			continue
		}
		info, err := e.Info()
		if err != nil || !info.ModTime().Before(cutoff) {
			continue
		}
		if os.Remove(filepath.Join(dir, e.Name())) == nil {
			n++
		}
	}
	return n
}

// touch marks a cache file as recently used.
func touch(path string) {
	now := time.Now()
	_ = os.Chtimes(path, now, now)
}
