package store

import (
	"context"
	"fmt"
)

// Asset is an uploaded image kept in its original form; processed variants are cached on disk.
type Asset struct {
	ID        int64  `db:"id"`
	Name      string `db:"name"`
	Filename  string `db:"filename"`
	Mime      string `db:"mime"`
	Width     int    `db:"width"`
	Height    int    `db:"height"`
	Size      int64  `db:"size"`
	CreatedAt Time   `db:"created_at"`
}

func (s *Store) Assets(ctx context.Context) ([]Asset, error) {
	var out []Asset
	err := s.DB.SelectContext(ctx, &out, `SELECT * FROM assets ORDER BY id DESC`)
	return out, err
}

func (s *Store) Asset(ctx context.Context, id int64) (Asset, error) {
	var a Asset
	err := s.DB.GetContext(ctx, &a, `SELECT * FROM assets WHERE id = ?`, id)
	return a, notFound(err)
}

func (s *Store) AssetByFilename(ctx context.Context, filename string) (Asset, error) {
	var a Asset
	err := s.DB.GetContext(ctx, &a, `SELECT * FROM assets WHERE filename = ?`, filename)
	return a, notFound(err)
}

func (s *Store) CreateAsset(ctx context.Context, a *Asset) error {
	a.CreatedAt = Now()
	res, err := s.DB.NamedExecContext(ctx, `
		INSERT INTO assets (name, filename, mime, width, height, size, created_at)
		VALUES (:name, :filename, :mime, :width, :height, :size, :created_at)`, a)
	if err != nil {
		return fmt.Errorf("insert asset: %w", err)
	}
	a.ID, err = res.LastInsertId()
	return err
}

func (s *Store) DeleteAsset(ctx context.Context, id int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM assets WHERE id = ?`, id)
	if err != nil {
		return err
	}
	return affected(res)
}

// ScreensUsingAsset returns the names of screens whose background references the asset.
func (s *Store) ScreensUsingAsset(ctx context.Context, id int64) ([]string, error) {
	var names []string
	err := s.DB.SelectContext(ctx, &names, `
		SELECT DISTINCT sc.name FROM screens sc, json_each(sc.background, '$.asset_ids') ids
		WHERE ids.value = ? ORDER BY sc.name`, id)
	return names, err
}
