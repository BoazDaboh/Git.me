package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
)

const sample = `{"version":1,"projects":[{"id":"p1","name":"GitMe","initials":"g/","color":"blue","description":"Local","stage":"Building","branch":"main","folder":"C:\\projects\\gitme","url":"","local":"http://127.0.0.1:3015","note":"Keep details","stack":["Go"],"progress":10,"services":[{"id":"s1","name":"API","role":"backend","stack":["Go"],"runtime":"Go","folder":"","repository":"","branch":"main","port":8080,"healthPath":"/health","commands":{"install":"go mod download","develop":"go run ./cmd/api","build":"go build ./cmd/api","test":"go test ./...","start":"./api"},"deployment":{"provider":"","method":"native","build":"","start":"","dockerfile":"Dockerfile","image":""},"environments":{"local":{"url":"http://127.0.0.1:8080","variables":["PORT"]},"preview":{"url":"","variables":[]},"production":{"url":"","variables":[]}},"updatedAt":100,"custom":"preserve nested fields"}]}],"tasks":[{"id":"t1","title":"Save notes","project":"p1","status":"Planned","priority":"High"}],"notes":[{"id":"n1","title":"A note","body":"- [x] Saved\nỌmọ","kind":"note","project":"p1","pinned":true,"updatedAt":100}],"sessions":[{"project":"p1","started":100,"ended":200}],"drafts":[{"id":"d1","platform":"X","project":"p1","text":"Hello"}],"focus":{"project":"p1","started":300},"document":{"title":"Doc","text":"Content"},"projectDirectory":"C:\\projects","collapsedTaskGroups":["Planned:p1"]}`

func openTest(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "data", "gitme.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func equalJSON(t *testing.T, left, right []byte) {
	t.Helper()
	var a, b any
	if err := json.Unmarshal(left, &a); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(right, &b); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(a, b) {
		t.Fatalf("workspace changed: %s", left)
	}
}

func TestWorkspaceNameRoundTrip(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)
	empty, err := s.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var data map[string]json.RawMessage
	if err := json.Unmarshal([]byte(sample), &data); err != nil {
		t.Fatal(err)
	}
	data["workspaceName"] = json.RawMessage(`"Ada's workspace"`)
	raw, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Save(ctx, empty.DatabaseID, 0, raw); err != nil {
		t.Fatal(err)
	}
	saved, err := s.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	equalJSON(t, saved.Workspace, raw)
}

func TestPersistenceAcrossRestart(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)
	empty, err := s.Load(ctx)
	if err != nil || empty.Revision != 0 || string(empty.Workspace) != "null" || empty.DatabaseID == "" {
		t.Fatalf("empty: %+v %v", empty, err)
	}
	if revision, err := s.Save(ctx, empty.DatabaseID, 0, json.RawMessage(sample)); err != nil || revision != 1 {
		t.Fatalf("save: %d %v", revision, err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(s.Path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	saved, err := reopened.Load(ctx)
	if err != nil || saved.Revision != 1 || saved.DatabaseID != empty.DatabaseID || saved.UpdatedAt == "" {
		t.Fatalf("reload: %+v %v", saved, err)
	}
	equalJSON(t, saved.Workspace, []byte(sample))
	var count int
	if err := reopened.db.QueryRow("SELECT count(*) FROM notes").Scan(&count); err != nil || count != 1 {
		t.Fatalf("notes: %d %v", count, err)
	}
}

func TestConcurrentWritersAndDatabaseIdentity(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	initial, _ := s.Load(ctx)
	if _, err := s.Save(ctx, "different-database", 0, json.RawMessage(sample)); !errors.Is(err, ErrConflict) {
		t.Fatalf("wrong database: %v", err)
	}
	var group sync.WaitGroup
	results := make(chan error, 2)
	for range 2 {
		group.Add(1)
		go func() {
			defer group.Done()
			_, err := s.Save(ctx, initial.DatabaseID, 0, json.RawMessage(sample))
			results <- err
		}()
	}
	group.Wait()
	close(results)
	success, conflicts := 0, 0
	for err := range results {
		if err == nil {
			success++
		} else if errors.Is(err, ErrConflict) {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || conflicts != 1 {
		t.Fatalf("success=%d conflicts=%d", success, conflicts)
	}
}

func TestInvalidDataAndFailedTransactionKeepSavedCopy(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	initial, _ := s.Load(ctx)
	if _, err := s.Save(ctx, initial.DatabaseID, 0, json.RawMessage(sample)); err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{`{}`, strings.Replace(sample, `"body":"- [x] Saved\nỌmọ"`, `"body":null`, 1), strings.Replace(sample, `"notes":[`, `"notes":[null,`, 1), strings.Replace(sample, `"status":"Planned"`, `"status":"Wrong"`, 1)} {
		if _, err := s.Save(ctx, initial.DatabaseID, 1, json.RawMessage(raw)); err == nil {
			t.Fatalf("accepted invalid workspace: %s", raw)
		}
	}
	if _, err := s.db.Exec(`CREATE TRIGGER fail_notes BEFORE INSERT ON notes BEGIN SELECT RAISE(ABORT, 'test failure'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Save(ctx, initial.DatabaseID, 1, json.RawMessage(strings.Replace(sample, "Save notes", "Changed task", 1))); err == nil {
		t.Fatal("expected transaction failure")
	}
	saved, err := s.Load(ctx)
	if err != nil || saved.Revision != 1 {
		t.Fatalf("revision changed after failure: %d %v", saved.Revision, err)
	}
	equalJSON(t, saved.Workspace, []byte(sample))
}

func TestDeletionAndEmptyCollections(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	initial, _ := s.Load(ctx)
	_, err := s.Save(ctx, initial.DatabaseID, 0, json.RawMessage(sample))
	if err != nil {
		t.Fatal(err)
	}
	data, _ := Validate(json.RawMessage(sample))
	for _, table := range tables {
		data[table] = json.RawMessage("[]")
	}
	data["focus"] = json.RawMessage("null")
	raw, _ := json.Marshal(data)
	_, err = s.Save(ctx, initial.DatabaseID, 1, raw)
	if err != nil {
		t.Fatal(err)
	}
	saved, err := s.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	equalJSON(t, saved.Workspace, raw)
}

func TestDataDirectoryOverride(t *testing.T) {
	directory := t.TempDir()
	t.Setenv("GITME_DATA_DIR", directory)
	path, err := DefaultPath()
	if err != nil || path != filepath.Join(directory, "gitme.sqlite") {
		t.Fatalf("path=%s %v", path, err)
	}
}
