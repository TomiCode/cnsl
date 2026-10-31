package store

import (
	"context"
	"database/sql"
	"fmt"
)

type Connector struct {
	ID        int64  `db:"id"`
	Type      string `db:"type"`
	Name      string `db:"name"`
	Config    string `db:"config"` // JSON object
	CreatedAt Time   `db:"created_at"`
	UpdatedAt Time   `db:"updated_at"`
}

type Binding struct {
	ID          int64          `db:"id"`
	ScreenID    int64          `db:"screen_id"`
	RegionKey   sql.NullString `db:"region_key"` // NULL = all regions
	ConnectorID int64          `db:"connector_id"`
	Alias       string         `db:"alias"`
	Params      string         `db:"params"` // JSON object
	TTL         int            `db:"ttl"`    // seconds
	Enabled     bool           `db:"enabled"`
}

type CacheEntry struct {
	Key         string         `db:"key"`
	ConnectorID int64          `db:"connector_id"`
	Data        sql.NullString `db:"data"` // JSON
	FetchedAt   NullTime       `db:"fetched_at"`
	ExpiresAt   Time           `db:"expires_at"`
	LastError   sql.NullString `db:"last_error"`
	LastErrorAt NullTime       `db:"last_error_at"`
}

func (s *Store) Connectors(ctx context.Context) ([]Connector, error) {
	var out []Connector
	err := s.DB.SelectContext(ctx, &out, `SELECT * FROM connectors ORDER BY name, id`)
	return out, err
}

func (s *Store) Connector(ctx context.Context, id int64) (Connector, error) {
	var c Connector
	err := s.DB.GetContext(ctx, &c, `SELECT * FROM connectors WHERE id = ?`, id)
	return c, notFound(err)
}

func (s *Store) SaveConnector(ctx context.Context, c *Connector) error {
	now := Now()
	c.UpdatedAt = now
	if c.ID == 0 {
		c.CreatedAt = now
		res, err := s.DB.NamedExecContext(ctx, `
			INSERT INTO connectors (type, name, config, created_at, updated_at)
			VALUES (:type, :name, :config, :created_at, :updated_at)`, c)
		if err != nil {
			return fmt.Errorf("insert connector: %w", err)
		}
		c.ID, err = res.LastInsertId()
		return err
	}
	res, err := s.DB.NamedExecContext(ctx, `
		UPDATE connectors SET name = :name, config = :config, updated_at = :updated_at WHERE id = :id`, c)
	if err != nil {
		return fmt.Errorf("update connector: %w", err)
	}
	if err := affected(res); err != nil {
		return err
	}
	// Config changed: cached data may be wrong now.
	_, err = s.DB.ExecContext(ctx, `DELETE FROM connector_cache WHERE connector_id = ?`, c.ID)
	return err
}

func (s *Store) DeleteConnector(ctx context.Context, id int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM connectors WHERE id = ?`, id)
	if err != nil {
		return err
	}
	return affected(res)
}

// ConnectorUsage counts bindings per connector.
func (s *Store) ConnectorUsage(ctx context.Context) (map[int64]int, error) {
	var rows []struct {
		ConnectorID int64 `db:"connector_id"`
		N           int   `db:"n"`
	}
	if err := s.DB.SelectContext(ctx, &rows, `SELECT connector_id, COUNT(*) AS n FROM bindings GROUP BY connector_id`); err != nil {
		return nil, err
	}
	out := make(map[int64]int, len(rows))
	for _, r := range rows {
		out[r.ConnectorID] = r.N
	}
	return out, nil
}

func (s *Store) Bindings(ctx context.Context, screenID int64) ([]Binding, error) {
	var out []Binding
	err := s.DB.SelectContext(ctx, &out, `SELECT * FROM bindings WHERE screen_id = ? ORDER BY id`, screenID)
	return out, err
}

// ReplaceBindings makes the screen's bindings exactly the given list (matched by id).
func (s *Store) ReplaceBindings(ctx context.Context, screenID int64, bindings []Binding) error {
	tx, err := s.DB.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	keep := []any{screenID}
	for i := range bindings {
		b := &bindings[i]
		b.ScreenID = screenID
		if b.ID != 0 {
			res, err := tx.NamedExecContext(ctx, `
				UPDATE bindings SET region_key = :region_key, connector_id = :connector_id, alias = :alias,
					params = :params, ttl = :ttl, enabled = :enabled
				WHERE id = :id AND screen_id = :screen_id`, b)
			if err != nil {
				return fmt.Errorf("update binding: %w", err)
			}
			if n, _ := res.RowsAffected(); n == 1 {
				keep = append(keep, b.ID)
				continue
			}
		}
		res, err := tx.NamedExecContext(ctx, `
			INSERT INTO bindings (screen_id, region_key, connector_id, alias, params, ttl, enabled)
			VALUES (:screen_id, :region_key, :connector_id, :alias, :params, :ttl, :enabled)`, b)
		if err != nil {
			return fmt.Errorf("insert binding: %w", err)
		}
		if b.ID, err = res.LastInsertId(); err != nil {
			return err
		}
		keep = append(keep, b.ID)
	}
	q := `DELETE FROM bindings WHERE screen_id = ?`
	if len(keep) > 1 {
		q += ` AND id NOT IN (?` + repeatPlaceholders(len(keep)-2) + `)`
	}
	if _, err := tx.ExecContext(ctx, q, keep...); err != nil {
		return err
	}
	return tx.Commit()
}

// ActiveBindings returns enabled bindings of screens that an active device's playlist uses.
func (s *Store) ActiveBindings(ctx context.Context) ([]Binding, error) {
	var out []Binding
	err := s.DB.SelectContext(ctx, &out, `
		SELECT b.* FROM bindings b
		WHERE b.enabled = 1 AND b.screen_id IN (
			SELECT pi.screen_id FROM playlist_items pi
			JOIN devices d ON d.playlist_id = pi.playlist_id
			WHERE pi.enabled = 1 AND d.status = 'active')`)
	return out, err
}

// AllBindings returns every binding; used to find screens to re-render when data changes.
func (s *Store) AllBindings(ctx context.Context) ([]Binding, error) {
	var out []Binding
	err := s.DB.SelectContext(ctx, &out, `SELECT * FROM bindings`)
	return out, err
}

func (s *Store) CacheEntry(ctx context.Context, key string) (CacheEntry, error) {
	var e CacheEntry
	err := s.DB.GetContext(ctx, &e, `SELECT * FROM connector_cache WHERE key = ?`, key)
	return e, notFound(err)
}

func (s *Store) SaveCacheEntry(ctx context.Context, e CacheEntry) error {
	_, err := s.DB.NamedExecContext(ctx, `
		INSERT INTO connector_cache (key, connector_id, data, fetched_at, expires_at, last_error, last_error_at)
		VALUES (:key, :connector_id, :data, :fetched_at, :expires_at, :last_error, :last_error_at)
		ON CONFLICT (key) DO UPDATE SET data = excluded.data, fetched_at = excluded.fetched_at,
			expires_at = excluded.expires_at, last_error = excluded.last_error, last_error_at = excluded.last_error_at`, e)
	return err
}

func repeatPlaceholders(n int) string {
	out := make([]byte, 0, 2*n)
	for range n {
		out = append(out, ',', '?')
	}
	return string(out)
}
