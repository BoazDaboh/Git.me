package main

import (
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gitme/companion/internal/processes"
	"gitme/companion/internal/releases"
	"gitme/companion/internal/workspace"
)

func TestBrowserFixture(t *testing.T) {
	directory := os.Getenv("GITME_BROWSER_TEST_DIR")
	if directory == "" {
		t.Skip("opt-in isolated browser fixture")
	}
	store, err := workspace.Open(filepath.Join(directory, "gitme.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	listener, err := net.Listen("tcp", "127.0.0.1:47822")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{}, 1)
	manager := processes.New(store)
	defer manager.Close()
	handler := newHandlerWithCloud(time.Now(), store, manager, fixtureGitHub(t), fixtureVercelMode(t, true))
	defer handler.(*gitHandler).Close()
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/__fixture/release" && r.Method == "POST" {
			var record releases.Record
			if json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&record) != nil {
				http.Error(w, "bad fixture record", 400)
				return
			}
			saved, err := store.RecordRelease(r.Context(), record)
			if err != nil {
				http.Error(w, "fixture record rejected", 400)
				return
			}
			respondJSON(w, 200, saved)
			return
		}
		if r.URL.Path == "/__fixture/stop" {
			w.WriteHeader(204)
			select {
			case done <- struct{}{}:
			default:
			}
			return
		}
		handler.ServeHTTP(w, r)
	}), ReadHeaderTimeout: time.Second}
	go func() { _ = server.Serve(listener) }()
	t.Log("BROWSER_FIXTURE_READY")
	select {
	case <-done:
	case <-time.After(4 * time.Minute):
		t.Error("browser fixture timed out")
	}
	_ = server.Close()
}
