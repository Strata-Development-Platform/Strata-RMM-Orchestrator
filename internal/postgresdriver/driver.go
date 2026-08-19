// Package postgresdriver registers pgx's database/sql adapter under the
// historical "postgres" driver name used throughout Strata RMM. Keeping
// the name stable lets the application migrate away from lib/pq without
// changing connection construction at every call site.
package postgresdriver

import (
	"database/sql"

	"github.com/jackc/pgx/v5/stdlib"
)

func init() {
	sql.Register("postgres", stdlib.GetDefaultDriver())
}
