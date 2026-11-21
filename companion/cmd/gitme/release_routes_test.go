package main

import (
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gitme/companion/internal/processes"
	"gitme/companion/internal/workspace"
)

func TestReleaseInspectionRequiresSavedServiceAndTrustedClient(t *testing.T) {
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
		body, origin string
		client       bool
		want         int
	}{
		{`{"project_id":"p","service_id":"s"}`, "https://evil.example", true, 403},
		{`{"project_id":"p","service_id":"s"}`, "", false, 415},
		{`{"project_id":"p","service_id":"s","folder":"C:\\private"}`, "", true, 400},
		{`{"project_id":"p"}`, "", true, 400},
		{`{"project_id":"p","service_id":"s"}`, "", true, 404},
	} {
		req := httptest.NewRequest("POST", "http://"+address+"/releases/inspect", strings.NewReader(test.body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", test.origin)
		if test.client {
			req.Header.Set("X-GitMe-Client", "workspace-v1")
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, req)
		if response.Code != test.want {
			t.Fatalf("unexpected inspection response %d", response.Code)
		}
	}
	snapshot, err := store.Load(t.Context())
	if err != nil || snapshot.Revision != 0 {
		t.Fatal("inspection mutated workspace")
	}
}
