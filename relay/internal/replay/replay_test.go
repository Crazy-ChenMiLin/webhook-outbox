package replay_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/ianfoxdev/webhook-outbox/relay/internal/endpoint"
	"github.com/ianfoxdev/webhook-outbox/relay/internal/pgtest"
	"github.com/ianfoxdev/webhook-outbox/relay/internal/replay"
	"github.com/ianfoxdev/webhook-outbox/relay/internal/schema"
)

type db struct {
	t    *testing.T
	ctx  context.Context
	conn *pgx.Conn
}

func setup(t *testing.T) *db {
	t.Helper()
	conn := pgtest.Connect(t)
	ctx := context.Background()
	if _, err := schema.Migrate(ctx, conn); err != nil {
		t.Fatal(err)
	}
	return &db{t, ctx, conn}
}

func (d *db) exec(sql string, args ...any) {
	d.t.Helper()
	if _, err := d.conn.Exec(d.ctx, sql, args...); err != nil {
		d.t.Fatal(err)
	}
}

func (d *db) endpoint(customer string, types ...string) string {
	d.t.Helper()
	e, err := endpoint.Add(d.ctx, d.conn, endpoint.New{Customer: customer, URL: "https://hooks.example", EventTypes: types})
	if err != nil {
		d.t.Fatal(err)
	}
	return e.ID
}

// message inserts a dispatched message created age ago.
func (d *db) message(id, customer, typ string, age time.Duration) {
	d.t.Helper()
	d.exec(`INSERT INTO webhook_messages (id, customer, type, payload, created_at, dispatched_at)
		VALUES ($1, $2, $3, '{}', now() - $4 * interval '1 microsecond', now())`, id, customer, typ, age.Microseconds())
}

func (d *db) delivery(message, ep, status string, attempts int) {
	d.t.Helper()
	d.exec(`INSERT INTO webhook_deliveries (message_id, endpoint_id, status, attempts, next_attempt_at)
		VALUES ($1, $2, $3, $4, now() + interval '1 hour')`, message, ep, status, attempts)
}

// state returns status/attempts and whether the delivery is due now, or "none".
func (d *db) state(message, ep string) string {
	d.t.Helper()
	var status string
	var attempts int
	var due bool
	err := d.conn.QueryRow(d.ctx, `SELECT status, attempts, next_attempt_at <= now() FROM webhook_deliveries
		WHERE message_id = $1 AND endpoint_id = $2`, message, ep).Scan(&status, &attempts, &due)
	if errors.Is(err, pgx.ErrNoRows) {
		return "none"
	}
	if err != nil {
		d.t.Fatal(err)
	}
	s := status + "/" + string(rune('0'+attempts))
	if due {
		s += " due"
	}
	return s
}

func TestMessagesRequeuesFinishedDeliveries(t *testing.T) {
	d := setup(t)
	a, b, c := d.endpoint("acme"), d.endpoint("acme"), d.endpoint("acme")
	d.message("msg_1", "acme", "order.paid", 0)
	d.delivery("msg_1", a, "succeeded", 1)
	d.delivery("msg_1", b, "failed", 8)
	d.delivery("msg_1", c, "pending", 3)

	res, err := replay.Messages(d.ctx, d.conn, []string{"msg_1"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if res != (replay.Result{Requeued: 2}) {
		t.Fatalf("%+v", res)
	}
	for ep, want := range map[string]string{a: "pending/0 due", b: "pending/0 due", c: "pending/3"} {
		if got := d.state("msg_1", ep); got != want {
			t.Errorf("%s: %s, want %s", ep, got, want)
		}
	}
}

func TestMessagesToOneEndpoint(t *testing.T) {
	d := setup(t)
	got, missed, other := d.endpoint("acme"), d.endpoint("acme"), d.endpoint("acme")
	paidOnly := d.endpoint("acme", "order.paid")
	stranger := d.endpoint("beta")
	d.message("msg_1", "acme", "order.refunded", 0)
	d.delivery("msg_1", got, "failed", 8)
	d.delivery("msg_1", other, "failed", 8)

	res, err := replay.Messages(d.ctx, d.conn, []string{"msg_1"}, got)
	if err != nil || res != (replay.Result{Requeued: 1}) {
		t.Fatalf("%+v, %v", res, err)
	}
	if s := d.state("msg_1", other); s != "failed/8" {
		t.Fatalf("other endpoint touched: %s", s)
	}
	res, err = replay.Messages(d.ctx, d.conn, []string{"msg_1"}, missed)
	if err != nil || res != (replay.Result{Created: 1}) || d.state("msg_1", missed) != "pending/0 due" {
		t.Fatalf("%+v, %v", res, err)
	}
	// Another customer's endpoint, or one that does not take the type, gets nothing.
	for _, ep := range []string{stranger, paidOnly} {
		res, err = replay.Messages(d.ctx, d.conn, []string{"msg_1"}, ep)
		if err != nil || res != (replay.Result{}) || d.state("msg_1", ep) != "none" {
			t.Fatalf("%s: %+v, %v", ep, res, err)
		}
	}
}

func TestMessagesNotFound(t *testing.T) {
	d := setup(t)
	ep := d.endpoint("acme")
	d.message("msg_1", "acme", "order.paid", 0)
	d.delivery("msg_1", ep, "failed", 8)

	_, err := replay.Messages(d.ctx, d.conn, []string{"msg_1", "msg_2"}, "")
	if !errors.Is(err, replay.ErrMessageNotFound) || err.Error() != "replay: message not found: msg_2" {
		t.Fatalf("%v", err)
	}
	if d.state("msg_1", ep) != "failed/8" {
		t.Fatal("a failed replay changed a delivery")
	}
	if _, err := replay.Messages(d.ctx, d.conn, []string{"msg_1"}, "ep_missing"); !errors.Is(err, endpoint.ErrNotFound) {
		t.Fatalf("%v", err)
	}
}

func TestSinceCatchesAnEndpointUp(t *testing.T) {
	d := setup(t)
	ep := d.endpoint("acme", "order.paid")
	d.message("msg_old", "acme", "order.paid", 48*time.Hour)
	d.message("msg_ok", "acme", "order.paid", 3*time.Hour)
	d.message("msg_failed", "acme", "order.paid", 2*time.Hour)
	d.message("msg_waiting", "acme", "order.paid", 2*time.Hour)
	d.message("msg_missed", "acme", "order.paid", time.Hour)
	d.message("msg_type", "acme", "order.refunded", time.Hour)
	d.message("msg_beta", "beta", "order.paid", time.Hour)
	d.exec(`INSERT INTO webhook_messages (id, customer, type, payload) VALUES ('msg_new', 'acme', 'order.paid', '{}')`)
	d.delivery("msg_old", ep, "failed", 8)
	d.delivery("msg_ok", ep, "succeeded", 1)
	d.delivery("msg_failed", ep, "failed", 8)
	d.delivery("msg_waiting", ep, "pending", 2)

	res, err := replay.Since(d.ctx, d.conn, ep, time.Now().Add(-24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if res != (replay.Result{Requeued: 1, Created: 1}) {
		t.Fatalf("%+v", res)
	}
	for msg, want := range map[string]string{
		"msg_old":     "failed/8",
		"msg_ok":      "succeeded/1",
		"msg_failed":  "pending/0 due",
		"msg_waiting": "pending/2",
		"msg_missed":  "pending/0 due",
		"msg_type":    "none",
		"msg_beta":    "none",
		"msg_new":     "none", // not dispatched yet: dispatch will take care of it
	} {
		if got := d.state(msg, ep); got != want {
			t.Errorf("%s: %s, want %s", msg, got, want)
		}
	}
	if _, err := replay.Since(d.ctx, d.conn, "ep_missing", time.Now()); !errors.Is(err, endpoint.ErrNotFound) {
		t.Fatalf("%v", err)
	}
}
