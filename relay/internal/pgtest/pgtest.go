// Package pgtest gives each test a PostgreSQL schema of its own.
package pgtest

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

// EnvURL names the variable with the test database. Tests that need it are skipped
// without it; make test and CI set it.
const EnvURL = "WEBHOOK_OUTBOX_TEST_DATABASE_URL"

// URL returns a database URL whose connections use a fresh schema, dropped when the
// test ends.
func URL(t *testing.T) string {
	t.Helper()
	base := os.Getenv(EnvURL)
	if base == "" {
		t.Skip(EnvURL + " is not set")
	}
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, base)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer func() { _ = conn.Close(ctx) }()
	name := "t_" + strings.ToLower(strings.NewReplacer("/", "_", " ", "_", "-", "_").Replace(t.Name()))
	if len(name) > 63 {
		name = name[:63]
	}
	ident := pgx.Identifier{name}.Sanitize()
	if _, err := conn.Exec(ctx, "DROP SCHEMA IF EXISTS "+ident+" CASCADE; CREATE SCHEMA "+ident); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	t.Cleanup(func() {
		c, err := pgx.Connect(context.Background(), base)
		if err != nil {
			return
		}
		_, _ = c.Exec(context.Background(), "DROP SCHEMA IF EXISTS "+ident+" CASCADE")
		_ = c.Close(context.Background())
	})
	sep := "?"
	if strings.Contains(base, "?") {
		sep = "&"
	}
	return fmt.Sprintf("%s%ssearch_path=%s", base, sep, name)
}

// Connect opens a connection to the test's schema.
func Connect(t *testing.T) *pgx.Conn {
	t.Helper()
	conn, err := pgx.Connect(context.Background(), URL(t))
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close(context.Background()) })
	return conn
}
