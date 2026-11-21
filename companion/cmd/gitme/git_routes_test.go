package main

import (
	"gitme/companion/internal/processes"
	"gitme/companion/internal/workspace"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLocalGitRoutesRejectUntrustedAndArbitraryCommands(t *testing.T) {
	store, err := workspace.Open(filepath.Join(t.TempDir(), "test.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	manager := processes.New(store)
	defer manager.Close()
	handler := newHandlerWithCloud(time.Now(), store, manager, fixtureGitHub(t), fixtureVercel(t))
	defer handler.(*gitHandler).Close()
	for _, test := range []struct {
		path, body, origin string
		client             bool
		status             int
	}{
		{"status", `{"project_id":"p","service_id":"s"}`, "https://evil.example", true, 403},
		{"status", `{"project_id":"p","service_id":"s"}`, "", false, 415},
		{"prepare", `{"project_id":"p","service_id":"s","command":"git push"}`, "", true, 400},
		{"execute", `{"project_id":"p","service_id":"s","folder":"C:\\private","token":"fake"}`, "", true, 400},
		{"execute", `{"project_id":"p","service_id":"s","token":"fake"}`, "", true, 409},
		{"push", `{}`, "", true, 404},
		{"push-review", `{"project_id":"p","service_id":"s"}`, "https://evil.example", true, 403},
		{"push-review", `{"project_id":"p","service_id":"s","url":"https://evil.example/repo"}`, "", true, 400},
		{"push-execute", `{"project_id":"p","service_id":"s","command":"git push --force"}`, "", true, 400},
		{"push-execute", `{"project_id":"p","service_id":"s","token":"fake"}`, "", true, 409},
	} {
		r := httptest.NewRequest("POST", "http://"+address+"/git/"+test.path, strings.NewReader(test.body))
		r.Header.Set("Origin", test.origin)
		r.Header.Set("Content-Type", "application/json")
		if test.client {
			r.Header.Set("X-GitMe-Client", "workspace-v1")
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != test.status {
			t.Fatalf("unexpected Git route response %s %d", test.path, w.Code)
		}
	}
	snapshot, err := store.Load(t.Context())
	if err != nil || snapshot.Revision != 0 {
		t.Fatal("invalid request modified workspace")
	}
}
