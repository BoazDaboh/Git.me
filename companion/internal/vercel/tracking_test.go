package vercel

import (
	"encoding/json"
	"gitme/companion/internal/credentials"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestTrackingMatchesExactCommitBranchProjectAndAccount(t *testing.T) {
	sha := strings.Repeat("a", 40)
	index := &memoryVault{}
	vault := &memoryVault{}
	calls := 0
	mode := "BUILDING"
	badRepo := false
	service := New(index, func(string) credentials.Vault { return vault }, &http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
		if r.Method != "GET" || r.URL.Host != "api.vercel.com" || r.URL.Scheme != "https" {
			t.Fatal("tracking sent a write or credentials to another host")
		}
		calls++
		var body any
		switch r.URL.Path {
		case "/v2/user":
			body = map[string]any{"user": Account{ID: "account_one", Username: "fixture"}}
		case "/v9/projects/prj_demo":
			org := "fixture"
			if badRepo {
				org = "another"
			}
			body = map[string]any{"id": "prj_demo", "name": "demo", "accountId": "team_one", "link": map[string]string{"type": "github", "org": org, "repo": "repo"}, "targets": map[string]any{"production": map[string]string{"id": "dpl_match"}}}
		case "/v7/deployments":
			if r.URL.Query().Get("projectId") != "prj_demo" || r.URL.Query().Get("sha") != sha || r.URL.Query().Get("branch") != "main" || r.URL.Query().Get("teamId") != "team_one" {
				t.Fatal("unscoped tracking query")
			}
			item := func(id, state, commit, branch, repo string) any {
				return map[string]any{"uid": id, "state": state, "target": "production", "url": "fixture.vercel.app", "created": int64(1000), "meta": map[string]string{"githubCommitSha": commit, "githubCommitRef": branch, "githubCommitOrg": "fixture", "githubCommitRepo": repo, "secret": "PRIVATE_SENTINEL"}}
			}
			deployments := []any{}
			if mode != "none" {
				deployments = []any{item("dpl_match", mode, sha, "main", "repo"), item("dpl_wrong_sha", "READY", strings.Repeat("b", 40), "main", "repo"), item("dpl_wrong_branch", "READY", sha, "other", "repo"), item("dpl_wrong_repo", "READY", sha, "main", "other")}
			}
			body = map[string]any{"deployments": deployments}
		default:
			t.Fatal("unexpected tracking endpoint", r.URL.Path)
		}
		raw, _ := json.Marshal(body)
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(string(raw)))}, nil
	})})
	if _, err := service.Connect(t.Context(), "fixture_vercel_account_one_123", ""); err != nil {
		t.Fatal(err)
	}
	link := Link{ConnectionID: "account_one", ProjectID: "prj_demo", ProjectName: "demo", TeamID: "team_one", ScopeSlug: "fixture"}
	expire := func() {
		service.mu.Lock()
		for key, v := range service.tracking {
			v.CheckedAt = time.Now().Add(-11 * time.Second)
			service.tracking[key] = v
		}
		service.mu.Unlock()
	}
	for _, state := range []string{"BUILDING", "READY", "ERROR", "CANCELED", "BLOCKED", "none"} {
		mode = state
		expire()
		result, err := service.TrackCommit(t.Context(), link, "fixture/repo", "main", sha)
		if err != nil {
			t.Fatal(err)
		}
		if state == "none" {
			if len(result.Deployments) != 0 {
				t.Fatal("missing deployment fabricated")
			}
		} else if len(result.Deployments) != 1 || result.Deployments[0].ID != "dpl_match" || result.Deployments[0].State != state {
			t.Fatal("wrong commit/branch/repo matched")
		}
		raw, _ := json.Marshal(result)
		if strings.Contains(string(raw), "PRIVATE_SENTINEL") {
			t.Fatal("raw deployment metadata leaked")
		}
	}
	before := calls
	if _, err := service.TrackCommit(t.Context(), link, "fixture/repo", "main", sha); err != nil || calls != before {
		t.Fatal("tracking ignored cache interval", err)
	}
	expire()
	badRepo = true
	if _, err := service.TrackCommit(t.Context(), link, "fixture/repo", "main", sha); err == nil {
		t.Fatal("project Git repository mismatch accepted")
	}
	badRepo = false
	wrong := link
	wrong.ConnectionID = "disconnected"
	if _, err := service.TrackCommit(t.Context(), wrong, "fixture/repo", "main", sha); err == nil {
		t.Fatal("account fallback")
	}
	if err := service.Disconnect(link.ConnectionID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.TrackCommit(t.Context(), link, "fixture/repo", "main", sha); err == nil {
		t.Fatal("disconnected account served private cached readings")
	}
}
