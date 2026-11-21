package apitester

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestJSONPostReviewsBindExactDataAndSendOnce(t *testing.T) {
	var sent atomic.Int32
	body := "{\n \"name\": \"Ikeja\", \"private\": \"SENTINEL_BODY\"\n}"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sent.Add(1)
		raw, _ := io.ReadAll(r.Body)
		if r.Method != "POST" || string(raw) != body || r.Header.Get("Authorization") != "Bearer SENTINEL_AUTH" || r.Header.Get("Content-Type") != "application/json" {
			t.Error("reviewed request changed")
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(201)
		fmt.Fprint(w, `{"created":true}`)
	}))
	defer server.Close()
	input := Input{URL: server.URL + "/communities", Method: "POST", Body: body, Headers: []Header{{"Authorization", "Bearer SENTINEL_AUTH"}}}
	reviews := NewReviews()
	prepare := func() PostPlan {
		t.Helper()
		p, err := reviews.Prepare("project/service", "fingerprint", input)
		if err != nil {
			t.Fatal(err)
		}
		return p
	}
	first := prepare()
	second := prepare()
	if sent.Load() != 0 {
		t.Fatal("review sent data")
	}
	if _, err := reviews.Execute(t.Context(), "project/service", "fingerprint", first.Token, Runner{}); err == nil {
		t.Fatal("superseded review executed")
	}
	if _, err := reviews.Execute(t.Context(), "other/service", "fingerprint", second.Token, Runner{}); err == nil {
		t.Fatal("cross-service execution")
	}
	reviews.Cancel("project/service", second.Token)
	if _, err := reviews.Execute(t.Context(), "project/service", "fingerprint", second.Token, Runner{}); err == nil {
		t.Fatal("canceled review executed")
	}
	changed := prepare()
	if _, err := reviews.Execute(t.Context(), "project/service", "changed settings", changed.Token, Runner{}); err == nil {
		t.Fatal("changed scope accepted")
	}
	expired := prepare()
	now := time.Now()
	reviews.now = func() time.Time { return now.Add(6 * time.Minute) }
	if _, err := reviews.Execute(t.Context(), "project/service", "fingerprint", expired.Token, Runner{}); err == nil {
		t.Fatal("expired review executed")
	}
	reviews.now = time.Now
	plan := prepare()
	raw, _ := json.Marshal(plan)
	if strings.Contains(string(raw), "SENTINEL") {
		t.Fatal("review returns request credentials or body")
	}
	input.Headers[0].Value = "changed after review"
	result, err := reviews.Execute(t.Context(), "project/service", "fingerprint", plan.Token, Runner{})
	if err != nil || result.Status != 201 || result.Delivery != "response" || sent.Load() != 1 {
		t.Fatal(result, err)
	}
	if _, err := reviews.Execute(t.Context(), "project/service", "fingerprint", plan.Token, Runner{}); err == nil || sent.Load() != 1 {
		t.Fatal("review replay")
	}
	for _, bad := range []Input{{URL: server.URL, Method: "POST", Body: "invalid"}, {URL: server.URL, Method: "POST", Body: `{"x":"` + strings.Repeat("a", MaxJSON) + `"}`}, {URL: server.URL, Method: "POST", Body: "{}", Headers: []Header{{"Content-Type", "text/plain"}}}, {URL: server.URL, Method: "DELETE", Body: "{}"}} {
		if _, err := reviews.Prepare("p/s", "f", bad); err == nil {
			t.Fatal("invalid POST review accepted")
		}
	}
	if sent.Load() != 1 {
		t.Fatal("invalid reviews sent data")
	}
}

func TestInterruptedPostIsUnconfirmedWithoutRetry(t *testing.T) {
	var sent atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.ReadAll(r.Body)
		sent.Add(1)
		connection, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		connection.Close()
	}))
	defer server.Close()
	input := Input{URL: server.URL, Method: "POST", Body: "{}", Headers: []Header{{"Idempotency-Key", "fixture-only"}}}
	result, err := (Runner{}).Run(t.Context(), input)
	if err != nil || result.Status != 0 || result.Delivery != "unconfirmed" || sent.Load() != 1 {
		t.Fatal("uncertain POST retried or misclassified", result, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	before := sent.Load()
	result, err = (Runner{}).Run(ctx, input)
	if err != nil || sent.Load() != before || result.Delivery != "not-sent" {
		t.Fatal("pre-cancelled request sent", result, err)
	}
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { sent.Add(1); http.Redirect(w, r, server.URL, 307) }))
	defer redirect.Close()
	input.URL = redirect.URL
	before = sent.Load()
	result, err = (Runner{}).Run(t.Context(), input)
	if err != nil || result.Status != 307 || result.Delivery != "response" || sent.Load() != before+1 {
		t.Fatal("POST redirect followed", result, err)
	}
}
