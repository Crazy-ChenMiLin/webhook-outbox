package metrics_test

import (
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/ianfoxdev/webhook-outbox/relay/internal/metrics"
	"github.com/ianfoxdev/webhook-outbox/relay/internal/pgtest"
	"github.com/ianfoxdev/webhook-outbox/relay/internal/schema"
)

func TestBacklog(t *testing.T) {
	ctx := context.Background()
	conn := pgtest.Connect(t)
	if _, err := schema.Migrate(ctx, conn); err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.New(ctx, conn.Config().ConnString())
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	m := metrics.New(pool, "test", slog.New(slog.DiscardHandler))

	empty := `
# HELP webhook_outbox_delivery_lag_seconds How long the most overdue delivery to an enabled endpoint has been due, 0 when none is.
# TYPE webhook_outbox_delivery_lag_seconds gauge
webhook_outbox_delivery_lag_seconds 0
# HELP webhook_outbox_pending_deliveries Pending deliveries: due (should go out now), waiting (for the next retry, or in flight) and endpoint_disabled.
# TYPE webhook_outbox_pending_deliveries gauge
webhook_outbox_pending_deliveries{state="due"} 0
webhook_outbox_pending_deliveries{state="endpoint_disabled"} 0
webhook_outbox_pending_deliveries{state="waiting"} 0
`
	if err := testutil.GatherAndCompare(m.Registry, strings.NewReader(empty),
		"webhook_outbox_delivery_lag_seconds", "webhook_outbox_pending_deliveries"); err != nil {
		t.Fatal(err)
	}

	if _, err := conn.Exec(ctx, `
		INSERT INTO webhook_endpoints (id, customer, url, secret) VALUES ('ep_on', 'acme', 'https://a', 'whsec_x');
		INSERT INTO webhook_endpoints (id, customer, url, secret, disabled_at) VALUES ('ep_off', 'acme', 'https://b', 'whsec_x', now());
		INSERT INTO webhook_messages (id, customer, type, payload, dispatched_at) VALUES
			('m1', 'acme', 't', '{}', now()), ('m2', 'acme', 't', '{}', now()), ('m3', 'acme', 't', '{}', now()),
			('m4', 'acme', 't', '{}', now()), ('m5', 'acme', 't', '{}', now());
		INSERT INTO webhook_messages (id, customer, type, payload, created_at) VALUES ('m6', 'acme', 't', '{}', now() - interval '90 seconds');
		INSERT INTO webhook_deliveries (message_id, endpoint_id, status, next_attempt_at) VALUES
			('m1', 'ep_on', 'pending', now() - interval '2 minutes'),
			('m2', 'ep_on', 'pending', now() - interval '1 minute'),
			('m3', 'ep_on', 'pending', now() + interval '1 hour'),
			('m4', 'ep_off', 'pending', now() - interval '1 day'),
			('m5', 'ep_on', 'succeeded', now() - interval '1 day')`); err != nil {
		t.Fatal(err)
	}
	gauges := map[string]float64{}
	families, err := m.Registry.Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range families {
		for _, metric := range f.GetMetric() {
			name := f.GetName()
			for _, l := range metric.GetLabel() {
				name += "/" + l.GetValue()
			}
			if g := metric.GetGauge(); g != nil {
				gauges[name] = g.GetValue()
			}
		}
	}
	for name, want := range map[string]float64{
		"webhook_outbox_pending_deliveries/due":               2,
		"webhook_outbox_pending_deliveries/waiting":           1,
		"webhook_outbox_pending_deliveries/endpoint_disabled": 1,
		"webhook_outbox_undispatched_messages":                1,
		"webhook_outbox_disabled_endpoints":                   1,
		"webhook_outbox_backlog_up":                           1,
	} {
		if gauges[name] != want {
			t.Errorf("%s = %v, want %v", name, gauges[name], want)
		}
	}
	// The disabled endpoint's day-old delivery does not count: nobody can send it.
	if lag := gauges["webhook_outbox_delivery_lag_seconds"]; lag < 120 || lag > 130 {
		t.Errorf("delivery lag %v, want about 120", lag)
	}
	if lag := gauges["webhook_outbox_dispatch_lag_seconds"]; lag < 90 || lag > 100 {
		t.Errorf("dispatch lag %v, want about 90", lag)
	}
}

func TestAttempt(t *testing.T) {
	// Nothing listens on port 1: the backlog query fails and the counters still work.
	pool, err := pgxpool.New(context.Background(), "postgres://u@127.0.0.1:1/db?connect_timeout=1")
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	m := metrics.New(pool, "test", slog.New(slog.DiscardHandler))
	if err := testutil.GatherAndCompare(m.Registry, strings.NewReader(`
# HELP webhook_outbox_backlog_up 1 if the last backlog query succeeded.
# TYPE webhook_outbox_backlog_up gauge
webhook_outbox_backlog_up 0
`), "webhook_outbox_backlog_up"); err != nil {
		t.Fatal(err)
	}
	m.Attempt("succeeded", 204, 30*time.Millisecond)
	m.Attempt("retrying", 503, time.Second)
	m.Attempt("retrying", 0, 15*time.Second)
	m.Attempt("failed", 410, time.Second)
	m.EndpointDisabled("gone")
	want := `
# HELP webhook_outbox_attempts_total Recorded attempts by result (succeeded, retrying, failed) and status code class (2xx..5xx, none).
# TYPE webhook_outbox_attempts_total counter
webhook_outbox_attempts_total{code="2xx",result="succeeded"} 1
webhook_outbox_attempts_total{code="3xx",result="failed"} 0
webhook_outbox_attempts_total{code="3xx",result="retrying"} 0
webhook_outbox_attempts_total{code="4xx",result="failed"} 1
webhook_outbox_attempts_total{code="4xx",result="retrying"} 0
webhook_outbox_attempts_total{code="5xx",result="failed"} 0
webhook_outbox_attempts_total{code="5xx",result="retrying"} 1
webhook_outbox_attempts_total{code="none",result="failed"} 0
webhook_outbox_attempts_total{code="none",result="retrying"} 1
# HELP webhook_outbox_endpoints_disabled_total Endpoints this relay disabled, by reason: gone (410) or failing.
# TYPE webhook_outbox_endpoints_disabled_total counter
webhook_outbox_endpoints_disabled_total{reason="failing"} 0
webhook_outbox_endpoints_disabled_total{reason="gone"} 1
`
	if err := testutil.GatherAndCompare(m.Registry, strings.NewReader(want),
		"webhook_outbox_attempts_total", "webhook_outbox_endpoints_disabled_total"); err != nil {
		t.Fatal(err)
	}
}
