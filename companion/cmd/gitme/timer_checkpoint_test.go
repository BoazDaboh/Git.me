package main

import (
	"context"
	"encoding/json"
	"gitme/companion/internal/workspace"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTimerRecoveryUsesCheckpointAndPreservesWorkspace(t *testing.T) {
	fixture, err := os.ReadFile(filepath.Join("..", "..", "internal", "workspace", "store_test.go"))
	if err != nil {
		t.Fatal(err)
	}
	p := strings.SplitN(string(fixture), "const sample = `", 2)
	if len(p) != 2 {
		t.Fatal("fixture missing")
	}
	raw := strings.SplitN(p[1], "`", 2)[0]
	store, err := workspace.Open(filepath.Join(t.TempDir(), "gitme.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	snap, err := store.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.Save(ctx, snap.DatabaseID, 0, json.RawMessage(raw)); err != nil {
		t.Fatal(err)
	}
	if err = writeTimerCheckpoint(ctx, store, 1500); err != nil {
		t.Fatal(err)
	}
	recovered, err := recoverTimerCheckpoint(ctx, store, 10000)
	if err != nil || !recovered {
		t.Fatalf("recover: %v %v", recovered, err)
	}
	snap, err = store.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Focus    *activeTimer                     `json:"focus"`
		Sessions []struct{ Started, Ended int64 } `json:"sessions"`
		Notes    []json.RawMessage                `json:"notes"`
	}
	if err = json.Unmarshal(snap.Workspace, &got); err != nil {
		t.Fatal(err)
	}
	if got.Focus != nil || len(got.Sessions) != 2 || got.Sessions[1].Started != 300 || got.Sessions[1].Ended != 1500 || len(got.Notes) != 1 {
		t.Fatalf("unexpected recovery: %+v", got)
	}
	recovered, err = recoverTimerCheckpoint(ctx, store, 10000)
	if err != nil || recovered {
		t.Fatal("recovery repeated", err)
	}
}
func TestTimerRecoveryRejectsMismatchedSession(t *testing.T) {
	store, err := workspace.Open(filepath.Join(t.TempDir(), "gitme.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	snap, err := store.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(timerCheckpoint{DatabaseID: snap.DatabaseID, Started: 1, Checked: 2})
	if err = os.WriteFile(checkpointPath(store), data, 0600); err != nil {
		t.Fatal(err)
	}
	recovered, err := recoverTimerCheckpoint(context.Background(), store, 10)
	if recovered || err != nil {
		t.Fatal("recovered an empty database", err)
	}
}
