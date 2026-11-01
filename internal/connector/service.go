package connector

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/TomiCode/cnsl/internal/store"
)

const (
	fetchTimeout = 15 * time.Second
	refreshEvery = 20 * time.Second
	MinTTL       = 30
	MaxTTL       = 86400
)

// Service resolves bindings to data, with caching, and keeps in-use data fresh.
type Service struct {
	store *store.Store
	log   *slog.Logger
	env   Env
	box   Sealer // encrypts secret config fields at rest

	// OnChange is called with the screens whose data changed after a background refresh.
	OnChange func(screenIDs []int64)

	mu       sync.Mutex
	inFlight map[string]*sync.Mutex // one fetch per cache key at a time
}

func NewService(st *store.Store, log *slog.Logger, box Sealer) *Service {
	return &Service{
		store:    st,
		log:      log,
		box:      box,
		env:      Env{HTTP: &http.Client{Timeout: fetchTimeout}},
		inFlight: map[string]*sync.Mutex{},
	}
}

// CacheKey identifies the data a binding asks for: same connector and params share a cache entry.
func CacheKey(connectorID int64, params Values) string {
	b, _ := json.Marshal(params) // map keys are sorted, so this is canonical
	sum := sha256.Sum256(fmt.Appendf(nil, "%d|%s", connectorID, b))
	return hex.EncodeToString(sum[:16])
}

// Result is the data of one binding as templates and the editor see it.
type Result struct {
	Data      any
	Stale     bool // the last refresh failed, or the data is older than its TTL
	FetchedAt *time.Time
	Error     string
}

// Resolve returns data for a screen's bindings that apply to a region (regionKey "" = only
// screen-wide bindings), keyed by alias. Fresh cached data is used as is; missing or expired data
// is fetched now, falling back to the old data (marked stale) if that fails. With sample set,
// connector types' sample data is used instead.
func (s *Service) Resolve(ctx context.Context, screenID int64, regionKey string, sample bool) (map[string]Result, error) {
	bindings, err := s.store.Bindings(ctx, screenID)
	if err != nil {
		return nil, err
	}
	out := map[string]Result{}
	for _, b := range bindings {
		if !b.Enabled || (b.RegionKey.Valid && b.RegionKey.String != regionKey) {
			continue
		}
		out[b.Alias] = s.resolve(ctx, b, sample)
	}
	return out, nil
}

// ResolveAll returns data for every binding of a screen keyed by alias (for the editor).
func (s *Service) ResolveAll(ctx context.Context, screenID int64, sample bool) (map[string]Result, error) {
	bindings, err := s.store.Bindings(ctx, screenID)
	if err != nil {
		return nil, err
	}
	out := map[string]Result{}
	for _, b := range bindings {
		out[b.Alias] = s.resolve(ctx, b, sample)
	}
	return out, nil
}

func (s *Service) resolve(ctx context.Context, b store.Binding, sample bool) Result {
	conn, typ, config, err := s.load(ctx, b.ConnectorID)
	if err != nil {
		return Result{Error: err.Error()}
	}
	params, err := ParseValues(b.Params)
	if err != nil {
		return Result{Error: err.Error()}
	}
	if sample {
		return Result{Data: typ.Sample(params)}
	}

	key := CacheKey(conn.ID, params)
	e, err := s.store.CacheEntry(ctx, key)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return Result{Error: err.Error()}
	}
	if errors.Is(err, store.ErrNotFound) || !e.Data.Valid || time.Now().After(e.ExpiresAt.Time) {
		e, _, err = s.refresh(ctx, conn, typ, config, params, b.TTL)
		if err != nil && !e.Data.Valid {
			return Result{Error: err.Error()}
		}
	}
	return toResult(e)
}

func toResult(e store.CacheEntry) Result {
	r := Result{FetchedAt: e.FetchedAt.Ptr()}
	if e.Data.Valid {
		_ = json.Unmarshal([]byte(e.Data.String), &r.Data)
	}
	if e.LastError.Valid && (!e.FetchedAt.Valid || e.LastErrorAt.Time.After(e.FetchedAt.Time.Time)) {
		r.Error = e.LastError.String
		r.Stale = true
	}
	return r
}

func (s *Service) load(ctx context.Context, id int64) (store.Connector, Type, Values, error) {
	conn, err := s.store.Connector(ctx, id)
	if err != nil {
		return conn, nil, nil, fmt.Errorf("connector %d: %w", id, err)
	}
	typ, ok := Lookup(conn.Type)
	if !ok {
		return conn, nil, nil, fmt.Errorf("unknown connector type %q", conn.Type)
	}
	config, err := s.OpenConfig(conn.Type, conn.Config)
	return conn, typ, config, err
}

// refresh fetches and stores data for (connector, params). It returns the new cache entry and
// whether the data changed. On a failed fetch the previous data is kept and the error recorded.
func (s *Service) refresh(ctx context.Context, conn store.Connector, typ Type, config, params Values, ttl int) (store.CacheEntry, bool, error) {
	key := CacheKey(conn.ID, params)
	lock := s.keyLock(key)
	lock.Lock()
	defer lock.Unlock()

	prev, err := s.store.CacheEntry(ctx, key)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return prev, false, err
	}
	// Another caller may have refreshed while we waited for the lock.
	if err == nil && prev.Data.Valid && time.Now().Before(prev.ExpiresAt.Time) {
		return prev, false, nil
	}

	fctx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()
	data, ferr := typ.Fetch(fctx, s.env, config, params)

	e := prev
	e.Key, e.ConnectorID = key, conn.ID
	e.ExpiresAt = store.Time{Time: time.Now().Add(time.Duration(clampTTL(ttl)) * time.Second).UTC()}
	changed := false
	if ferr != nil {
		e.LastError = sql.NullString{String: ferr.Error(), Valid: true}
		e.LastErrorAt = store.NewNullTime(time.Now())
		// Retry failures sooner than the TTL, but not in a tight loop.
		e.ExpiresAt = store.Time{Time: time.Now().Add(time.Duration(min(clampTTL(ttl), 120)) * time.Second).UTC()}
		s.log.WarnContext(ctx, "connector fetch failed", "connector", conn.Name, "type", conn.Type, "err", ferr)
	} else {
		b, err := json.Marshal(data)
		if err != nil {
			return prev, false, fmt.Errorf("encode data: %w", err)
		}
		changed = !prev.Data.Valid || !bytes.Equal([]byte(prev.Data.String), b)
		e.Data = sql.NullString{String: string(b), Valid: true}
		e.FetchedAt = store.NewNullTime(time.Now())
		e.LastError, e.LastErrorAt = sql.NullString{}, store.NullTime{}
	}
	if err := s.store.SaveCacheEntry(ctx, e); err != nil {
		return e, false, err
	}
	return e, changed, ferr
}

func (s *Service) keyLock(key string) *sync.Mutex {
	s.mu.Lock()
	defer s.mu.Unlock()
	l, ok := s.inFlight[key]
	if !ok {
		l = &sync.Mutex{}
		s.inFlight[key] = l
	}
	return l
}

func clampTTL(ttl int) int {
	return min(max(ttl, MinTTL), MaxTTL)
}

// Test fetches once with the given settings, without caching (the "test" button).
func (s *Service) Test(ctx context.Context, typeKey string, config, params Values) (any, error) {
	typ, ok := Lookup(typeKey)
	if !ok {
		return nil, fmt.Errorf("unknown connector type %q", typeKey)
	}
	ctx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()
	return typ.Fetch(ctx, s.env, config, params)
}

// Run refreshes expired data of in-use bindings until ctx ends.
func (s *Service) Run(ctx context.Context) {
	t := time.NewTicker(refreshEvery)
	defer t.Stop()
	for {
		s.refreshActive(ctx)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func (s *Service) refreshActive(ctx context.Context) {
	bindings, err := s.store.ActiveBindings(ctx)
	if err != nil {
		s.log.ErrorContext(ctx, "connector refresh: bindings", "err", err)
		return
	}
	// Smallest TTL wins when several bindings share data.
	type job struct {
		conn   int64
		params Values
		ttl    int
	}
	jobs := map[string]*job{}
	for _, b := range bindings {
		params, err := ParseValues(b.Params)
		if err != nil {
			continue
		}
		key := CacheKey(b.ConnectorID, params)
		if j, ok := jobs[key]; ok {
			j.ttl = min(j.ttl, b.TTL)
		} else {
			jobs[key] = &job{conn: b.ConnectorID, params: params, ttl: b.TTL}
		}
	}

	changedKeys := map[string]bool{}
	for key, j := range jobs {
		if e, err := s.store.CacheEntry(ctx, key); err == nil && e.Data.Valid && time.Now().Before(e.ExpiresAt.Time) {
			continue
		}
		conn, typ, config, err := s.load(ctx, j.conn)
		if err != nil {
			continue
		}
		if _, changed, _ := s.refresh(ctx, conn, typ, config, j.params, j.ttl); changed {
			changedKeys[key] = true
		}
	}
	if len(changedKeys) == 0 || s.OnChange == nil {
		return
	}
	screens := map[int64]bool{}
	for _, b := range bindings {
		params, _ := ParseValues(b.Params)
		if changedKeys[CacheKey(b.ConnectorID, params)] {
			screens[b.ScreenID] = true
		}
	}
	ids := make([]int64, 0, len(screens))
	for id := range screens {
		ids = append(ids, id)
	}
	s.OnChange(ids)
}
