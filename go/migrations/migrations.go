package migrations

import "embed"

// Files contains the SQL migrations applied by worker startup.
//
// The migrations are embedded so containerized workers do not depend on a
// writable or mounted source tree at runtime.
//
//go:embed *.sql
var Files embed.FS
