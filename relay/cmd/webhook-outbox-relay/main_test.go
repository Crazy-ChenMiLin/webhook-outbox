package main

import (
	"bytes"
	"context"
	"testing"

	"github.com/ianfoxdev/webhook-outbox/relay/internal/pgtest"
)

// cli runs a command with the given environment and returns its exit code and output.
func cli(t *testing.T, env map[string]string, args ...string) (int, string, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code := run(context.Background(), args, func(k string) string { return env[k] }, &out, &errOut)
	return code, out.String(), errOut.String()
}

func TestVersion(t *testing.T) {
	code, out, _ := cli(t, nil, "version")
	if code != 0 || out != "dev\n" {
		t.Fatalf("exit %d, %q", code, out)
	}
}

func TestUsage(t *testing.T) {
	for _, args := range [][]string{nil, {"nope"}} {
		if code, _, errOut := cli(t, nil, args...); code != 2 || errOut == "" {
			t.Fatalf("%v: exit %d", args, code)
		}
	}
}

func TestMigrateNeedsDatabaseURL(t *testing.T) {
	code, _, errOut := cli(t, nil, "migrate")
	if code != 2 || errOut != "DATABASE_URL is not set\n" {
		t.Fatalf("exit %d, %q", code, errOut)
	}
}

func TestMigrate(t *testing.T) {
	env := map[string]string{"DATABASE_URL": pgtest.URL(t)}
	if code, out, errOut := cli(t, env, "migrate"); code != 0 || out != "applied 0001_init.sql\n" {
		t.Fatalf("exit %d, %q, %q", code, out, errOut)
	}
	if code, out, _ := cli(t, env, "migrate"); code != 0 || out != "up to date\n" {
		t.Fatalf("exit %d, %q", code, out)
	}
}
