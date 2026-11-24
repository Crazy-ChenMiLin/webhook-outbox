package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/ianfoxdev/webhook-outbox/relay/internal/pgtest"
)

func TestReplay(t *testing.T) {
	env := map[string]string{"DATABASE_URL": pgtest.URL(t)}
	if code, _, errOut := cli(t, env, "migrate"); code != 0 {
		t.Fatal(errOut)
	}
	_, out, _ := cli(t, env, "endpoints", "add", "-customer", "acme", "-url", "https://acme.example/hooks")
	id := strings.TrimPrefix(strings.Split(out, "\n")[0], "id ")
	conn, err := pgx.Connect(context.Background(), env["DATABASE_URL"])
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close(context.Background()) }()
	if _, err := conn.Exec(context.Background(), `
		INSERT INTO webhook_messages (id, customer, type, payload, dispatched_at) VALUES ('msg_1', 'acme', 'order.paid', '{}', now());
		INSERT INTO webhook_deliveries (message_id, endpoint_id, status, attempts) VALUES ('msg_1', '`+id+`', 'failed', 8)`); err != nil {
		t.Fatal(err)
	}

	if code, out, errOut := cli(t, env, "replay", "msg_1"); code != 0 || out != "requeued 1, created 0\n" {
		t.Fatalf("exit %d, %q, %q", code, out, errOut)
	}
	if code, _, _ := cli(t, env, "endpoints", "disable", id); code != 0 {
		t.Fatal("disable")
	}
	if _, err := conn.Exec(context.Background(), `UPDATE webhook_deliveries SET status = 'failed'`); err != nil {
		t.Fatal(err)
	}
	code, out, errOut := cli(t, env, "replay", "-endpoint", id, "-since", "1h")
	if code != 0 || out != "requeued 1, created 0\n"+id+" is disabled: the deliveries go out after endpoints enable\n" {
		t.Fatalf("exit %d, %q, %q", code, out, errOut)
	}
	if code, _, errOut := cli(t, env, "replay", "msg_nope"); code != 1 || !strings.Contains(errOut, "message not found: msg_nope") {
		t.Fatalf("exit %d, %q", code, errOut)
	}
}

func TestReplayUsage(t *testing.T) {
	for _, args := range [][]string{
		{"replay"},
		{"replay", "-since", "1h"},
		{"replay", "-endpoint", "ep_1", "-since", "1h", "msg_1"},
		{"replay", "-endpoint", "ep_1", "-since", "yesterday"},
	} {
		if code, _, errOut := cli(t, map[string]string{"DATABASE_URL": "postgres://unused"}, args...); code != 2 || errOut == "" {
			t.Errorf("%v: exit %d", args, code)
		}
	}
}

func TestParseSince(t *testing.T) {
	now := time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)
	if got, _ := parseSince("36h", now); !got.Equal(now.Add(-36 * time.Hour)) {
		t.Fatal(got)
	}
	if got, _ := parseSince("2026-11-20T09:00:00Z", now); !got.Equal(time.Date(2026, 11, 20, 9, 0, 0, 0, time.UTC)) {
		t.Fatal(got)
	}
}
