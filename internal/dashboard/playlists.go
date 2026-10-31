package dashboard

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/TomiCode/cnsl/internal/dashboard/gen"
	"github.com/TomiCode/cnsl/internal/events"
	"github.com/TomiCode/cnsl/internal/store"
)

func (s *Server) ListPlaylists(ctx context.Context, _ gen.ListPlaylistsRequestObject) (gen.ListPlaylistsResponseObject, error) {
	playlists, err := s.store.Playlists(ctx)
	if err != nil {
		return nil, err
	}
	usage, err := s.store.PlaylistUsage(ctx)
	if err != nil {
		return nil, err
	}
	out := make(gen.ListPlaylists200JSONResponse, 0, len(playlists))
	for _, p := range playlists {
		pl, err := s.playlistResponse(ctx, p, usage[p.ID])
		if err != nil {
			return nil, err
		}
		out = append(out, gen.Playlist(pl))
	}
	return out, nil
}

func (s *Server) playlistResponse(ctx context.Context, p store.Playlist, devices int) (gen.PlaylistJSONResponse, error) {
	items, err := s.store.PlaylistItems(ctx, p.ID)
	if err != nil {
		return gen.PlaylistJSONResponse{}, err
	}
	out := gen.PlaylistJSONResponse{Id: p.ID, Name: p.Name, DeviceCount: devices, UpdatedAt: p.UpdatedAt.Time,
		Items: make([]gen.PlaylistItem, 0, len(items))}
	for _, it := range items {
		id := it.ID
		gi := gen.PlaylistItem{Id: &id, ScreenId: it.ScreenID, Dwell: it.Dwell, Enabled: it.Enabled}
		if it.RefreshRate.Valid {
			r := int(it.RefreshRate.Int64)
			gi.RefreshRate = &r
		}
		out.Items = append(out.Items, gi)
	}
	return out, nil
}

func (s *Server) GetPlaylist(ctx context.Context, req gen.GetPlaylistRequestObject) (gen.GetPlaylistResponseObject, error) {
	p, err := s.store.Playlist(ctx, req.Id)
	if errors.Is(err, store.ErrNotFound) {
		return gen.GetPlaylist404ApplicationProblemPlusJSONResponse{ProblemApplicationProblemPlusJSONResponse: notFoundProblem("playlist")}, nil
	}
	if err != nil {
		return nil, err
	}
	usage, err := s.store.PlaylistUsage(ctx)
	if err != nil {
		return nil, err
	}
	resp, err := s.playlistResponse(ctx, p, usage[p.ID])
	return gen.GetPlaylist200JSONResponse{PlaylistJSONResponse: resp}, err
}

// savePlaylist validates the input and writes it; id 0 creates a new playlist.
func (s *Server) savePlaylist(ctx context.Context, id int64, in gen.PlaylistInput) (gen.PlaylistJSONResponse, error) {
	p := store.Playlist{ID: id, Name: strings.TrimSpace(in.Name)}
	if p.Name == "" {
		return gen.PlaylistJSONResponse{}, fmt.Errorf("%w: name is required", errInvalidInput)
	}
	if id != 0 {
		existing, err := s.store.Playlist(ctx, id)
		if err != nil {
			return gen.PlaylistJSONResponse{}, err
		}
		p.CreatedAt = existing.CreatedAt
	}
	items := make([]store.PlaylistItem, 0, len(in.Items))
	for i, it := range in.Items {
		if _, err := s.store.Screen(ctx, it.ScreenId); errors.Is(err, store.ErrNotFound) {
			return gen.PlaylistJSONResponse{}, fmt.Errorf("%w: item %d: unknown screen %d", errInvalidInput, i+1, it.ScreenId)
		} else if err != nil {
			return gen.PlaylistJSONResponse{}, err
		}
		si := store.PlaylistItem{ScreenID: it.ScreenId, Dwell: it.Dwell, Enabled: it.Enabled}
		if it.Id != nil {
			si.ID = *it.Id
		}
		if it.RefreshRate != nil {
			si.RefreshRate = sql.NullInt64{Int64: int64(*it.RefreshRate), Valid: true}
		}
		items = append(items, si)
	}
	if err := s.store.SavePlaylist(ctx, &p, items); err != nil {
		return gen.PlaylistJSONResponse{}, err
	}
	for _, it := range items { // get renders ready before devices ask for them
		if it.Enabled {
			if _, ok, err := s.scheduler.LatestImage(ctx, it.ScreenID); err == nil && !ok {
				s.scheduler.Request(it.ScreenID, "playlist")
			}
		}
	}
	s.events.Publish(events.Event{Type: events.PlaylistUpdated, PlaylistID: p.ID})
	usage, err := s.store.PlaylistUsage(ctx)
	if err != nil {
		return gen.PlaylistJSONResponse{}, err
	}
	return s.playlistResponse(ctx, p, usage[p.ID])
}

func (s *Server) CreatePlaylist(ctx context.Context, req gen.CreatePlaylistRequestObject) (gen.CreatePlaylistResponseObject, error) {
	resp, err := s.savePlaylist(ctx, 0, *req.Body)
	if errors.Is(err, errInvalidInput) {
		return gen.CreatePlaylist422ApplicationProblemPlusJSONResponse{ProblemApplicationProblemPlusJSONResponse: invalidProblem(err)}, nil
	}
	if err != nil {
		return nil, err
	}
	return gen.CreatePlaylist201JSONResponse{PlaylistJSONResponse: resp}, nil
}

func (s *Server) UpdatePlaylist(ctx context.Context, req gen.UpdatePlaylistRequestObject) (gen.UpdatePlaylistResponseObject, error) {
	resp, err := s.savePlaylist(ctx, req.Id, *req.Body)
	switch {
	case errors.Is(err, store.ErrNotFound):
		return gen.UpdatePlaylist404ApplicationProblemPlusJSONResponse{ProblemApplicationProblemPlusJSONResponse: notFoundProblem("playlist")}, nil
	case errors.Is(err, errInvalidInput):
		return gen.UpdatePlaylist422ApplicationProblemPlusJSONResponse(invalidProblem(err)), nil
	case err != nil:
		return nil, err
	}
	return gen.UpdatePlaylist200JSONResponse{PlaylistJSONResponse: resp}, nil
}

func (s *Server) DeletePlaylist(ctx context.Context, req gen.DeletePlaylistRequestObject) (gen.DeletePlaylistResponseObject, error) {
	err := s.store.DeletePlaylist(ctx, req.Id)
	if errors.Is(err, store.ErrNotFound) {
		return gen.DeletePlaylist404ApplicationProblemPlusJSONResponse{ProblemApplicationProblemPlusJSONResponse: notFoundProblem("playlist")}, nil
	}
	if err != nil {
		return nil, err
	}
	s.events.Publish(events.Event{Type: events.PlaylistUpdated, PlaylistID: req.Id})
	return gen.DeletePlaylist204Response{}, nil
}
