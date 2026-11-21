package workspace

import (
	"encoding/json"
	"testing"
)

func legacyServiceWorkspace(t *testing.T) map[string]any {
	t.Helper()
	var data map[string]any
	if json.Unmarshal([]byte(sample), &data) != nil {
		t.Fatal("fixture parse")
	}
	return data
}
func TestProjectRepositoryMigrationPreservesServiceAccountsAndUnmatchedLinks(t *testing.T) {
	for _, test := range []struct {
		name        string
		frontRepo   string
		existing    bool
		wantPending bool
	}{
		{"matching", "git@github.com:owner/repo.git", false, false},
		{"different repository", "https://github.com/owner/another", false, true},
		{"explicit account wins", "https://github.com/owner/repo", true, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			data := legacyServiceWorkspace(t)
			project := data["projects"].([]any)[0].(map[string]any)
			service := project["services"].([]any)[0].(map[string]any)
			link := map[string]any{"id": float64(42), "fullName": "owner/repo", "connectionId": "123"}
			project["github"] = link
			service["repository"] = test.frontRepo
			if test.existing {
				service["github"] = map[string]any{"id": float64(42), "fullName": "owner/repo", "connectionId": "456"}
			}
			raw, _ := json.Marshal(data)
			migrated, changed, err := migrateProjectGitHub(raw)
			if err != nil || !changed {
				t.Fatal("not migrated", err)
			}
			var got map[string]any
			json.Unmarshal(migrated, &got)
			p := got["projects"].([]any)[0].(map[string]any)
			s := p["services"].([]any)[0].(map[string]any)
			if _, exists := p["github"]; exists {
				t.Fatal("project binding retained")
			}
			_, pending := p["unassignedGitHubLinks"]
			if pending != test.wantPending {
				t.Fatal("unmatched link lost or copied unnecessarily")
			}
			if test.existing {
				if s["github"].(map[string]any)["connectionId"] != "456" {
					t.Fatal("explicit service account overwritten")
				}
			} else if !test.wantPending {
				if s["github"].(map[string]any)["connectionId"] != "123" {
					t.Fatal("original account lost")
				}
			} else if _, exists := s["github"]; exists {
				t.Fatal("different repository inherited account")
			}
			old, _ := json.Marshal(data["notes"])
			after, _ := json.Marshal(got["notes"])
			equalJSON(t, old, after)
			again, second, err := migrateProjectGitHub(migrated)
			if err != nil || second {
				t.Fatal("migration not idempotent", err)
			}
			equalJSON(t, migrated, again)
		})
	}
}
func TestExistingDatabaseProjectsAreProjectedThenSavedWithoutConflicts(t *testing.T) {
	store := openTest(t)
	initial, _ := store.Load(t.Context())
	if _, err := store.Save(t.Context(), initial.DatabaseID, 0, json.RawMessage(sample)); err != nil {
		t.Fatal(err)
	}
	data := legacyServiceWorkspace(t)
	project := data["projects"].([]any)[0].(map[string]any)
	service := project["services"].([]any)[0].(map[string]any)
	project["github"] = map[string]any{"id": 42, "fullName": "owner/repo"}
	service["repository"] = "https://github.com/owner/repo.git"
	raw, _ := json.Marshal(project)
	if _, err := store.db.Exec("UPDATE projects SET payload=? WHERE id='p1'", string(raw)); err != nil {
		t.Fatal(err)
	}
	projected, err := store.Load(t.Context())
	if err != nil || !projected.ServiceLinkMigration || projected.Revision != 1 {
		t.Fatal("legacy database projection", err)
	}
	var got map[string]any
	json.Unmarshal(projected.Workspace, &got)
	p := got["projects"].([]any)[0].(map[string]any)
	s := p["services"].([]any)[0].(map[string]any)
	if _, exists := p["github"]; exists || s["github"] == nil {
		t.Fatal("service link not projected")
	}
	if _, err = store.Save(t.Context(), projected.DatabaseID, projected.Revision, projected.Workspace); err != nil {
		t.Fatal("canonical save conflicted", err)
	}
	saved, err := store.Load(t.Context())
	if err != nil || saved.ServiceLinkMigration || saved.Revision != 2 {
		t.Fatal("migration did not persist", err)
	}
	var payload string
	store.db.QueryRow("SELECT payload FROM projects WHERE id='p1'").Scan(&payload)
	var persisted map[string]any
	json.Unmarshal([]byte(payload), &persisted)
	if _, exists := persisted["github"]; exists {
		t.Fatal("old project link remained on disk")
	}
}
func TestLegacyImportsAndNoServiceLinksAreRetained(t *testing.T) {
	store := openTest(t)
	snapshot, _ := store.Load(t.Context())
	data := legacyServiceWorkspace(t)
	project := data["projects"].([]any)[0].(map[string]any)
	project["github"] = map[string]any{"id": 42, "fullName": "owner/repo"}
	project["services"] = []any{}
	raw, _ := json.Marshal(data)
	if _, err := store.Save(t.Context(), snapshot.DatabaseID, 0, raw); err != nil {
		t.Fatal(err)
	}
	saved, err := store.Load(t.Context())
	if err != nil || saved.ServiceLinkMigration {
		t.Fatal(err)
	}
	var got map[string]any
	json.Unmarshal(saved.Workspace, &got)
	p := got["projects"].([]any)[0].(map[string]any)
	pending := p["unassignedGitHubLinks"].([]any)
	if len(pending) != 1 || pending[0].(map[string]any)["fullName"] != "owner/repo" {
		t.Fatal("unmatched imported link lost")
	}
	p["unassignedGitHubLinks"] = []any{map[string]any{"id": 42, "fullName": "owner/repo", "token": "PRIVATE_SENTINEL"}}
	bad, _ := json.Marshal(got)
	if _, err = store.Save(t.Context(), saved.DatabaseID, saved.Revision, bad); err == nil {
		t.Fatal("secret-bearing saved link accepted")
	}
}
