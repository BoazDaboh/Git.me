package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"gitme/companion/internal/workspace"
)

func TestSavedGETChecksUseSavedDefinitionsAndExcludePOST(t *testing.T) {
	var requests atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Method != "GET" || r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" || r.Header.Get("X-GitMe-Client") != "" {
			t.Error("batch inherited request data")
		}
		if r.URL.Path == "/missing" {
			w.WriteHeader(404)
		}
		w.Write([]byte("SENTINEL_RESPONSE_BODY"))
	}))
	defer target.Close()
	store, err := workspace.Open(filepath.Join(t.TempDir(), "test.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	template := func(id, method, path string, expected any) map[string]any {
		record := map[string]any{"id": id, "name": id, "method": method, "url": target.URL + path, "serviceId": "", "updatedAt": 1}
		if expected != nil {
			record["expectedStatus"] = expected
		}
		return record
	}
	project := map[string]any{"id": "p1", "name": "Example", "initials": "e", "color": "blue", "description": "", "stack": []string{}, "stage": "Building", "progress": 0, "branch": "main", "folder": "", "url": "", "local": "", "note": "", "apiRequests": []any{template("legacy", "GET", "/ok", nil), template("fail", "GET", "/missing", 200), template("expected_error", "GET", "/missing", 404), template("post", "POST", "/write", nil)}}
	data := map[string]any{"version": 1, "projects": []any{project}, "tasks": []any{}, "notes": []any{}, "sessions": []any{}, "drafts": []any{}, "focus": nil, "document": map[string]string{"title": "", "text": ""}}
	snapshot, _ := store.Load(t.Context())
	raw, _ := json.Marshal(data)
	if _, err := store.Save(t.Context(), snapshot.DatabaseID, 0, raw); err != nil {
		t.Fatal(err)
	}
	handler := newHandler(time.Now(), store)
	defer handler.(*gitHandler).Close()
	call := func(body, origin, client string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "http://"+address+"/api-test/check", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Origin", origin)
		r.Header.Set("X-GitMe-Client", client)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	for _, test := range []struct {
		body, origin, client string
		want                 int
	}{{`{"project_id":"p1","request_id":"legacy"}`, "https://evil.example", "workspace-v1", 403}, {`{"project_id":"p1","request_id":"legacy"}`, "", "", 415}, {`{"project_id":"p1","request_id":"legacy","url":"https://other.example"}`, "", "workspace-v1", 400}, {`{"project_id":"other","request_id":"legacy"}`, "", "workspace-v1", 409}, {`{"project_id":"p1","request_id":"post"}`, "", "workspace-v1", 409}} {
		w := call(test.body, test.origin, test.client)
		if w.Code != test.want {
			t.Fatal("check boundary", w.Code, w.Body)
		}
	}
	if requests.Load() != 0 {
		t.Fatal("invalid checks contacted target")
	}
	for _, test := range []struct {
		id               string
		expected, actual int
		passed           bool
	}{{"legacy", 200, 200, true}, {"fail", 200, 404, false}, {"expected_error", 404, 404, true}} {
		w := call(`{"project_id":"p1","request_id":"`+test.id+`"}`, "", "workspace-v1")
		var reading struct {
			Expected int  `json:"expected_status"`
			Actual   int  `json:"actual_status"`
			Passed   bool `json:"passed"`
		}
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &reading) != nil || reading.Expected != test.expected || reading.Actual != test.actual || reading.Passed != test.passed || strings.Contains(w.Body.String(), "SENTINEL_RESPONSE") {
			t.Fatal("bad status assertion", w.Code, w.Body)
		}
	}
	if requests.Load() != 3 {
		t.Fatal("extra check requests")
	}
	after, _ := store.Load(t.Context())
	if string(after.Workspace) != string(raw) {
		t.Fatal("check result persisted into workspace")
	}
}
