// Command webhook-outbox-relay delivers the webhooks an application queued in its
// database.
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/jackc/pgx/v5"

	"github.com/ianfoxdev/webhook-outbox/relay/internal/schema"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

const usage = `usage: webhook-outbox-relay <command>

  migrate    create or upgrade the webhook_ tables
  version    print the version

The database is taken from DATABASE_URL (postgres://user:password@host/db).
`

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := run(ctx, os.Args[1:], os.Getenv, os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}

func run(ctx context.Context, args []string, getenv func(string) string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}
	switch args[0] {
	case "version":
		fmt.Fprintln(stdout, version)
		return 0
	case "migrate":
		return withDB(ctx, getenv, stderr, func(conn *pgx.Conn) error {
			applied, err := schema.Migrate(ctx, conn)
			for _, name := range applied {
				fmt.Fprintln(stdout, "applied", name)
			}
			if err == nil && len(applied) == 0 {
				fmt.Fprintln(stdout, "up to date")
			}
			return err
		})
	default:
		fmt.Fprint(stderr, usage)
		return 2
	}
}

// withDB connects to DATABASE_URL, runs fn and reports its error. Exit code 1 means the
// command failed.
func withDB(ctx context.Context, getenv func(string) string, stderr io.Writer, fn func(*pgx.Conn) error) int {
	url := getenv("DATABASE_URL")
	if url == "" {
		fmt.Fprintln(stderr, "DATABASE_URL is not set")
		return 2
	}
	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		fmt.Fprintln(stderr, "connect:", err)
		return 1
	}
	defer func() { _ = conn.Close(context.Background()) }()
	if err := fn(conn); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}
