package main

import (
	"bytes"
	"net/http/httptest"
	"testing"
	"time"
)

func TestProcessRoutesRequireReviewedSavedCommands(t *testing.T) {
	handler := testHandler(t, time.Now())
	for _, item := range []struct {
		name, path, origin, body, client string
		status                           int
	}{
		{"missing review", "/services/run", "", `{"token":"not-issued"}`, "workspace-v1", 409},
		{"injected command", "/services/run", "", `{"token":"x","command":"echo unwanted"}`, "workspace-v1", 400},
		{"injected PID", "/services/stop", "", `{"project_id":"p1","service_id":"s1","run_id":"x","pid":1}`, "workspace-v1", 400},
		{"unknown ownership", "/services/stop", "", `{"project_id":"p1","service_id":"s1","run_id":"x"}`, "workspace-v1", 409},
		{"untrusted website", "/services/prepare", "https://example.com", `{"project_id":"p1","service_id":"s1","action":"start"}`, "workspace-v1", 403},
		{"missing header", "/services/prepare", "", `{}`, "", 415},
		{"unknown source", "/services/prepare", "", `{"project_id":"missing","service_id":"missing","action":"start"}`, "workspace-v1", 409},
	} {
		t.Run(item.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "http://"+address+item.path, bytes.NewBufferString(item.body))
			req.Header.Set("Origin", item.origin)
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-GitMe-Client", item.client)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, req)
			if response.Code != item.status {
				t.Fatalf("status=%d body=%s", response.Code, response.Body)
			}
		})
	}
	for _, path := range []string{"/services/run", "/services/stop", "/services/health"} {
		req := httptest.NewRequest("OPTIONS", "http://"+address+path, nil)
		req.Header.Set("Origin", "http://127.0.0.1:3015")
		req.Header.Set("Access-Control-Request-Method", "POST")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, req)
		if response.Code != 204 || response.Header().Get("Access-Control-Allow-Headers") != "Content-Type, X-GitMe-Client" {
			t.Fatal("process preflight failed")
		}
	}
}
