package main

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/ianfoxdev/webhook-outbox/relay/internal/endpoint"
	"github.com/ianfoxdev/webhook-outbox/relay/internal/replay"
)

const replayUsage = `usage: webhook-outbox-relay replay [-endpoint ID] MESSAGE...
       webhook-outbox-relay replay -endpoint ID -since TIME

  The first form sends the messages again to every endpoint that got them, or to one
  endpoint, also if it never got them. The second catches an endpoint up after an
  outage: failed deliveries of messages created since TIME are sent again, and the
  messages it missed while disabled are sent to it. TIME is RFC 3339
  (2026-11-20T09:00:00Z) or a duration before now (36h).
  Pending deliveries are left as they are.
`

func replayCmd(ctx context.Context, args []string, getenv func(string) string, stdout, stderr io.Writer) int {
	fs := flagSet("replay", stderr)
	fs.Usage = func() { fmt.Fprint(stderr, replayUsage) }
	endpointID := fs.String("endpoint", "", "only this endpoint")
	sinceText := fs.String("since", "", "messages created at or after this time")
	if fs.Parse(args) != nil {
		return 2
	}
	messages := fs.Args()
	var since time.Time
	switch {
	case *sinceText != "" && (len(messages) > 0 || *endpointID == ""):
		fmt.Fprintln(stderr, "replay: -since needs -endpoint and no message ids")
		return 2
	case *sinceText != "":
		var err error
		if since, err = parseSince(*sinceText, time.Now()); err != nil {
			fmt.Fprintln(stderr, "replay:", err)
			return 2
		}
	case len(messages) == 0:
		fmt.Fprint(stderr, replayUsage)
		return 2
	}
	return withDB(ctx, getenv, stderr, func(conn *pgx.Conn) error {
		var res replay.Result
		var err error
		if *sinceText != "" {
			res, err = replay.Since(ctx, conn, *endpointID, since)
		} else {
			res, err = replay.Messages(ctx, conn, messages, *endpointID)
		}
		if err != nil {
			return err
		}
		fmt.Fprintf(stdout, "requeued %d, created %d\n", res.Requeued, res.Created)
		if *endpointID != "" && res.Requeued+res.Created > 0 {
			if e, err := endpoint.Get(ctx, conn, *endpointID); err == nil && e.DisabledAt != nil {
				fmt.Fprintf(stdout, "%s is disabled: the deliveries go out after endpoints enable\n", e.ID)
			}
		}
		return nil
	})
}

func parseSince(v string, now time.Time) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, v); err == nil {
		return t, nil
	}
	if d, err := time.ParseDuration(v); err == nil && d > 0 {
		return now.Add(-d), nil
	}
	return time.Time{}, fmt.Errorf("-since %q is neither a time such as 2026-11-20T09:00:00Z nor a duration such as 36h", v)
}
