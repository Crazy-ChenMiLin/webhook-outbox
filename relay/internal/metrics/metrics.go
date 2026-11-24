// Package metrics exposes the relay to Prometheus.
//
// Two kinds of metrics live here. Counters and the attempt histogram count what this
// process did, so sum them over replicas. The backlog gauges are read from the
// database on every scrape and are the same on every replica, so take max, not sum.
package metrics

import (
	"context"
	"log/slog"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
)

// Metrics owns a registry with every relay metric. It implements relay.Metrics.
type Metrics struct {
	Registry *prometheus.Registry

	dispatched prometheus.Counter
	attempts   *prometheus.CounterVec
	duration   prometheus.Histogram
	disabled   *prometheus.CounterVec
}

// New registers the metrics. pool is queried for the backlog on every scrape.
func New(pool *pgxpool.Pool, version string, logger *slog.Logger) *Metrics {
	m := &Metrics{
		Registry: prometheus.NewRegistry(),
		dispatched: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "webhook_outbox_dispatched_messages_total",
			Help: "Messages turned into deliveries by this relay.",
		}),
		attempts: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "webhook_outbox_attempts_total",
			Help: "Recorded attempts by result (succeeded, retrying, failed) and status code class (2xx..5xx, none).",
		}, []string{"result", "code"}),
		// From connecting to reading the response, so a timeout lands at RELAY_TIMEOUT.
		duration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "webhook_outbox_attempt_duration_seconds",
			Help:    "Duration of attempts, with or without a response.",
			Buckets: []float64{.025, .05, .1, .25, .5, 1, 2.5, 5, 10, 15, 30},
		}),
		disabled: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "webhook_outbox_endpoints_disabled_total",
			Help: "Endpoints this relay disabled, by reason: gone (410) or failing.",
		}, []string{"reason"}),
	}
	for _, result := range []string{"succeeded", "retrying", "failed"} {
		for _, code := range []string{"2xx", "3xx", "4xx", "5xx", "none"} {
			if (result == "succeeded") == (code == "2xx") {
				m.attempts.WithLabelValues(result, code)
			}
		}
	}
	m.disabled.WithLabelValues("gone")
	m.disabled.WithLabelValues("failing")
	info := prometheus.NewGauge(prometheus.GaugeOpts{
		Name:        "webhook_outbox_relay_info",
		Help:        "Build information.",
		ConstLabels: prometheus.Labels{"version": version},
	})
	info.Set(1)
	m.Registry.MustRegister(
		m.dispatched, m.attempts, m.duration, m.disabled, info,
		&backlog{pool: pool, logger: logger},
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)
	return m
}

// Dispatched implements relay.Metrics.
func (m *Metrics) Dispatched(messages int) { m.dispatched.Add(float64(messages)) }

// Attempt implements relay.Metrics.
func (m *Metrics) Attempt(result string, status int, d time.Duration) {
	code := "none"
	if status >= 100 && status <= 599 {
		code = strconv.Itoa(status/100) + "xx"
	}
	m.attempts.WithLabelValues(result, code).Inc()
	m.duration.Observe(d.Seconds())
}

// EndpointDisabled implements relay.Metrics.
func (m *Metrics) EndpointDisabled(reason string) { m.disabled.WithLabelValues(reason).Inc() }

var (
	undispatchedDesc = prometheus.NewDesc("webhook_outbox_undispatched_messages",
		"Messages the relay has not turned into deliveries yet.", nil, nil)
	dispatchLagDesc = prometheus.NewDesc("webhook_outbox_dispatch_lag_seconds",
		"Age of the oldest undispatched message, 0 when there is none.", nil, nil)
	pendingDesc = prometheus.NewDesc("webhook_outbox_pending_deliveries",
		"Pending deliveries: due (should go out now), waiting (for the next retry, or in flight) and endpoint_disabled.",
		[]string{"state"}, nil)
	deliveryLagDesc = prometheus.NewDesc("webhook_outbox_delivery_lag_seconds",
		"How long the most overdue delivery to an enabled endpoint has been due, 0 when none is.", nil, nil)
	disabledDesc = prometheus.NewDesc("webhook_outbox_disabled_endpoints",
		"Endpoints that are disabled.", nil, nil)
	upDesc = prometheus.NewDesc("webhook_outbox_backlog_up",
		"1 if the last backlog query succeeded.", nil, nil)
)

// backlog reads the queue from the database. Both lags stay near 0 while the relays
// keep up; a growing lag means too few relays, or none running.
type backlog struct {
	pool   *pgxpool.Pool
	logger *slog.Logger
}

func (c *backlog) Describe(ch chan<- *prometheus.Desc) {
	for _, d := range []*prometheus.Desc{undispatchedDesc, dispatchLagDesc, pendingDesc, deliveryLagDesc, disabledDesc, upDesc} {
		ch <- d
	}
}

func (c *backlog) Collect(ch chan<- prometheus.Metric) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	var undispatched, due, waiting, onDisabled, disabled int64
	var dispatchLag, deliveryLag float64
	err := c.pool.QueryRow(ctx, `
		SELECT
			(SELECT count(*) FROM webhook_messages WHERE dispatched_at IS NULL),
			(SELECT COALESCE(EXTRACT(EPOCH FROM now() - min(created_at)), 0)::float8
				FROM webhook_messages WHERE dispatched_at IS NULL),
			count(*) FILTER (WHERE e.disabled_at IS NULL AND d.next_attempt_at <= now()),
			count(*) FILTER (WHERE e.disabled_at IS NULL AND d.next_attempt_at > now()),
			count(*) FILTER (WHERE e.disabled_at IS NOT NULL),
			COALESCE(EXTRACT(EPOCH FROM now() - min(d.next_attempt_at)
				FILTER (WHERE e.disabled_at IS NULL AND d.next_attempt_at <= now())), 0)::float8,
			(SELECT count(*) FROM webhook_endpoints WHERE disabled_at IS NOT NULL)
		FROM webhook_deliveries d
		JOIN webhook_endpoints e ON e.id = d.endpoint_id
		WHERE d.status = 'pending'`).Scan(&undispatched, &dispatchLag, &due, &waiting, &onDisabled, &deliveryLag, &disabled)
	if err != nil {
		c.logger.Warn("backlog query failed", "error", err)
		ch <- prometheus.MustNewConstMetric(upDesc, prometheus.GaugeValue, 0)
		return
	}
	gauge := func(d *prometheus.Desc, v float64, labels ...string) {
		ch <- prometheus.MustNewConstMetric(d, prometheus.GaugeValue, v, labels...)
	}
	gauge(upDesc, 1)
	gauge(undispatchedDesc, float64(undispatched))
	gauge(dispatchLagDesc, max(dispatchLag, 0))
	gauge(pendingDesc, float64(due), "due")
	gauge(pendingDesc, float64(waiting), "waiting")
	gauge(pendingDesc, float64(onDisabled), "endpoint_disabled")
	gauge(deliveryLagDesc, max(deliveryLag, 0))
	gauge(disabledDesc, float64(disabled))
}
