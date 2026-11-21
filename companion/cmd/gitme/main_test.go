package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gitme/companion/internal/workspace"
)

func testHandler(t *testing.T, started time.Time) http.Handler {
	t.Helper()
	store, err := workspace.Open(filepath.Join(t.TempDir(), "test.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	handler := newHandler(started, store)
	t.Cleanup(func() { handler.(*gitHandler).Close() })
	return handler
}

func TestHealthIdentity(t *testing.T) {
	started := time.Date(2026, 11, 18, 10, 0, 0, 0, time.UTC)
	req := httptest.NewRequest(http.MethodGet, "http://"+address+"/health", nil)
	req.Header.Set("Origin", "http://127.0.0.1:3015")
	response := httptest.NewRecorder()
	testHandler(t, started).ServeHTTP(response, req)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d", response.Code)
	}
	var payload healthResponse
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.App != "gitme-companion" || payload.Status != "ok" || payload.Protocol != 1 || payload.Version != "0.17.0" || payload.StartedAt != "2026-11-18T10:00:00Z" {
		t.Fatalf("unexpected identity: %+v", payload)
	}
	if len(payload.Capabilities) != 16 || payload.Capabilities[0] != "health" || payload.Capabilities[1] != "workspace" || payload.Capabilities[2] != "project-scan" || payload.Capabilities[3] != "processes" || payload.Capabilities[4] != "github" || payload.Capabilities[5] != "github-accounts" || payload.Capabilities[6] != "vercel" || payload.Capabilities[7] != "release-inspection" || payload.Capabilities[8] != "local-git" || payload.Capabilities[9] != "git-push" || payload.Capabilities[10] != "deployment-tracking" || payload.Capabilities[11] != "reviewed-deployments" || payload.Capabilities[12] != "local-uploads" || payload.Capabilities[13] != "api-testing" || payload.Capabilities[14] != "reviewed-api-posts" || payload.Capabilities[15] != "api-status-checks" {
		t.Fatalf("unexpected capabilities: %v", payload.Capabilities)
	}
	if response.Header().Get("Cache-Control") != "no-store" || response.Header().Get("Access-Control-Allow-Origin") != "http://127.0.0.1:3015" {
		t.Fatal("missing cache or origin headers")
	}
}

func TestAccessAndRoutes(t *testing.T) {
	cases := []struct {
		name, method, host, path, origin, preflight string
		status                                      int
	}{
		{"direct address", "GET", address, "/health", "", "", 200},
		{"localhost app", "GET", "localhost:47821", "/health", "http://localhost:3015", "", 200},
		{"different website", "GET", address, "/health", "https://example.com", "", 403},
		{"null origin", "GET", address, "/health", "null", "", 403},
		{"different host", "GET", "example.com:47821", "/health", "", "", 403},
		{"wrong method", "POST", address, "/health", "", "", 405},
		{"unknown route", "GET", address, "/commands", "", "", 404},
		{"preflight", "OPTIONS", address, "/health", "http://127.0.0.1:3015", "GET", 204},
		{"blocked preflight", "OPTIONS", address, "/health", "http://127.0.0.1:3015", "POST", 405},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			req := httptest.NewRequest(item.method, "http://"+item.host+item.path, nil)
			req.Header.Set("Origin", item.origin)
			req.Header.Set("Access-Control-Request-Method", item.preflight)
			response := httptest.NewRecorder()
			testHandler(t, time.Now()).ServeHTTP(response, req)
			if response.Code != item.status {
				t.Fatalf("status = %d, want %d", response.Code, item.status)
			}
			if response.Header().Get("Access-Control-Allow-Origin") == "*" {
				t.Fatal("unexpected wildcard origin")
			}
			if item.status == 403 && response.Header().Get("Access-Control-Allow-Origin") != "" {
				t.Fatal("forbidden request received access header")
			}
		})
	}
}

func TestWorkspaceHTTPGuards(t *testing.T) {
	handler := testHandler(t, time.Now())
	cases := []struct {
		name, method, origin, contentType, client, body string
		status                                          int
	}{
		{"read", "GET", "http://127.0.0.1:3015", "", "", "", 200},
		{"blocked read", "GET", "https://example.com", "", "", "", 403},
		{"blocked write", "PUT", "https://example.com", "application/json", "workspace-v1", "{}", 403},
		{"plain form", "PUT", "", "text/plain", "", "{}", 415},
		{"missing client", "PUT", "", "application/json", "", "{}", 415},
		{"bad JSON", "PUT", "", "application/json", "workspace-v1", "{", 400},
		{"missing workspace", "PUT", "", "application/json", "workspace-v1", `{"database_id":"id","revision":0}`, 400},
		{"extra JSON", "PUT", "", "application/json", "workspace-v1", `{} {}`, 400},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			req := httptest.NewRequest(item.method, "http://"+address+"/workspace", bytes.NewBufferString(item.body))
			req.Header.Set("Origin", item.origin)
			req.Header.Set("Content-Type", item.contentType)
			req.Header.Set("X-GitMe-Client", item.client)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, req)
			if response.Code != item.status {
				t.Fatalf("status %d: %s", response.Code, response.Body.String())
			}
		})
	}
	req := httptest.NewRequest("OPTIONS", "http://"+address+"/workspace", nil)
	req.Header.Set("Origin", "http://127.0.0.1:3015")
	req.Header.Set("Access-Control-Request-Method", "PUT")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	if response.Code != 204 || response.Header().Get("Access-Control-Allow-Headers") != "Content-Type, X-GitMe-Client" {
		t.Fatal("workspace preflight failed")
	}
}

func TestProjectScanUsesSavedFolderAndDoesNotWriteWorkspace(t *testing.T) {
	folder := t.TempDir()
	if err := os.WriteFile(filepath.Join(folder, "package.json"), []byte(`{"name":"fixture","scripts":{"dev":"node server.js"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	store, err := workspace.Open(filepath.Join(t.TempDir(), "test.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	initial, err := store.Load(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	project := map[string]any{"id": "p1", "name": "Example", "initials": "e", "color": "blue", "description": "", "stack": []string{}, "stage": "Building", "progress": 0, "branch": "main", "folder": folder, "url": "", "local": "", "note": ""}
	data := map[string]any{"version": 1, "projects": []any{project}, "tasks": []any{}, "notes": []any{}, "sessions": []any{}, "drafts": []any{}, "focus": nil, "document": map[string]string{"title": "", "text": ""}}
	raw, _ := json.Marshal(data)
	if _, err := store.Save(t.Context(), initial.DatabaseID, 0, raw); err != nil {
		t.Fatal(err)
	}
	handler := newHandler(time.Now(), store)
	for _, item := range []struct {
		name, body, origin, client string
		status                     int
	}{
		{"valid", `{"project_id":"p1"}`, "http://127.0.0.1:3015", "workspace-v1", 200},
		{"unknown", `{"project_id":"missing"}`, "", "workspace-v1", 404},
		{"arbitrary path", `{"project_id":"p1","folder":"C:/other"}`, "", "workspace-v1", 400},
		{"another origin", `{"project_id":"p1"}`, "https://example.com", "workspace-v1", 403},
		{"missing client", `{"project_id":"p1"}`, "", "", 415},
		{"extra body", `{"project_id":"p1"}{}`, "", "workspace-v1", 400},
	} {
		t.Run(item.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "http://"+address+"/projects/scan", bytes.NewBufferString(item.body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-GitMe-Client", item.client)
			req.Header.Set("Origin", item.origin)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, req)
			if response.Code != item.status {
				t.Fatalf("status=%d body=%s", response.Code, response.Body)
			}
			if item.status == 200 {
				var result struct {
					Services []struct {
						Folder string `json:"folder"`
					} `json:"services"`
				}
				if json.Unmarshal(response.Body.Bytes(), &result) != nil || len(result.Services) != 1 || result.Services[0].Folder != folder {
					t.Fatalf("result=%s", response.Body)
				}
			}
		})
	}
	after, err := store.Load(t.Context())
	if err != nil || after.Revision != 1 {
		t.Fatalf("scan mutated workspace: %+v %v", after, err)
	}
	req := httptest.NewRequest("OPTIONS", "http://"+address+"/projects/scan", nil)
	req.Header.Set("Origin", "http://127.0.0.1:3015")
	req.Header.Set("Access-Control-Request-Method", "POST")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	if response.Code != 204 || response.Header().Get("Access-Control-Allow-Headers") != "Content-Type, X-GitMe-Client" {
		t.Fatal("scan preflight failed")
	}
}
