package main

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gitme/companion/internal/credentials"
	"gitme/companion/internal/processes"
	"gitme/companion/internal/vercel"
	"gitme/companion/internal/workspace"
)

const vercelFixtureToken = "vercel_fixture_readonly_account_one"
const vercelSecondToken = "vercel_fixture_readonly_account_two"
const vercelSecretSentinel = "private_provider_value_do_not_return"

func fixtureVercel(t *testing.T) *vercel.Service {
	return fixtureVercelMode(t, false)
}
func fixtureVercelMode(t *testing.T, allowDeploy bool) *vercel.Service {
	t.Helper()
	vaults := map[string]*fixtureVault{}
	client := &http.Client{Transport: fixtureTransport(func(r *http.Request) (*http.Response, error) {
		if (r.Method != "GET" && !(allowDeploy && r.Method == "POST" && (r.URL.Path == "/v13/deployments" || r.URL.Path == "/v2/files"))) || r.URL.Host != "api.vercel.com" || r.URL.Scheme != "https" {
			t.Error("Vercel fixture received external write or unexpected destination")
		}
		account, username, team := "vercel_user_one", "fixture-vercel", "team_one"
		status := 200
		header := r.Header.Get("Authorization")
		if header == "Bearer "+vercelSecondToken {
			account, username, team = "vercel_user_two", "fixture-vercel-two", "team_two"
		} else if header != "Bearer "+vercelFixtureToken {
			status = 401
		}
		var result any = map[string]string{"message": vercelSecretSentinel}
		q := r.URL.Query()
		if q.Get("teamId") != "" && q.Get("teamId") != team && q.Get("teamId") != team+"_extra" {
			status = 403
		}
		if status == 200 {
			owner := account
			if q.Get("teamId") != "" {
				owner = q.Get("teamId")
			}
			project := func(id, name string) map[string]any {
				value := map[string]any{"id": id, "name": name, "accountId": owner, "framework": "nextjs", "env": []any{map[string]string{"value": vercelSecretSentinel}}, "targets": map[string]any{"production": map[string]any{"id": "dpl_current", "readyState": "READY", "url": "fixture-current.vercel.app", "createdAt": 1795000000000}}}
				if allowDeploy {
					value["link"] = map[string]string{"type": "github", "org": "fixture-user", "repo": "private-repo"}
				}
				return value
			}
			switch {
			case r.URL.Path == "/v2/files" && allowDeploy:
				raw, err := io.ReadAll(io.LimitReader(r.Body, 8<<20+1))
				sum := sha1.Sum(raw)
				if err != nil || len(raw) > 8<<20 || r.Header.Get("x-vercel-digest") != hex.EncodeToString(sum[:]) || r.ContentLength != int64(len(raw)) || q.Get("teamId") != "team_one" {
					t.Error("invalid file upload")
					status = 400
				}
				if directory := os.Getenv("GITME_BROWSER_TEST_DIR"); directory != "" {
					f, e := os.OpenFile(filepath.Join(directory, "upload-writes.ndjson"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
					if e != nil {
						t.Error(e)
					} else {
						metadata, _ := json.Marshal(map[string]any{"sha": hex.EncodeToString(sum[:]), "bytes": len(raw)})
						f.Write(append(metadata, '\n'))
						f.Close()
					}
				}
				result = map[string]any{}
			case r.URL.Path == "/v13/deployments/dpl_manual":
				result = map[string]any{"id": "dpl_manual", "projectId": "prj_example", "readyState": "READY", "url": "fixture-manual.vercel.app", "env": vercelSecretSentinel}
			case r.URL.Path == "/v13/deployments" && allowDeploy:
				var payload struct {
					Project   string            `json:"project"`
					Target    string            `json:"target"`
					GitSource map[string]string `json:"gitSource"`
					Files     []struct {
						File string `json:"file"`
						SHA  string `json:"sha"`
						Size int    `json:"size"`
					} `json:"files"`
				}
				if json.NewDecoder(r.Body).Decode(&payload) != nil || payload.Project != "prj_example" || (len(payload.Files) == 0 && (payload.GitSource["sha"] != strings.Repeat("a", 40) || payload.GitSource["ref"] != "main")) || len(payload.Files) > 0 && len(payload.GitSource) > 0 || q.Get("teamId") != "team_one" || q.Get("forceNew") != "1" {
					t.Error("unexpected mock deployment source or destination")
					status = 400
				}
				if directory := os.Getenv("GITME_BROWSER_TEST_DIR"); directory != "" {
					raw, _ := json.Marshal(payload)
					f, err := os.OpenFile(filepath.Join(directory, "deployment-writes.ndjson"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
					if err != nil {
						t.Error(err)
					} else {
						_, _ = f.Write(append(raw, '\n'))
						_ = f.Close()
					}
					if mode, err := os.ReadFile(filepath.Join(directory, "deployment-response.txt")); err == nil && strings.TrimSpace(string(mode)) == "rejected" {
						status = 403
					}
				}
				result = map[string]any{"id": "dpl_manual", "projectId": "prj_example", "readyState": "QUEUED", "url": "fixture-manual.vercel.app", "env": vercelSecretSentinel}
			case r.URL.Path == "/v2/user":
				result = map[string]any{"user": map[string]string{"id": account, "username": username, "email": vercelSecretSentinel}}
			case r.URL.Path == "/v2/teams":
				teams := []any{map[string]string{"id": team, "name": "Fixture team", "slug": team}}
				var next any = int64(1)
				if q.Get("until") == "1" {
					teams = []any{map[string]string{"id": team + "_extra", "name": "Another team", "slug": team + "_extra"}}
					next = nil
				}
				result = map[string]any{"teams": teams, "pagination": map[string]any{"next": next}}
			case r.URL.Path == "/v10/projects":
				projects := []any{project("prj_example", "example-web"), project("prj_api", "example-api")}
				var next any = int64(1)
				if q.Get("teamId") == team+"_extra" {
					next = "NEXT_PAGE"
				}
				if q.Get("until") == "1" || q.Get("from") == "NEXT_PAGE" {
					projects = []any{project("prj_older", "older-project")}
					next = nil
				}
				if q.Get("search") != "" {
					projects = []any{project("prj_search", "searched-project")}
					next = nil
				}
				result = map[string]any{"projects": projects, "pagination": map[string]any{"next": next}}
			case strings.HasPrefix(r.URL.Path, "/v9/projects/"):
				id := strings.TrimPrefix(r.URL.Path, "/v9/projects/")
				name := "example-web"
				if id == "prj_api" {
					name = "example-api"
				}
				result = project(id, name)
			case r.URL.Path == "/v7/deployments":
				if q.Get("sha") != "" {
					state := "BUILDING"
					if directory := os.Getenv("GITME_BROWSER_TEST_DIR"); directory != "" {
						if raw, err := os.ReadFile(filepath.Join(directory, "tracking-state.json")); err == nil {
							var setting struct {
								State string `json:"state"`
							}
							if json.Unmarshal(raw, &setting) == nil {
								state = setting.State
							}
						}
					}
					deployments := []any{}
					if state != "none" && q.Get("sha") == strings.Repeat("a", 40) {
						deployments = append(deployments, map[string]any{"uid": "dpl_current", "state": state, "target": "production", "url": "fixture-current.vercel.app", "created": time.Now().UnixMilli(), "meta": map[string]string{"githubCommitRef": q.Get("branch"), "githubCommitSha": q.Get("sha"), "githubCommitOrg": "fixture-user", "githubCommitRepo": "private-repo", "private": vercelSecretSentinel}})
					}
					result = map[string]any{"deployments": deployments}
					break
				}
				result = map[string]any{"deployments": []any{
					map[string]any{"uid": "dpl_failed", "state": "ERROR", "target": "production", "url": "fixture-failed.vercel.app", "created": 1795000100000, "meta": map[string]string{"githubCommitRef": "main", "githubCommitSha": strings.Repeat("a", 40), "private": vercelSecretSentinel}},
					map[string]any{"uid": "dpl_building", "state": "BUILDING", "url": nil, "created": 1795000050000},
					map[string]any{"uid": "dpl_preview", "state": "READY", "target": "preview", "url": "fixture-preview.vercel.app", "created": 1795000000000},
					map[string]any{"uid": "dpl_untrusted", "state": "READY", "url": "https://evil.example/?secret=" + vercelSecretSentinel, "created": 1795000000000},
				}}
			default:
				status = 404
			}
		}
		raw, _ := json.Marshal(result)
		return &http.Response{StatusCode: status, Header: http.Header{}, Body: io.NopCloser(bytes.NewReader(raw))}, nil
	})}
	return vercel.New(&fixtureVault{}, func(id string) credentials.Vault {
		if vaults[id] == nil {
			vaults[id] = &fixtureVault{}
		}
		return vaults[id]
	}, client)
}

func TestVercelRoutesReadOnlyScopeAndSecretIsolation(t *testing.T) {
	store, err := workspace.Open(filepath.Join(t.TempDir(), "test.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	manager := processes.New(store)
	defer manager.Close()
	cloud := fixtureVercel(t)
	handler := newHandlerWithCloud(time.Now(), store, manager, fixtureGitHub(t), cloud)
	defer handler.(*gitHandler).Close()
	call := func(method, path, body, origin string, client bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "http://"+address+path, strings.NewReader(body))
		r.Header.Set("Origin", origin)
		r.Header.Set("Content-Type", "application/json")
		if client {
			r.Header.Set("X-GitMe-Client", "workspace-v1")
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	input := `{"token":"` + vercelFixtureToken + `"}`
	if w := call("POST", "/vercel/connect", input, "https://evil.example", true); w.Code != 403 {
		t.Fatal("foreign website could save a token")
	}
	if w := call("POST", "/vercel/connect", input, "", false); w.Code != 415 {
		t.Fatal("missing client header accepted")
	}
	if w := call("POST", "/vercel/connect", input, "http://127.0.0.1:3015", true); w.Code != 200 || strings.Contains(w.Body.String(), vercelFixtureToken) {
		t.Fatal("connect failed or exposed credential")
	}
	if w := call("POST", "/vercel/connect", `{"token":"`+vercelSecondToken+`"}`, "", true); w.Code != 200 {
		t.Fatal("second account failed")
	}
	for _, path := range []string{"/vercel/connections", "/vercel/teams?connection_id=vercel_user_one", "/vercel/projects?connection_id=vercel_user_one&team_id=team_one", "/vercel/project?connection_id=vercel_user_one&team_id=team_one&name=prj_example"} {
		w := call("GET", path, "", "", false)
		if w.Code != 200 || strings.Contains(w.Body.String(), vercelFixtureToken) || strings.Contains(w.Body.String(), vercelSecretSentinel) {
			t.Fatal("safe read failed or returned private provider fields", path, w.Code)
		}
	}
	if w := call("GET", "/vercel/projects?connection_id=vercel_user_two&team_id=team_one", "", "", false); w.Code != 422 {
		t.Fatal("project read crossed account scope")
	}
	if w := call("POST", "/vercel/connect", `{"token":"`+vercelSecondToken+`","connection_id":"vercel_user_one"}`, "", true); w.Code != 422 {
		t.Fatal("cross-account token replacement accepted")
	}
	link := vercel.Link{ConnectionID: "vercel_user_one", TeamID: "team_one", ProjectID: "prj_example", ProjectName: "example-web", ScopeSlug: "team_one"}
	activity, err := cloud.Activity(t.Context(), link, false)
	if err != nil || activity.Production == nil || activity.Production.State != "READY" || activity.Deployments[0].State != "ERROR" {
		t.Fatal("latest failed build confused with active production", err)
	}
	if activity.Deployments[3].URL != "" {
		t.Fatal("untrusted URL accepted")
	}
	raw, _ := json.Marshal(activity)
	if strings.Contains(string(raw), vercelSecretSentinel) {
		t.Fatal("private deployment metadata returned")
	}
	if w := call("POST", "/vercel/disconnect", `{"connection_id":"vercel_user_two"}`, "", true); w.Code != 200 {
		t.Fatal("disconnect failed")
	}
	if w := call("GET", "/vercel/projects?connection_id=vercel_user_two&team_id=team_two", "", "", false); w.Code != 422 {
		t.Fatal("disconnected account fell back to another account")
	}
	if w := call("GET", "/vercel/projects?connection_id=vercel_user_one&team_id=team_one", "", "", false); w.Code != 200 {
		t.Fatal("disconnect affected another account")
	}
	snapshot, err := store.Load(t.Context())
	if err != nil || snapshot.Revision != 0 || strings.Contains(string(snapshot.Workspace), vercelFixtureToken) {
		t.Fatal("connection modified workspace or stored a credential there")
	}
}
