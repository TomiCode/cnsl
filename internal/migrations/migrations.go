// Package migrations embeds the SQL migrations applied on startup by golang-migrate.
//
// Files are named NNNN_description.up.sql / NNNN_description.down.sql.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
