package main

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gitme/companion/internal/processes"
	"gitme/companion/internal/releases"
	"gitme/companion/internal/vercel"
	"gitme/companion/internal/workspace"
)

func TestLocalUploadReviewScopeChangesAndDurableHistory(t *testing.T) {
	root := t.TempDir()
	sourceFile := filepath.Join(root, "index.html")
	os.WriteFile(sourceFile, []byte("<h1>Local source</h1>"), 0600)
	os.WriteFile(filepath.Join(root, ".env"), []byte("SENTINEL_ENV=private"), 0600)
	store, err := workspace.Open(filepath.Join(root, "test.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	target := &vercel.Link{ConnectionID: "vercel_user_one", TeamID: "team_one", ProjectID: "prj_example", ProjectName: "example-web", ScopeSlug: "team_one"}
	service := map[string]any{"id": "s1", "name": "Web", "role": "frontend", "folder": root, "repository": "", "branch": "", "stack": []string{}, "runtime": "Node.js", "port": nil, "healthPath": "", "updatedAt": 1, "commands": map[string]string{"install": "", "develop": "", "build": "", "test": "", "start": ""}, "deployment": map[string]string{"provider": "Vercel", "method": "managed", "build": "", "start": "", "dockerfile": "", "image": ""}, "environments": map[string]any{"local": map[string]any{"url": "", "variables": []string{}}, "preview": map[string]any{"url": "", "variables": []string{}}, "production": map[string]any{"url": "", "variables": []string{}}}, "vercel": target}
	project := map[string]any{"id": "p1", "name": "Fixture", "stack": []string{}, "description": "", "url": "", "local": "", "note": "", "initials": "F", "color": "blue", "stage": "Building", "branch": "main", "folder": root, "progress": 0, "services": []any{service}}
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
	input := `{"project_id":"p1","service_id":"s1","environment":"preview"}`
	for _, test := range []struct {
		body, origin string
		client       bool
		want         int
	}{{input, "https://evil.example", true, 403}, {input, "", false, 415}, {strings.TrimSuffix(input, "}") + `,"folder":"C:/other"}`, "", true, 400}, {strings.Replace(input, "s1", "other", 1), "", true, 409}, {strings.Replace(input, "preview", "custom", 1), "", true, 409}} {
		if w := call("POST", "/releases/local-review", test.body, test.origin, test.client); w.Code != test.want {
			t.Fatal("unsafe review", w.Code, w.Body)
		}
	}
	review := func() string {
		t.Helper()
		w := call("POST", "/releases/local-review", input, "", true)
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body)
		}
		var p struct {
			Token string `json:"token"`
		}
		json.Unmarshal(w.Body.Bytes(), &p)
		if strings.Contains(w.Body.String(), "SENTINEL_ENV") || strings.Contains(w.Body.String(), vercelFixtureToken) || strings.Contains(w.Body.String(), "<h1>") {
			t.Fatal("review leaked contents")
		}
		return p.Token
	}
	execute := func(token, serviceID string) *httptest.ResponseRecorder {
		return call("POST", "/releases/local-execute", `{"project_id":"p1","service_id":"`+serviceID+`","token":"`+token+`"}`, "", true)
	}
	first, second := review(), review()
	if execute(first, "s1").Code != 409 {
		t.Fatal("superseded review accepted")
	}
	if execute(second, "other").Code != 409 {
		t.Fatal("cross-service review accepted")
	}
	call("POST", "/releases/local-cancel", `{"project_id":"p1","service_id":"s1","token":"`+second+`"}`, "", true)
	if execute(second, "s1").Code != 409 {
		t.Fatal("canceled review executed")
	}
	token := review()
	os.WriteFile(sourceFile, []byte("changed"), 0600)
	if execute(token, "s1").Code != 409 {
		t.Fatal("changed files accepted")
	}
	token = review()
	savedTarget := service["vercel"]
	delete(service, "vercel")
	save()
	if execute(token, "s1").Code != 409 {
		t.Fatal("changed target accepted")
	}
	service["vercel"] = savedTarget
	save()
	before, _ := store.Load(t.Context())
	token = review()
	w := execute(token, "s1")
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	var action releases.DeploymentAction
	json.Unmarshal(w.Body.Bytes(), &action)
	if action.Result.Outcome != "accepted" || action.Local == nil || action.Local.Files != 1 || action.Release.Valid() {
		t.Fatal("local request misrecorded", w.Body)
	}
	if execute(token, "s1").Code != 409 {
		t.Fatal("duplicate upload allowed")
	}
	reading := call("GET", "/releases/local-status?project_id=p1&service_id=s1&action_id="+action.ID, "", "", false)
	if reading.Code != 200 || !strings.Contains(reading.Body.String(), "READY") || strings.Contains(reading.Body.String(), vercelSecretSentinel) {
		t.Fatal("unsafe tracking", reading.Code, reading.Body)
	}
	after, _ := store.Load(t.Context())
	if before.Revision != after.Revision {
		t.Fatal("upload changed workspace")
	}
	store.Close()
	reopened, err := workspace.Open(store.Path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	actions, err := reopened.DeploymentActions(t.Context(), "p1", "s1")
	if err != nil || len(actions) != 1 || actions[0].Local.Digest != action.Local.Digest || actions[0].Result.DeploymentID != "dpl_manual" {
		t.Fatal("local history not durable", err)
	}
}
