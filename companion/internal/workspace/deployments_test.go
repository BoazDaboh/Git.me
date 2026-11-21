package workspace

import (
	"encoding/json"
	"gitme/companion/internal/releases"
	"gitme/companion/internal/vercel"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDeploymentJournalSurvivesInterruptionAndSchemaTwoMigration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.sqlite")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, _ := s.Load(t.Context())
	if _, err = s.Save(t.Context(), snapshot.DatabaseID, 0, json.RawMessage(sample)); err != nil {
		t.Fatal(err)
	}
	if _, err = s.db.Exec("DROP TABLE deployment_actions"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.db.Exec("PRAGMA user_version=2"); err != nil {
		t.Fatal(err)
	}
	s.Close()
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	before, err := s.Load(t.Context())
	if err != nil || before.Revision != 1 {
		t.Fatal("workspace changed during migration", err)
	}
	equalJSON(t, before.Workspace, []byte(sample))
	link := &vercel.Link{ConnectionID: "account_one", TeamID: "team_one", ProjectID: "prj_demo", ProjectName: "demo", ScopeSlug: "fixture"}
	action := releases.DeploymentAction{ID: "fixture_request", Release: releases.Record{ProjectID: "p1", ServiceID: "s1", Repository: "https://github.com/fixture/repo", Branch: "main", Commit: strings.Repeat("a", 40), Source: "observed", Target: link}, Environment: "production", CreatedAt: time.Now().UTC(), Result: vercel.DeploymentResult{Outcome: "submitting", Message: "Confirmation unavailable. Check Vercel before retrying."}}
	if err = s.BeginDeployment(t.Context(), action); err != nil {
		t.Fatal(err)
	}
	if err = s.BeginDeployment(t.Context(), action); err == nil {
		t.Fatal("duplicate journal ID accepted")
	}
	s.Close()
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	actions, err := s.DeploymentActions(t.Context(), "p1", "s1")
	if err != nil || len(actions) != 1 || actions[0].Result.Outcome != "submitting" {
		t.Fatal("interrupted request disappeared", err)
	}
	action.Result = vercel.DeploymentResult{Outcome: "unconfirmed", Message: "Check Vercel before retrying."}
	if err = s.FinishDeployment(t.Context(), action); err != nil {
		t.Fatal(err)
	}
	action.Result.Outcome = "accepted"
	if err = s.FinishDeployment(t.Context(), action); err == nil {
		t.Fatal("finished journal rewritten")
	}
	after, _ := s.Load(t.Context())
	if after.Revision != before.Revision {
		t.Fatal("journal changed workspace revision")
	}
	equalJSON(t, before.Workspace, after.Workspace)
}
