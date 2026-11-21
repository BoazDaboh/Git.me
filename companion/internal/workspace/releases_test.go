package workspace

import (
	"encoding/json"
	"gitme/companion/internal/releases"
	"gitme/companion/internal/vercel"
	"path/filepath"
	"strings"
	"testing"
)

func TestReleaseRecordsPersistWithoutChangingWorkspace(t *testing.T) {
	s := openTest(t)
	before, _ := s.Load(t.Context())
	if _, err := s.Save(t.Context(), before.DatabaseID, 0, json.RawMessage(sample)); err != nil {
		t.Fatal(err)
	}
	before, _ = s.Load(t.Context())
	target := &vercel.Link{ConnectionID: "account_one", ProjectID: "prj_demo", ProjectName: "demo", ScopeSlug: "fixture", TeamID: "team_one"}
	record := releases.Record{ProjectID: "p1", ServiceID: "s1", Repository: "https://github.com/fixture/repo.git", Branch: "main", Commit: strings.Repeat("a", 40), Source: "observed", RemoteVerified: true, Target: target}
	first, err := s.RecordRelease(t.Context(), record)
	if err != nil {
		t.Fatal(err)
	}
	record.Source = "push"
	promoted, err := s.RecordRelease(t.Context(), record)
	if err != nil || first.ID != promoted.ID || !first.CreatedAt.Equal(promoted.CreatedAt) {
		t.Fatal("duplicate/promotion", err)
	}
	record.Source = "observed"
	_, err = s.RecordRelease(t.Context(), record)
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.Releases(t.Context(), "p1", "s1")
	if err != nil || len(got) != 1 || got[0].Source != "push" {
		t.Fatal("record was duplicated or downgraded", err)
	}
	other, err := s.Releases(t.Context(), "p1", "s2")
	if err != nil || len(other) != 0 {
		t.Fatal("service history crossed scope")
	}
	after, _ := s.Load(t.Context())
	if after.Revision != before.Revision {
		t.Fatal("tracking changed workspace revision")
	}
	equalJSON(t, before.Workspace, after.Workspace)
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(s.Path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	got, err = reopened.Releases(t.Context(), "p1", "s1")
	if err != nil || len(got) != 1 || got[0].Commit != record.Commit {
		t.Fatal("history lost after restart", err)
	}
	bad := record
	bad.Repository = "https://token@github.com/fixture/repo"
	if _, err = reopened.RecordRelease(t.Context(), bad); err == nil {
		t.Fatal("credentialed source accepted")
	}
}
func TestVersionOneMigrationPreservesNotes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.sqlite")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, _ := s.Load(t.Context())
	if _, err = s.Save(t.Context(), snapshot.DatabaseID, 0, json.RawMessage(sample)); err != nil {
		t.Fatal(err)
	}
	if _, err = s.db.Exec("DROP TABLE release_records"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.db.Exec("PRAGMA user_version=1"); err != nil {
		t.Fatal(err)
	}
	s.Close()
	migrated, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer migrated.Close()
	after, err := migrated.Load(t.Context())
	if err != nil || after.Revision != 1 {
		t.Fatal("migration changed notes revision", err)
	}
	equalJSON(t, after.Workspace, []byte(sample))
	if _, err = migrated.Releases(t.Context(), "p1", "s1"); err != nil {
		t.Fatal(err)
	}
}
