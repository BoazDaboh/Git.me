package main

import (
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gitme/companion/internal/processes"
	"gitme/companion/internal/releases"
	"gitme/companion/internal/vercel"
	"gitme/companion/internal/workspace"
)

func TestDeploymentActionsRequireReviewPersistAndStayScoped(t *testing.T) {
	store, err := workspace.Open(filepath.Join(t.TempDir(), "test.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	target := &vercel.Link{ConnectionID: "vercel_user_one", TeamID: "team_one", ProjectID: "prj_example", ProjectName: "example-web", ScopeSlug: "team_one"}
	service := map[string]any{"id": "s1", "name": "Web", "role": "frontend", "folder": "C:\\fixture", "repository": "https://github.com/fixture-user/private-repo", "branch": "main", "stack": []string{}, "runtime": "Node.js", "port": nil, "healthPath": "", "updatedAt": 1, "commands": map[string]string{"install": "", "develop": "", "build": "", "test": "", "start": ""}, "deployment": map[string]string{"provider": "Vercel", "method": "managed", "build": "", "start": "", "dockerfile": "", "image": ""}, "environments": map[string]any{"local": map[string]any{"url": "", "variables": []string{}}, "preview": map[string]any{"url": "", "variables": []string{}}, "production": map[string]any{"url": "", "variables": []string{}}}, "vercel": target}
	project := map[string]any{"id": "p1", "name": "Fixture", "stack": []string{}, "description": "", "url": "", "local": "", "note": "", "initials": "F", "color": "blue", "stage": "Building", "branch": "main", "folder": "C:\\fixture", "progress": 0, "services": []any{service}}
	data := map[string]any{"version": 1, "projects": []any{project}, "tasks": []any{}, "notes": []any{}, "sessions": []any{}, "drafts": []any{}, "focus": nil, "document": map[string]string{"title": "", "text": ""}}
	save := func() {
		t.Helper()
		s, _ := store.Load(t.Context())
		raw, _ := json.Marshal(data)
		if _, err := store.Save(t.Context(), s.DatabaseID, s.Revision, raw); err != nil {
			t.Fatal(err)
		}
	}
	save()
	record, err := store.RecordRelease(t.Context(), releases.Record{ProjectID: "p1", ServiceID: "s1", Repository: "https://github.com/fixture-user/private-repo", Branch: "main", Commit: strings.Repeat("a", 40), Source: "observed", RemoteVerified: true, Target: target})
	if err != nil {
		t.Fatal(err)
	}
	manager := processes.New(store)
	defer manager.Close()
	cloud := fixtureVercelMode(t, true)
	if _, err := cloud.Connect(t.Context(), vercelFixtureToken, ""); err != nil {
		t.Fatal(err)
	}
	handler := newHandlerWithCloud(time.Now(), store, manager, fixtureGitHub(t), cloud)
	defer handler.(*gitHandler).Close()
	call := func(method, path, body, origin string, client bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "http://"+address+path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		if client {
			r.Header.Set("X-GitMe-Client", "workspace-v1")
		}
		r.Header.Set("Origin", origin)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	input := `{"project_id":"p1","service_id":"s1","release_id":"` + record.ID + `","environment":"preview"}`
	for _, test := range []struct {
		body, origin string
		client       bool
		status       int
	}{{input, "https://evil.example", true, 403}, {input, "", false, 415}, {strings.TrimSuffix(input, "}") + `,"files":[]}`, "", true, 400}, {strings.Replace(input, `"s1"`, `"other"`, 1), "", true, 409}, {strings.Replace(input, `"preview"`, `"custom"`, 1), "", true, 409}} {
		if w := call("POST", "/releases/deploy-review", test.body, test.origin, test.client); w.Code != test.status {
			t.Fatal("bad review accepted", w.Code, w.Body)
		}
	}
	review := func() string {
		t.Helper()
		w := call("POST", "/releases/deploy-review", input, "", true)
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body)
		}
		var plan struct {
			Token string `json:"token"`
		}
		json.Unmarshal(w.Body.Bytes(), &plan)
		if strings.Contains(w.Body.String(), vercelFixtureToken) || strings.Contains(w.Body.String(), vercelSecretSentinel) {
			t.Fatal("review leaked private fields")
		}
		return plan.Token
	}
	execute := func(token string) *httptest.ResponseRecorder {
		return call("POST", "/releases/deploy-execute", `{"project_id":"p1","service_id":"s1","token":"`+token+`"}`, "", true)
	}
	first := review()
	second := review()
	if execute(first).Code != 409 {
		t.Fatal("superseded review accepted")
	}
	if w := call("POST", "/releases/deploy-cancel", `{"project_id":"p1","service_id":"s1","token":"`+second+`"}`, "", true); w.Code != 200 || execute(second).Code != 409 {
		t.Fatal("cancel did not revoke review")
	}
	token := review()
	changed := *target
	changed.ConnectionID = "disconnected"
	service["vercel"] = &changed
	save()
	if execute(token).Code != 409 {
		t.Fatal("retargeted review executed")
	}
	service["vercel"] = target
	save()
	before, _ := store.Load(t.Context())
	token = review()
	w := execute(token)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	var action releases.DeploymentAction
	json.Unmarshal(w.Body.Bytes(), &action)
	if action.Result.Outcome != "accepted" || action.Result.DeploymentID != "dpl_manual" {
		t.Fatal("request result", w.Body)
	}
	if execute(token).Code != 409 {
		t.Fatal("review reused; duplicate provider write")
	}
	for _, path := range []string{"/releases/deploy-actions?project_id=p1&service_id=s1", "/releases/tracking?project_id=p1&service_id=s1"} {
		w := call("GET", path, "", "", false)
		if w.Code != 200 || strings.Contains(w.Body.String(), vercelFixtureToken) || strings.Contains(w.Body.String(), vercelSecretSentinel) {
			t.Fatal("unsafe history", w.Code, w.Body)
		}
	}
	after, _ := store.Load(t.Context())
	if before.Revision != after.Revision {
		t.Fatal("deployment action rewrote workspace")
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := workspace.Open(store.Path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	actions, err := reopened.DeploymentActions(t.Context(), "p1", "s1")
	if err != nil || len(actions) != 1 || actions[0].Result.DeploymentID != "dpl_manual" {
		t.Fatal("deployment history not durable", err)
	}
	other, err := reopened.DeploymentActions(t.Context(), "p1", "other")
	if err != nil || len(other) != 0 {
		t.Fatal("cross service history")
	}
}
