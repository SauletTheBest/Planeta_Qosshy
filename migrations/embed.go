// Package migrations holds the versioned SQL schema changes. They are embedded into
// the binary, so the server applies them at startup without shipping .sql files.
//
// Rules: never edit a migration that has already run anywhere — add a new file instead.
// File name: <6-digit version>_<what_it_does>.up.sql (and optionally .down.sql).
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
