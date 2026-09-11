package migrations

import "embed"

// FS contains ordered PostgreSQL migrations.
//
//go:embed *.sql
var FS embed.FS
