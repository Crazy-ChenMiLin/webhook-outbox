package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ianfoxdev/webhook-outbox/relay/internal/relay"
)

// config reads the relay settings from the environment. Unset variables keep the
// defaults of relay.Config.
func config(getenv func(string) string) (relay.Config, error) {
	var c relay.Config
	var err error
	duration := func(name string, dst *time.Duration) {
		if v := getenv(name); v != "" && err == nil {
			d, e := time.ParseDuration(v)
			if e != nil || d <= 0 {
				err = fmt.Errorf("%s: %q is not a positive duration such as 500ms or 2s", name, v)
				return
			}
			*dst = d
		}
	}
	number := func(name string, dst *int) {
		if v := getenv(name); v != "" && err == nil {
			n, e := strconv.Atoi(v)
			if e != nil || n <= 0 {
				err = fmt.Errorf("%s: %q is not a positive number", name, v)
				return
			}
			*dst = n
		}
	}
	duration("RELAY_POLL_INTERVAL", &c.PollInterval)
	duration("RELAY_TIMEOUT", &c.Timeout)
	number("RELAY_BATCH_SIZE", &c.BatchSize)
	number("RELAY_CONCURRENCY", &c.Concurrency)
	number("RELAY_RESPONSE_LIMIT", &c.ResponseLimit)
	switch v := getenv("RELAY_ALLOW_PRIVATE_NETWORKS"); v {
	case "", "false":
	case "true":
		c.AllowPrivateNetworks = true
	default:
		return c, fmt.Errorf("RELAY_ALLOW_PRIVATE_NETWORKS: %q is not true or false", v)
	}
	if v := getenv("RELAY_RETRY_SCHEDULE"); v != "" && err == nil {
		c.Schedule = []time.Duration{}
		if v != "none" {
			for _, part := range strings.Split(v, ",") {
				d, e := time.ParseDuration(strings.TrimSpace(part))
				if e != nil || d < 0 {
					return c, fmt.Errorf("RELAY_RETRY_SCHEDULE: %q is not a duration", part)
				}
				c.Schedule = append(c.Schedule, d)
			}
		}
	}
	return c, err
}

func runRelay(ctx context.Context, getenv func(string) string, stderr io.Writer) int {
	cfg, err := config(getenv)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	url := getenv("DATABASE_URL")
	if url == "" {
		fmt.Fprintln(stderr, "DATABASE_URL is not set")
		return 2
	}
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		fmt.Fprintln(stderr, "connect:", err)
		return 1
	}
	defer pool.Close()
	cfg.Logger = slog.New(slog.NewJSONHandler(stderr, nil))
	cfg.UserAgent = "webhook-outbox/" + version
	cfg = cfg.Defaults()
	cfg.Logger.Info("relay started", "version", version, "poll_interval", cfg.PollInterval.String(),
		"batch_size", cfg.BatchSize, "concurrency", cfg.Concurrency, "timeout", cfg.Timeout.String(), "attempts", len(cfg.Schedule)+1,
		"allow_private_networks", cfg.AllowPrivateNetworks)
	if err := relay.New(pool, cfg).Run(ctx); err != nil {
		cfg.Logger.Error("relay stopped", "error", err)
		return 1
	}
	cfg.Logger.Info("relay stopped")
	return 0
}
