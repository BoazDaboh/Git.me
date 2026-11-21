package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"gitme/companion/internal/workspace"
)

func TestAPIRequestsRequireSavedProjectAndExplicitGET(t *testing.T) {
	store, err := workspace.Open(filepath.Join(t.TempDir(), "test.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	project := map[string]any{"id": "p1", "name": "Example", "initials": "e", "color": "blue", "description": "", "stack": []string{}, "stage": "Building", "progress": 0, "branch": "main", "folder": "", "url": "", "local": "", "note": ""}
	data := map[string]any{"version": 1, "projects": []any{project}, "tasks": []any{}, "notes": []any{}, "sessions": []any{}, "drafts": []any{}, "focus": nil, "document": map[string]string{"title": "", "text": ""}}
	before, _ := store.Load(t.Context())
	raw, _ := json.Marshal(data)
	if _, err := store.Save(t.Context(), before.DatabaseID, 0, raw); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.Header.Get("Authorization") != "Bearer TEST_ONLY_HEADER" {
			t.Error("wrong outbound request")
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"ok":true}`)
	}))
	defer server.Close()
	handler := newHandler(time.Now(), store)
	defer handler.(*gitHandler).Close()
	input := `{"project_id":"p1","service_id":"","url":"` + server.URL + `","headers":[{"name":"Authorization","value":"Bearer TEST_ONLY_HEADER"}]}`
	for _, test := range []struct {
		body, origin, client string
		status               int
	}{{input, "http://127.0.0.1:3015", "workspace-v1", 200}, {input, "https://evil.example", "workspace-v1", 403}, {input, "", "", 415}, {strings.Replace(input, `"p1"`, `"missing"`, 1), "", "workspace-v1", 409}, {strings.TrimSuffix(input, "}") + `,"method":"POST"}`, "", "workspace-v1", 400}, {strings.TrimSuffix(input, "}") + `,"body":"x"}`, "", "workspace-v1", 400}, {strings.Replace(input, `"service_id":""`, `"service_id":"other"`, 1), "", "workspace-v1", 409}} {
		r := httptest.NewRequest("POST", "http://"+address+"/api-test/send", strings.NewReader(test.body))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Origin", test.origin)
		r.Header.Set("X-GitMe-Client", test.client)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != test.status {
			t.Fatal("API boundary", w.Code, w.Body)
		}
	}
	after, _ := store.Load(t.Context())
	if strings.Contains(string(after.Workspace), "TEST_ONLY_HEADER") || string(after.Workspace) != string(raw) {
		t.Fatal("API testing persisted transient inputs or response")
	}
}

func TestReviewedPOSTRoutesSendOnlyAfterConfirmation(t *testing.T) {
	store, err := workspace.Open(filepath.Join(t.TempDir(), "test.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	project := map[string]any{"id": "p1", "name": "Example", "initials": "e", "color": "blue", "description": "", "stack": []string{}, "stage": "Building", "progress": 0, "branch": "main", "folder": "", "url": "", "local": "", "note": ""}
	data := map[string]any{"version": 1, "projects": []any{project}, "tasks": []any{}, "notes": []any{}, "sessions": []any{}, "drafts": []any{}, "focus": nil, "document": map[string]string{"title": "", "text": ""}}
	snapshot, _ := store.Load(t.Context())
	raw, _ := json.Marshal(data)
	if _, err := store.Save(t.Context(), snapshot.DatabaseID, 0, raw); err != nil {
		t.Fatal(err)
	}
	var posts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		posts.Add(1)
		body, _ := io.ReadAll(r.Body)
		if r.Method != "POST" || string(body) != `{"name":"SENTINEL_PRIVATE_BODY"}` || r.Header.Get("Authorization") != "Bearer SENTINEL_AUTH" {
			t.Error("POST changed")
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(201)
		fmt.Fprint(w, `{"ok":true}`)
	}))
	defer server.Close()
	handler := newHandler(time.Now(), store)
	defer handler.(*gitHandler).Close()
	call := func(path string, body any, origin, client string) *httptest.ResponseRecorder {
		encoded, _ := json.Marshal(body)
		r := httptest.NewRequest("POST", "http://"+address+"/api-test/"+path, strings.NewReader(string(encoded)))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-GitMe-Client", client)
		r.Header.Set("Origin", origin)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	input := map[string]any{"project_id": "p1", "service_id": "", "url": server.URL, "body": `{"name":"SENTINEL_PRIVATE_BODY"}`, "headers": []map[string]string{{"name": "Authorization", "value": "Bearer SENTINEL_AUTH"}}}
	if call("post-review", input, "https://evil.example", "workspace-v1").Code != 403 || call("post-review", input, "", "").Code != 415 {
		t.Fatal("POST guard bypass")
	}
	prepared := call("post-review", input, "", "workspace-v1")
	if prepared.Code != 200 || posts.Load() != 0 || strings.Contains(prepared.Body.String(), "SENTINEL") {
		t.Fatal("unsafe review", prepared.Code, prepared.Body)
	}
	var plan struct {
		Token string `json:"token"`
	}
	json.Unmarshal(prepared.Body.Bytes(), &plan)
	execution := map[string]any{"project_id": "p1", "service_id": "", "token": plan.Token}
	if call("post-execute", map[string]any{"project_id": "p1", "service_id": "", "token": plan.Token, "body": "changed"}, "", "workspace-v1").Code != 400 {
		t.Fatal("execution accepted payload override")
	}
	result := call("post-execute", execution, "", "workspace-v1")
	if result.Code != 200 || posts.Load() != 1 || !strings.Contains(result.Body.String(), `"status":201`) {
		t.Fatal("POST failed", result.Code, result.Body)
	}
	if call("post-execute", execution, "", "workspace-v1").Code != 409 || posts.Load() != 1 {
		t.Fatal("POST replay")
	}
	after, _ := store.Load(t.Context())
	if string(after.Workspace) != string(raw) || strings.Contains(string(after.Workspace), "SENTINEL") {
		t.Fatal("POST persisted request data")
	}
}
