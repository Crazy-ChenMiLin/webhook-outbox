// Package schema creates and upgrades the webhook_ tables.
package schema

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
)

//go:embed migrations/*.sql
var files embed.FS

// lockKey is the advisory lock that keeps two migrate runs from applying the same file.
const lockKey = 7_301_115_011

// Migration is one numbered SQL file.
type Migration struct {
	Version int
	Name    string
	SQL     string
}

// Migrations returns the embedded migrations in order.
func Migrations() ([]Migration, error) {
	names, err := fs.Glob(files, "migrations/*.sql")
	if err != nil {
		return nil, err
	}
	out := make([]Migration, 0, len(names))
	for _, name := range names {
		base := strings.TrimPrefix(name, "migrations/")
		number, _, ok := strings.Cut(base, "_")
		version, err := strconv.Atoi(number)
		if !ok || err != nil {
			return nil, fmt.Errorf("migration %s: name must start with a number and _", base)
		}
		body, err := files.ReadFile(name)
		if err != nil {
			return nil, err
		}
		out = append(out, Migration{Version: version, Name: base, SQL: string(body)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Version < out[j].Version })
	return out, nil
}

// Migrate applies the migrations the database does not have yet, each in its own
// transaction, and returns the names of the ones it applied.
func Migrate(ctx context.Context, conn *pgx.Conn) ([]string, error) {
	migrations, err := Migrations()
	if err != nil {
		return nil, err
	}
	if _, err := conn.Exec(ctx, `CREATE TABLE IF NOT EXISTS webhook_schema_migrations (
		version    integer PRIMARY KEY,
		name       text NOT NULL,
		applied_at timestamptz NOT NULL DEFAULT now()
	)`); err != nil {
		return nil, fmt.Errorf("create webhook_schema_migrations: %w", err)
	}
	var applied []string
	for _, m := range migrations {
		done, err := apply(ctx, conn, m)
		if err != nil {
			return applied, fmt.Errorf("migration %s: %w", m.Name, err)
		}
		if done {
			applied = append(applied, m.Name)
		}
	}
	return applied, nil
}

func apply(ctx context.Context, conn *pgx.Conn, m Migration) (bool, error) {
	tx, err := conn.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, int64(lockKey)); err != nil {
		return false, err
	}
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM webhook_schema_migrations WHERE version = $1)`, m.Version).Scan(&exists); err != nil {
		return false, err
	}
	if exists {
		return false, nil
	}
	if _, err := tx.Exec(ctx, m.SQL); err != nil {
		return false, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO webhook_schema_migrations (version, name) VALUES ($1, $2)`, m.Version, m.Name); err != nil {
		return false, err
	}
	return true, tx.Commit(ctx)
}
