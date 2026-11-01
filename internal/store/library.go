package store

import (
	"context"
	"fmt"
)

type Component struct {
	ID          int64  `db:"id"`
	Key         string `db:"key"`
	Name        string `db:"name"`
	Description string `db:"description"`
	Source      string `db:"source"`
	Inputs      string `db:"inputs"` // JSON array, see tmpl.Input
	Builtin     bool   `db:"builtin"`
	UpdatedAt   Time   `db:"updated_at"`
}

type Snippet struct {
	ID          int64  `db:"id"`
	Prefix      string `db:"prefix"`
	Name        string `db:"name"`
	Description string `db:"description"`
	Body        string `db:"body"`
	Builtin     bool   `db:"builtin"`
	UpdatedAt   Time   `db:"updated_at"`
}

func (s *Store) Components(ctx context.Context) ([]Component, error) {
	var out []Component
	err := s.DB.SelectContext(ctx, &out, `SELECT * FROM components ORDER BY builtin DESC, name, id`)
	return out, err
}

func (s *Store) Component(ctx context.Context, id int64) (Component, error) {
	var c Component
	err := s.DB.GetContext(ctx, &c, `SELECT * FROM components WHERE id = ?`, id)
	return c, notFound(err)
}

func (s *Store) ComponentByKey(ctx context.Context, key string) (Component, error) {
	var c Component
	err := s.DB.GetContext(ctx, &c, `SELECT * FROM components WHERE key = ?`, key)
	return c, notFound(err)
}

func (s *Store) SaveComponent(ctx context.Context, c *Component) error {
	c.UpdatedAt = Now()
	if c.ID == 0 {
		res, err := s.DB.NamedExecContext(ctx, `
			INSERT INTO components (key, name, description, source, inputs, builtin, updated_at)
			VALUES (:key, :name, :description, :source, :inputs, :builtin, :updated_at)`, c)
		if err != nil {
			return fmt.Errorf("insert component: %w", err)
		}
		c.ID, err = res.LastInsertId()
		return err
	}
	res, err := s.DB.NamedExecContext(ctx, `
		UPDATE components SET key = :key, name = :name, description = :description, source = :source,
			inputs = :inputs, updated_at = :updated_at
		WHERE id = :id`, c)
	if err != nil {
		return fmt.Errorf("update component: %w", err)
	}
	return affected(res)
}

func (s *Store) DeleteComponent(ctx context.Context, id int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM components WHERE id = ?`, id)
	if err != nil {
		return err
	}
	return affected(res)
}

// UpsertBuiltinComponent creates or refreshes a built-in component by key. A user component that
// already uses the key keeps it; the built-in is skipped then.
func (s *Store) UpsertBuiltinComponent(ctx context.Context, c Component) error {
	c.Builtin = true
	c.UpdatedAt = Now()
	_, err := s.DB.NamedExecContext(ctx, `
		INSERT INTO components (key, name, description, source, inputs, builtin, updated_at)
		VALUES (:key, :name, :description, :source, :inputs, 1, :updated_at)
		ON CONFLICT (key) DO UPDATE SET name = excluded.name, description = excluded.description,
			source = excluded.source, inputs = excluded.inputs, updated_at = excluded.updated_at
		WHERE components.builtin = 1`, c)
	return err
}

// ScreensUsingComponent returns ids of screens whose region templates call the component.
func (s *Store) ScreensUsingComponent(ctx context.Context, key string) ([]int64, error) {
	var ids []int64
	err := s.DB.SelectContext(ctx, &ids, `
		SELECT DISTINCT screen_id FROM regions WHERE template LIKE ? ESCAPE '\'`,
		`%component "`+escapeLike(key)+`"%`)
	return ids, err
}

func (s *Store) Snippets(ctx context.Context) ([]Snippet, error) {
	var out []Snippet
	err := s.DB.SelectContext(ctx, &out, `SELECT * FROM snippets ORDER BY builtin DESC, prefix`)
	return out, err
}

func (s *Store) Snippet(ctx context.Context, id int64) (Snippet, error) {
	var sn Snippet
	err := s.DB.GetContext(ctx, &sn, `SELECT * FROM snippets WHERE id = ?`, id)
	return sn, notFound(err)
}

func (s *Store) SaveSnippet(ctx context.Context, sn *Snippet) error {
	sn.UpdatedAt = Now()
	if sn.ID == 0 {
		res, err := s.DB.NamedExecContext(ctx, `
			INSERT INTO snippets (prefix, name, description, body, builtin, updated_at)
			VALUES (:prefix, :name, :description, :body, :builtin, :updated_at)`, sn)
		if err != nil {
			return fmt.Errorf("insert snippet: %w", err)
		}
		sn.ID, err = res.LastInsertId()
		return err
	}
	res, err := s.DB.NamedExecContext(ctx, `
		UPDATE snippets SET prefix = :prefix, name = :name, description = :description, body = :body,
			updated_at = :updated_at
		WHERE id = :id`, sn)
	if err != nil {
		return fmt.Errorf("update snippet: %w", err)
	}
	return affected(res)
}

func (s *Store) DeleteSnippet(ctx context.Context, id int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM snippets WHERE id = ?`, id)
	if err != nil {
		return err
	}
	return affected(res)
}

// UpsertBuiltinSnippet creates or refreshes a built-in snippet by prefix (user snippets win).
func (s *Store) UpsertBuiltinSnippet(ctx context.Context, sn Snippet) error {
	sn.UpdatedAt = Now()
	_, err := s.DB.NamedExecContext(ctx, `
		INSERT INTO snippets (prefix, name, description, body, builtin, updated_at)
		VALUES (:prefix, :name, :description, :body, 1, :updated_at)
		ON CONFLICT (prefix) DO UPDATE SET name = excluded.name, description = excluded.description,
			body = excluded.body, updated_at = excluded.updated_at
		WHERE snippets.builtin = 1`, sn)
	return err
}
