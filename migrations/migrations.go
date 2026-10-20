// Package migrations embeds the versioned SQL schema migrations.
//
// Files are named NNNN_description.sql and applied in order by
// internal/platform/migrate. An applied migration is never edited; a change
// is always a new file.
package migrations

import "embed"

// FS holds every migration file.
//
//go:embed *.sql
var FS embed.FS
