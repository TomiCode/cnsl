package dashboard

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Events streams change notifications as Server-Sent Events (GET /api/v1/events). It is
// documented in the spec but hand-written, since the strict server can't stream.
func (s *Server) Events(w http.ResponseWriter, r *http.Request) {
	rc := http.NewResponseController(w)
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("X-Accel-Buffering", "no") // don't let reverse proxies buffer the stream
	w.WriteHeader(http.StatusOK)

	events, unsubscribe := s.events.Subscribe()
	defer unsubscribe()

	// Tell the client to reconnect quickly and confirm the stream is open.
	fmt.Fprint(w, "retry: 3000\n: connected\n\n")
	if err := rc.Flush(); err != nil {
		return
	}

	keepalive := time.NewTicker(25 * time.Second)
	defer keepalive.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-keepalive.C:
			fmt.Fprint(w, ": keepalive\n\n")
		case e := <-events:
			data, err := json.Marshal(e)
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", e.Type, data)
		}
		if err := rc.Flush(); err != nil {
			return
		}
	}
}
