package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

type Screen struct {
	ID               int64  `db:"id"`
	Name             string `db:"name"`
	ModelID          int64  `db:"model_id"`
	FrameworkVersion string `db:"framework_version"`
	Head             string `db:"head"`
	RenderInterval   int    `db:"render_interval"`
	CreatedAt        Time   `db:"created_at"`
	UpdatedAt        Time   `db:"updated_at"`
	Kind             string `db:"kind"`             // "template" | "image"
	Layout           string `db:"layout"`           // "full", a mashup key, or "free"
	Background       string `db:"background"`       // JSON, see render.Background
	BackgroundIndex  int    `db:"background_index"` // slideshow position
}

type Region struct {
	ID          int64  `db:"id"`
	ScreenID    int64  `db:"screen_id"`
	Key         string `db:"key"`
	X           int    `db:"x"`
	Y           int    `db:"y"`
	W           int    `db:"w"`
	H           int    `db:"h"`
	Z           int    `db:"z"`
	Template    string `db:"template"`
	Class       string `db:"class"`
	Transparent bool   `db:"transparent"`
}

type Render struct {
	ID         int64          `db:"id"`
	ScreenID   int64          `db:"screen_id"`
	Filename   sql.NullString `db:"filename"`
	Status     string         `db:"status"` // "ok" | "error"
	Error      sql.NullString `db:"error"`
	DurationMS int64          `db:"duration_ms"`
	Trigger    string         `db:"trigger"`
	CreatedAt  Time           `db:"created_at"`
}

func (s *Store) Screens(ctx context.Context) ([]Screen, error) {
	var screens []Screen
	err := s.DB.SelectContext(ctx, &screens, `SELECT * FROM screens ORDER BY name, id`)
	return screens, err
}

func (s *Store) Screen(ctx context.Context, id int64) (Screen, error) {
	var sc Screen
	err := s.DB.GetContext(ctx, &sc, `SELECT * FROM screens WHERE id = ?`, id)
	return sc, notFound(err)
}

func (s *Store) Regions(ctx context.Context, screenID int64) ([]Region, error) {
	var regions []Region
	err := s.DB.SelectContext(ctx, &regions, `SELECT * FROM regions WHERE screen_id = ? ORDER BY z, id`, screenID)
	return regions, err
}

// CreateScreen inserts a screen together with its regions.
func (s *Store) CreateScreen(ctx context.Context, sc *Screen, regions []Region) error {
	tx, err := s.DB.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	now := Now()
	sc.CreatedAt, sc.UpdatedAt = now, now
	if sc.Kind == "" {
		sc.Kind = "template"
	}
	if sc.Layout == "" {
		sc.Layout = "full"
	}
	if sc.Background == "" {
		sc.Background = `{"kind":"none"}`
	}
	res, err := tx.NamedExecContext(ctx, `
		INSERT INTO screens (name, model_id, framework_version, head, render_interval, created_at, updated_at,
			kind, layout, background)
		VALUES (:name, :model_id, :framework_version, :head, :render_interval, :created_at, :updated_at,
			:kind, :layout, :background)`, sc)
	if err != nil {
		return fmt.Errorf("insert screen: %w", err)
	}
	if sc.ID, err = res.LastInsertId(); err != nil {
		return err
	}
	for i := range regions {
		regions[i].ScreenID = sc.ID
		res, err := tx.NamedExecContext(ctx, `
			INSERT INTO regions (screen_id, key, x, y, w, h, z, template, class, transparent)
			VALUES (:screen_id, :key, :x, :y, :w, :h, :z, :template, :class, :transparent)`, &regions[i])
		if err != nil {
			return fmt.Errorf("insert region: %w", err)
		}
		if regions[i].ID, err = res.LastInsertId(); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// UpdateScreen writes the screen row and makes its regions exactly the given list: regions with
// an id are updated, without an id inserted, and regions not in the list deleted.
func (s *Store) UpdateScreen(ctx context.Context, sc *Screen, regions []Region) error {
	tx, err := s.DB.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	sc.UpdatedAt = Now()
	res, err := tx.NamedExecContext(ctx, `
		UPDATE screens SET name = :name, model_id = :model_id, framework_version = :framework_version,
			head = :head, render_interval = :render_interval, updated_at = :updated_at,
			kind = :kind, layout = :layout, background = :background
		WHERE id = :id`, sc)
	if err != nil {
		return fmt.Errorf("update screen: %w", err)
	}
	if err := affected(res); err != nil {
		return err
	}

	keep := []any{sc.ID}
	for i := range regions {
		r := &regions[i]
		r.ScreenID = sc.ID
		if r.ID != 0 {
			if _, err := tx.NamedExecContext(ctx, `
				UPDATE regions SET key = :key, x = :x, y = :y, w = :w, h = :h, z = :z,
					template = :template, class = :class, transparent = :transparent
				WHERE id = :id AND screen_id = :screen_id`, r); err != nil {
				return fmt.Errorf("update region: %w", err)
			}
		} else {
			res, err := tx.NamedExecContext(ctx, `
				INSERT INTO regions (screen_id, key, x, y, w, h, z, template, class, transparent)
				VALUES (:screen_id, :key, :x, :y, :w, :h, :z, :template, :class, :transparent)`, r)
			if err != nil {
				return fmt.Errorf("insert region: %w", err)
			}
			if r.ID, err = res.LastInsertId(); err != nil {
				return err
			}
		}
		keep = append(keep, r.ID)
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(keep)-1), ",")
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM regions WHERE screen_id = ? AND id NOT IN (`+placeholders+`)`, keep...); err != nil {
		return err
	}
	return tx.Commit()
}

// SetBackgroundIndex stores the slideshow position of a screen.
func (s *Store) SetBackgroundIndex(ctx context.Context, screenID int64, index int) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE screens SET background_index = ? WHERE id = ?`, index, screenID)
	return err
}

func (s *Store) DeleteScreen(ctx context.Context, id int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM screens WHERE id = ?`, id)
	if err != nil {
		return err
	}
	return affected(res)
}

func (s *Store) InsertRender(ctx context.Context, r *Render) error {
	r.CreatedAt = Now()
	res, err := s.DB.NamedExecContext(ctx, `
		INSERT INTO renders (screen_id, filename, status, error, duration_ms, trigger, created_at)
		VALUES (:screen_id, :filename, :status, :error, :duration_ms, :trigger, :created_at)`, r)
	if err != nil {
		return fmt.Errorf("insert render: %w", err)
	}
	r.ID, err = res.LastInsertId()
	return err
}

// LatestRender returns the newest render of a screen; okOnly skips failed ones.
func (s *Store) LatestRender(ctx context.Context, screenID int64, okOnly bool) (Render, error) {
	q := `SELECT * FROM renders WHERE screen_id = ?`
	if okOnly {
		q += ` AND status = 'ok'`
	}
	var r Render
	err := s.DB.GetContext(ctx, &r, q+` ORDER BY id DESC LIMIT 1`, screenID)
	return r, notFound(err)
}

func (s *Store) Renders(ctx context.Context, screenID int64, limit int) ([]Render, error) {
	var rows []Render
	err := s.DB.SelectContext(ctx, &rows, `SELECT * FROM renders WHERE screen_id = ? ORDER BY id DESC LIMIT ?`, screenID, limit)
	return rows, err
}

// PruneRenders keeps the newest keep renders per screen and returns the filenames of deleted
// rows that no remaining render still references.
func (s *Store) PruneRenders(ctx context.Context, screenID int64, keep int) ([]string, error) {
	var names []string
	err := s.DB.SelectContext(ctx, &names, `
		SELECT DISTINCT filename FROM renders
		WHERE screen_id = ? AND filename IS NOT NULL
		  AND id NOT IN (SELECT id FROM renders WHERE screen_id = ? ORDER BY id DESC LIMIT ?)`,
		screenID, screenID, keep)
	if err != nil {
		return nil, err
	}
	if _, err := s.DB.ExecContext(ctx, `
		DELETE FROM renders WHERE screen_id = ?
		  AND id NOT IN (SELECT id FROM renders WHERE screen_id = ? ORDER BY id DESC LIMIT ?)`,
		screenID, screenID, keep); err != nil {
		return nil, err
	}
	var unused []string
	for _, n := range names {
		var refs int
		if err := s.DB.GetContext(ctx, &refs, `
			SELECT COUNT(*) FROM renders WHERE filename = ?`, n); err != nil {
			return nil, err
		}
		if refs == 0 {
			unused = append(unused, n)
		}
	}
	return unused, nil
}

// ScreensInUse returns ids of screens that are in an enabled item of a playlist assigned to an
// active device — the ones the scheduler keeps fresh.
func (s *Store) ScreensInUse(ctx context.Context) ([]int64, error) {
	var ids []int64
	err := s.DB.SelectContext(ctx, &ids, `
		SELECT DISTINCT pi.screen_id FROM playlist_items pi
		JOIN devices d ON d.playlist_id = pi.playlist_id
		WHERE pi.enabled = 1 AND d.status = 'active'`)
	return ids, err
}
