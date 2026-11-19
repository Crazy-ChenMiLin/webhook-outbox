package main

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ianfoxdev/webhook-outbox/relay/internal/pgtest"
)

func TestConfig(t *testing.T) {
	c, err := config(func(k string) string {
		return map[string]string{
			"RELAY_POLL_INTERVAL":          "250ms",
			"RELAY_BATCH_SIZE":             "50",
			"RELAY_CONCURRENCY":            "4",
			"RELAY_TIMEOUT":                "5s",
			"RELAY_RESPONSE_LIMIT":         "512",
			"RELAY_RETRY_SCHEDULE":         "1s, 1m",
			"RELAY_ALLOW_PRIVATE_NETWORKS": "true",
		}[k]
	})
	if err != nil {
		t.Fatal(err)
	}
	if c.PollInterval != 250*time.Millisecond || c.BatchSize != 50 || c.Concurrency != 4 || c.Timeout != 5*time.Second ||
		c.ResponseLimit != 512 || !c.AllowPrivateNetworks || !reflect.DeepEqual(c.Schedule, []time.Duration{time.Second, time.Minute}) {
		t.Fatalf("%+v", c)
	}
	none, err := config(func(k string) string {
		if k == "RELAY_RETRY_SCHEDULE" {
			return "none"
		}
		return ""
	})
	if err != nil || none.Schedule == nil || len(none.Schedule) != 0 {
		t.Fatalf("%+v, %v", none.Schedule, err)
	}
}

func TestConfigRefuses(t *testing.T) {
	for name, value := range map[string]string{
		"RELAY_POLL_INTERVAL":          "1",
		"RELAY_TIMEOUT":                "-1s",
		"RELAY_BATCH_SIZE":             "0",
		"RELAY_CONCURRENCY":            "many",
		"RELAY_RETRY_SCHEDULE":         "1s,soon",
		"RELAY_ALLOW_PRIVATE_NETWORKS": "yes",
	} {
		_, err := config(func(k string) string {
			if k == name {
				return value
			}
			return ""
		})
		if err == nil || !strings.Contains(err.Error(), name) {
			t.Errorf("%s=%s: %v", name, value, err)
		}
	}
}

func TestRunStopsWithTheContext(t *testing.T) {
	env := map[string]string{"DATABASE_URL": pgtest.URL(t), "RELAY_POLL_INTERVAL": "10ms"}
	if code, _, errOut := cli(t, env, "migrate"); code != 0 {
		t.Fatal(errOut)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	var out, errOut strings.Builder
	code := run(ctx, []string{"run"}, func(k string) string { return env[k] }, &out, &errOut)
	if code != 0 || !strings.Contains(errOut.String(), `"msg":"relay stopped"`) {
		t.Fatalf("exit %d\n%s", code, errOut.String())
	}
}
