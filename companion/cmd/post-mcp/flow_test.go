package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gitme/companion/internal/workspace"
)

func TestMCPDraftFlow(t *testing.T) {
	ctx := context.Background()
	file, err := os.ReadFile(filepath.Join("..", "..", "internal", "workspace", "store_test.go"))
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.SplitN(string(file), "const sample = `", 2)
	if len(parts) != 2 {
		t.Fatal("fixture missing")
	}
	sample := strings.SplitN(parts[1], "`", 2)[0]
	path := filepath.Join(t.TempDir(), "workspace.sqlite")
	store, err := workspace.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var data map[string]json.RawMessage
	if err := json.Unmarshal([]byte(sample), &data); err != nil {
		t.Fatal(err)
	}
	var projects []map[string]any
	if err := json.Unmarshal(data["projects"], &projects); err != nil {
		t.Fatal(err)
	}
	other := map[string]any{}
	for k, v := range projects[0] {
		other[k] = v
	}
	other["id"] = "p2"
	other["name"] = "Other"
	projects = append(projects, other)
	data["projects"], _ = json.Marshal(projects)
	writeContext := func(title string) {
		t.Helper()
		data["postContexts"] = json.RawMessage(`[{"projectId":"p1","projectName":"GitMe","description":"Local","repository":"owner/repo","commits":[{"sha":"abc123","title":"` + title + `","date":""}],"extra":"Context","platform":"X","tone":"Casual","savedAt":123}]`)
		raw, e := json.Marshal(data)
		if e != nil {
			t.Fatal(e)
		}
		snap, e := store.Load(ctx)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = store.Save(ctx, snap.DatabaseID, snap.Revision, raw); e != nil {
			t.Fatal(e)
		}
	}
	raw, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.Save(ctx, snapshot.DatabaseID, 0, raw); err != nil {
		t.Fatal(err)
	}
	writeContext("First update")
	ct, st := mcp.NewInMemoryTransports()
	serverSession, err := newServer(store).Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer serverSession.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	session, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	call := func(name string, args map[string]any) *mcp.CallToolResult {
		t.Helper()
		res, e := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
		if e != nil {
			t.Fatal(e)
		}
		return res
	}
	content := func(res *mcp.CallToolResult) string {
		t.Helper()
		b, e := json.Marshal(res)
		if e != nil {
			t.Fatal(e)
		}
		return string(b)
	}
	first := call("read_post_context", map[string]any{"project_id": "p1"})
	if first.IsError || !strings.Contains(content(first), "First update") {
		t.Fatal("first context missing", content(first))
	}
	foreign := call("read_post_context", map[string]any{"project_id": "p2"})
	if !foreign.IsError || strings.Contains(content(foreign), "First update") {
		t.Fatal("context leaked", content(foreign))
	}
	writeContext("Second update")
	second := call("read_post_context", map[string]any{"project_id": "p1"})
	if second.IsError || !strings.Contains(content(second), "Second update") || strings.Contains(content(second), "First update") {
		t.Fatal("context not replaced", content(second))
	}
	created := call("create_post_draft", map[string]any{"project_id": "p1", "platform": "X", "text": "Initial draft"})
	if created.IsError {
		t.Fatal(content(created))
	}
	_, state, err := read(ctx, store)
	if err != nil {
		t.Fatal(err)
	}
	id := state.Drafts[0].ID
	updated := call("update_post_draft", map[string]any{"id": id, "expected_text": "Initial draft", "text": "Updated draft"})
	if updated.IsError {
		t.Fatal(content(updated))
	}
	stale := call("update_post_draft", map[string]any{"id": id, "expected_text": "Initial draft", "text": "Overwrite"})
	if !stale.IsError {
		t.Fatal("stale edit accepted")
	}
	filtered := call("read_project_drafts", map[string]any{"project_id": "p2"})
	if strings.Contains(content(filtered), "Updated draft") {
		t.Fatal("draft leaked", content(filtered))
	}
	session.Close()
	serverSession.Close()
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = workspace.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	_, state, err = read(ctx, store)
	if err != nil || state.Drafts[0].Text != "Updated draft" || len(state.Contexts) != 1 || state.Contexts[0].Commits[0].Title != "Second update" {
		t.Fatal("restart lost data", err)
	}
}
