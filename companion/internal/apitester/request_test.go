package apitester

import (
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestGETResponseLimitsHeadersAndRedirects(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != "GET" {
			t.Error("method changed")
		}
		if r.Header.Get("Cookie") != "" || r.Header.Get("X-GitMe-Client") != "" {
			t.Error("ambient dashboard credentials leaked")
		}
		switch r.URL.Path {
		case "/data":
			if r.Header.Get("Authorization") != "Bearer TEST_SESSION_VALUE" || r.URL.Query().Get("q") != "lagos" {
				t.Error("explicit header or query missing")
			}
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Set-Cookie", "SENTINEL_COOKIE_VALUE")
			w.Header().Set("X-Auth-Token", "SENTINEL_RESPONSE_TOKEN")
			fmt.Fprint(w, `{"city":"Lagos","ok":true}`)
		case "/missing":
			w.WriteHeader(404)
			fmt.Fprint(w, "Missing resource")
		case "/redirect":
			http.Redirect(w, r, "/data", 302)
		case "/large":
			w.Header().Set("Content-Type", "text/plain")
			w.Header().Set("Content-Encoding", "gzip")
			z := gzip.NewWriter(w)
			z.Write([]byte(strings.Repeat("a", MaxBody+200)))
			z.Close()
		case "/binary":
			w.Header().Set("Content-Type", "application/pdf")
			w.Write([]byte{0, 1, 2, 3})
		case "/slow":
			<-r.Context().Done()
		}
	}))
	defer server.Close()
	runner := Runner{}
	result, err := runner.Run(t.Context(), Input{URL: server.URL + "/data?q=lagos", Headers: []Header{{"Authorization", "Bearer TEST_SESSION_VALUE"}}})
	if err != nil || result.Status != 200 || result.Body != `{"city":"Lagos","ok":true}` || result.Binary {
		t.Fatal(result, err)
	}
	for _, h := range result.Headers {
		if strings.Contains(h.Value, "SENTINEL") {
			t.Fatal("sensitive response header not hidden")
		}
	}
	before := calls.Load()
	redirect, err := runner.Run(t.Context(), Input{URL: server.URL + "/redirect"})
	if err != nil || redirect.Status != 302 || calls.Load() != before+1 {
		t.Fatal("redirect followed", err)
	}
	missing, err := runner.Run(t.Context(), Input{URL: server.URL + "/missing"})
	if err != nil || missing.Status != 404 || missing.Body != "Missing resource" {
		t.Fatal("HTTP error lost", missing, err)
	}
	large, err := runner.Run(t.Context(), Input{URL: server.URL + "/large"})
	if err != nil || !large.Truncated || large.Bytes != MaxBody || len(large.Body) != MaxBody {
		t.Fatal("response not bounded", err)
	}
	binary, err := runner.Run(t.Context(), Input{URL: server.URL + "/binary"})
	if err != nil || !binary.Binary || binary.Body != "" {
		t.Fatal("binary rendered", err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	slow, err := runner.Run(ctx, Input{URL: server.URL + "/slow"})
	if err != nil || slow.Status != 0 || slow.Message == "" {
		t.Fatal("cancellation lost", err)
	}
}

func TestDestinationAndHeaderBoundaries(t *testing.T) {
	for _, raw := range []string{"file:///private", "ftp://example.test/a", "http://user:password@example.test", "http://localhost:47821/workspace", "http://127.0.0.1:3015", "http://169.254.169.254/latest", "http://[fd00:ec2::254]/", "http://metadata.google.internal/", "http://0.0.0.0/", "http://224.0.0.1/", "http://100.100.100.200/", "http://168.63.129.16/", "http://example.test/#fragment"} {
		if _, err := URL(raw); err == nil {
			t.Fatal("unsafe destination", raw)
		}
	}
	for _, headers := range [][]Header{{{"Host", "other.test"}}, {{"X-GitMe-Client", "workspace-v1"}}, {{"X-Test", "x\r\nInjected: yes"}}, {{"Accept", "a"}, {"accept", "b"}}, {{"Authorization", strings.Repeat("x", 4097)}}} {
		if err := validate(Input{URL: "http://localhost:8080", Headers: headers}); err == nil {
			t.Fatal("unsafe headers")
		}
	}
	dialed := false
	runner := Runner{Resolve: func(context.Context, string) ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("169.254.169.254")}, nil
	}, Dial: func(context.Context, string, string) (net.Conn, error) {
		dialed = true
		return nil, errors.New("unexpected")
	}}
	result, err := runner.Run(t.Context(), Input{URL: "https://example.test/data"})
	if err != nil || result.Status != 0 || dialed {
		t.Fatal("DNS rebinding boundary failed", result, err)
	}
	var attempts []string
	runner.Resolve = func(context.Context, string) ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("::1"), netip.MustParseAddr("127.0.0.1")}, nil
	}
	runner.Dial = func(_ context.Context, _ string, address string) (net.Conn, error) {
		attempts = append(attempts, address)
		return nil, errors.New("fixture unavailable")
	}
	runner.Run(t.Context(), Input{URL: "http://localhost:8080/health"})
	if len(attempts) != 2 || attempts[0] != "[::1]:8080" || attempts[1] != "127.0.0.1:8080" {
		t.Fatal("validated localhost fallback missing", attempts)
	}
}

func TestStatusOnlyCheckDoesNotWaitForOrReturnBody(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Auth-Token", "SENTINEL_HEADER")
		w.WriteHeader(200)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer target.Close()
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	result, err := (Runner{}).Run(ctx, Input{URL: target.URL, StatusOnly: true})
	if err != nil || result.Status != 200 || result.Body != "" || len(result.Headers) != 0 || ctx.Err() != nil {
		t.Fatal("status check waited for body", result, err)
	}
}
