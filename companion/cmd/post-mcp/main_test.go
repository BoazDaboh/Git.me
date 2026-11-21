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

func TestDraftChangesPreserveWorkspace(t *testing.T) {
	file, err := os.ReadFile(filepath.Join("..", "..", "internal", "workspace", "store_test.go"))
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.SplitN(string(file), "const sample = `", 2)
	if len(parts) != 2 {
		t.Fatal("fixture missing")
	}
	sample := strings.SplitN(parts[1], "`", 2)[0]
	store, err := workspace.Open(filepath.Join(t.TempDir(), "test.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	snap, err := store.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.Save(ctx, snap.DatabaseID, 0, json.RawMessage(sample)); err != nil {
		t.Fatal(err)
	}
	var document map[string]json.RawMessage
	if err = json.Unmarshal(json.RawMessage(sample), &document); err != nil {
		t.Fatal(err)
	}
	document["postContexts"] = json.RawMessage(`[{"projectId":"p1","projectName":"GitMe","description":"Local","repository":"owner/repo","commits":[{"sha":"abc","title":"Update","date":""}],"extra":"Context","platform":"X","tone":"Casual","savedAt":123}]`)
	withContext, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	snap, err = store.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.Save(ctx, snap.DatabaseID, snap.Revision, withContext); err != nil {
		t.Fatal(err)
	}
	_, contextState, err := read(ctx, store)
	if err != nil || len(contextState.Contexts) != 1 || contextState.Contexts[0].Extra != "Context" {
		t.Fatal("context not saved", err)
	}
	if _, err = saveDraft(ctx, store, Draft{Project: "p1", Platform: "X", Text: "Hello"}, nil); err != nil {
		t.Fatal(err)
	}
	_, state, err := read(ctx, store)
	if err != nil || len(state.Drafts) != 2 {
		t.Fatalf("drafts: %+v %v", state.Drafts, err)
	}
	wrong := "stale"
	if _, err = saveDraft(ctx, store, Draft{ID: state.Drafts[0].ID, Text: "Changed"}, &wrong); err == nil {
		t.Fatal("accepted stale edit")
	}
	correct := "Hello"
	if _, err = saveDraft(ctx, store, Draft{ID: state.Drafts[0].ID, Text: "Changed"}, &correct); err != nil {
		t.Fatal(err)
	}
	snap, state, err = read(ctx, store)
	if err != nil || state.Drafts[0].Text != "Changed" {
		t.Fatal("missing update", err)
	}
	var full map[string]json.RawMessage
	if json.Unmarshal(snap.Workspace, &full) != nil || !strings.Contains(string(full["notes"]), "A note") {
		t.Fatal("notes not preserved")
	}
}
