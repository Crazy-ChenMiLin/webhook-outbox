package netguard

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
)

func TestCheck(t *testing.T) {
	blocked := []string{
		"127.0.0.1", "127.8.8.8", "::1",
		"10.1.2.3", "172.16.0.1", "172.31.255.255", "192.168.1.1", "fd00::1",
		"169.254.169.254", "fe80::1",
		"0.0.0.0", "0.1.2.3", "::",
		"100.64.0.1", "198.18.0.1", "240.0.0.1", "255.255.255.255",
		"224.0.0.1", "ff02::1",
		"::ffff:127.0.0.1", "::ffff:10.0.0.1",
		"64:ff9b::a9fe:a9fe", // 169.254.169.254 over NAT64
	}
	for _, s := range blocked {
		if err := Check(netip.MustParseAddr(s)); err == nil {
			t.Errorf("%s is allowed, want blocked", s)
		}
	}
	allowed := []string{
		"8.8.8.8", "1.1.1.1", "172.32.0.1", "100.128.0.1", "2606:4700:4700::1111",
		"::ffff:8.8.8.8", "64:ff9b::808:808",
	}
	for _, s := range allowed {
		if err := Check(netip.MustParseAddr(s)); err != nil {
			t.Errorf("%s: %v, want allowed", s, err)
		}
	}
}

func TestCheckNamesTheRange(t *testing.T) {
	err := Check(netip.MustParseAddr("169.254.169.254"))
	if err == nil || !strings.Contains(err.Error(), "metadata") {
		t.Fatalf("err = %v, want a mention of metadata", err)
	}
}

func TestControlBlocksADialAfterDNS(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer srv.Close()
	// localhost is a name, so the check can only happen after it is resolved.
	url := strings.Replace(srv.URL, "127.0.0.1", "localhost", 1)

	dialer := &net.Dialer{Control: Control}
	client := &http.Client{Transport: &http.Transport{DialContext: dialer.DialContext}}
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	resp, err := client.Do(req)
	if err == nil {
		_ = resp.Body.Close()
		t.Fatal("request to localhost went through")
	}
	if !strings.Contains(err.Error(), "blocked") {
		t.Fatalf("err = %v, want blocked", err)
	}
}
