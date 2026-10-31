package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

type Playlist struct {
	ID        int64  `db:"id"`
	Name      string `db:"name"`
	CreatedAt Time   `db:"created_at"`
	UpdatedAt Time   `db:"updated_at"`
}

type PlaylistItem struct {
	ID          int64         `db:"id"`
	PlaylistID  int64         `db:"playlist_id"`
	ScreenID    int64         `db:"screen_id"`
	Position    int           `db:"position"`
	Dwell       int           `db:"dwell"`
	RefreshRate sql.NullInt64 `db:"refresh_rate"`
	Enabled     bool          `db:"enabled"`
}

type Playback struct {
	DeviceID   int64         `db:"device_id"`
	ItemID     sql.NullInt64 `db:"item_id"`
	ShownCount int           `db:"shown_count"`
}

func (s *Store) Playlists(ctx context.Context) ([]Playlist, error) {
	var out []Playlist
	err := s.DB.SelectContext(ctx, &out, `SELECT * FROM playlists ORDER BY name, id`)
	return out, err
}

func (s *Store) Playlist(ctx context.Context, id int64) (Playlist, error) {
	var p Playlist
	err := s.DB.GetContext(ctx, &p, `SELECT * FROM playlists WHERE id = ?`, id)
	return p, notFound(err)
}

func (s *Store) PlaylistItems(ctx context.Context, playlistID int64) ([]PlaylistItem, error) {
	var out []PlaylistItem
	err := s.DB.SelectContext(ctx, &out, `SELECT * FROM playlist_items WHERE playlist_id = ? ORDER BY position, id`, playlistID)
	return out, err
}

// SavePlaylist creates (ID 0) or updates a playlist and replaces its items. Items keep their
// id when they have one, so device playback positions survive reordering.
func (s *Store) SavePlaylist(ctx context.Context, p *Playlist, items []PlaylistItem) error {
	tx, err := s.DB.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	now := Now()
	p.UpdatedAt = now
	if p.ID == 0 {
		p.CreatedAt = now
		res, err := tx.NamedExecContext(ctx, `INSERT INTO playlists (name, created_at, updated_at) VALUES (:name, :created_at, :updated_at)`, p)
		if err != nil {
			return fmt.Errorf("insert playlist: %w", err)
		}
		if p.ID, err = res.LastInsertId(); err != nil {
			return err
		}
	} else {
		res, err := tx.NamedExecContext(ctx, `UPDATE playlists SET name = :name, updated_at = :updated_at WHERE id = :id`, p)
		if err != nil {
			return fmt.Errorf("update playlist: %w", err)
		}
		if err := affected(res); err != nil {
			return err
		}
	}

	keep := []any{p.ID}
	placeholders := "?"
	for i := range items {
		it := &items[i]
		it.PlaylistID = p.ID
		it.Position = i
		if it.ID != 0 {
			res, err := tx.NamedExecContext(ctx, `
				UPDATE playlist_items SET screen_id = :screen_id, position = :position, dwell = :dwell,
					refresh_rate = :refresh_rate, enabled = :enabled
				WHERE id = :id AND playlist_id = :playlist_id`, it)
			if err != nil {
				return fmt.Errorf("update playlist item: %w", err)
			}
			if n, _ := res.RowsAffected(); n == 1 {
				keep = append(keep, it.ID)
				placeholders += ",?"
				continue
			}
		}
		res, err := tx.NamedExecContext(ctx, `
			INSERT INTO playlist_items (playlist_id, screen_id, position, dwell, refresh_rate, enabled)
			VALUES (:playlist_id, :screen_id, :position, :dwell, :refresh_rate, :enabled)`, it)
		if err != nil {
			return fmt.Errorf("insert playlist item: %w", err)
		}
		if it.ID, err = res.LastInsertId(); err != nil {
			return err
		}
		keep = append(keep, it.ID)
		placeholders += ",?"
	}
	// Delete items that are no longer in the list (first placeholder is the playlist id).
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM playlist_items WHERE playlist_id = ? AND id NOT IN (`+placeholders[2:]+`)`, keep...); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) DeletePlaylist(ctx context.Context, id int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM playlists WHERE id = ?`, id)
	if err != nil {
		return err
	}
	return affected(res)
}

// PlaylistUsage counts devices per playlist.
func (s *Store) PlaylistUsage(ctx context.Context) (map[int64]int, error) {
	var rows []struct {
		PlaylistID int64 `db:"playlist_id"`
		N          int   `db:"n"`
	}
	if err := s.DB.SelectContext(ctx, &rows, `SELECT playlist_id, COUNT(*) AS n FROM devices WHERE playlist_id IS NOT NULL GROUP BY playlist_id`); err != nil {
		return nil, err
	}
	out := make(map[int64]int, len(rows))
	for _, r := range rows {
		out[r.PlaylistID] = r.N
	}
	return out, nil
}

func (s *Store) Playback(ctx context.Context, deviceID int64) (Playback, error) {
	p := Playback{DeviceID: deviceID}
	err := s.DB.GetContext(ctx, &p, `SELECT * FROM device_playback WHERE device_id = ?`, deviceID)
	if errors.Is(err, sql.ErrNoRows) {
		return p, nil
	}
	return p, err
}

func (s *Store) SavePlayback(ctx context.Context, p Playback) error {
	_, err := s.DB.NamedExecContext(ctx, `
		INSERT INTO device_playback (device_id, item_id, shown_count) VALUES (:device_id, :item_id, :shown_count)
		ON CONFLICT (device_id) DO UPDATE SET item_id = excluded.item_id, shown_count = excluded.shown_count`, p)
	return err
}

// PlayableItems returns a playlist's enabled items whose screen targets the given model, in order.
func (s *Store) PlayableItems(ctx context.Context, playlistID, modelID int64) ([]PlaylistItem, error) {
	var out []PlaylistItem
	err := s.DB.SelectContext(ctx, &out, `
		SELECT pi.* FROM playlist_items pi JOIN screens sc ON sc.id = pi.screen_id
		WHERE pi.playlist_id = ? AND pi.enabled = 1 AND sc.model_id = ?
		ORDER BY pi.position, pi.id`, playlistID, modelID)
	return out, err
}
