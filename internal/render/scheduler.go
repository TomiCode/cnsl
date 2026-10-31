package render

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/TomiCode/cnsl/internal/events"
	"github.com/TomiCode/cnsl/internal/store"
)

// Scheduler decides when screens are rendered: on request (edits, playlists, devices waiting
// for an image) and on each screen's interval while the screen is in use by an active device.
// Requests for the same screen are debounced and coalesced.
type Scheduler struct {
	r      *Renderer
	store  *store.Store
	events *events.Broker
	log    *slog.Logger

	mu      sync.Mutex
	pending map[int64]string // screen → trigger, queued or waiting for the debounce
	queue   chan job
}

type job struct {
	screenID int64
	trigger  string
}

const (
	debounce     = time.Second
	tickInterval = 30 * time.Second
	workers      = 2
)

func NewScheduler(r *Renderer, st *store.Store, ev *events.Broker, log *slog.Logger) *Scheduler {
	return &Scheduler{r: r, store: st, events: ev, log: log, pending: map[int64]string{}, queue: make(chan job, 256)}
}

// Request queues a render of the screen after a short debounce. Repeated requests while one is
// pending collapse into one render.
func (s *Scheduler) Request(screenID int64, trigger string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.pending[screenID]; ok {
		return
	}
	s.pending[screenID] = trigger
	time.AfterFunc(debounce, func() {
		select {
		case s.queue <- job{screenID: screenID, trigger: trigger}:
		default:
			s.log.Warn("render queue full, dropping request", "screen", screenID)
			s.mu.Lock()
			delete(s.pending, screenID)
			s.mu.Unlock()
		}
	})
}

// Run processes renders until ctx ends.
func (s *Scheduler) Run(ctx context.Context) {
	var wg sync.WaitGroup
	for range workers {
		wg.Go(func() {
			for {
				select {
				case <-ctx.Done():
					return
				case j := <-s.queue:
					s.mu.Lock()
					delete(s.pending, j.screenID) // edits during the render queue a new one
					s.mu.Unlock()
					s.render(ctx, j)
				}
			}
		})
	}

	s.tick(ctx)
	t := time.NewTicker(tickInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			wg.Wait()
			return
		case <-t.C:
			s.tick(ctx)
		}
	}
}

func (s *Scheduler) render(ctx context.Context, j job) {
	rctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	rec, err := s.r.RenderAndStore(rctx, j.screenID, j.trigger)
	if errors.Is(err, store.ErrNotFound) {
		return // screen deleted meanwhile
	}
	if err != nil {
		s.log.Error("render", "screen", j.screenID, "err", err)
		return
	}
	s.log.Debug("rendered screen", "screen", j.screenID, "trigger", j.trigger, "status", rec.Status, "ms", rec.DurationMS)
	s.events.Publish(events.Event{Type: events.ScreenRendered, ScreenID: j.screenID})
}

// tick re-renders in-use screens whose last render is older than their interval.
func (s *Scheduler) tick(ctx context.Context) {
	ids, err := s.store.ScreensInUse(ctx)
	if err != nil {
		s.log.Error("scheduler: screens in use", "err", err)
		return
	}
	now := time.Now()
	for _, id := range ids {
		sc, err := s.store.Screen(ctx, id)
		if err != nil {
			continue
		}
		last, err := s.store.LatestRender(ctx, id, false)
		if errors.Is(err, store.ErrNotFound) || (err == nil && now.Sub(last.CreatedAt.Time) >= time.Duration(sc.RenderInterval)*time.Second) {
			s.Request(id, "schedule")
		}
	}
}

// LatestImage returns the filename of the screen's newest successful render.
func (s *Scheduler) LatestImage(ctx context.Context, screenID int64) (string, bool, error) {
	r, err := s.store.LatestRender(ctx, screenID, true)
	if errors.Is(err, store.ErrNotFound) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return r.Filename.String, r.Filename.Valid, nil
}

// Calibration renders (or reuses) the calibration pattern for a model.
func (s *Scheduler) Calibration(ctx context.Context, modelID int64) (string, error) {
	return s.r.Calibration(ctx, modelID)
}
