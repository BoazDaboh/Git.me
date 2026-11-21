package main

import (
	"encoding/json"
	"gitme/companion/internal/localgit"
	"gitme/companion/internal/processes"
	"gitme/companion/internal/releases"
	"gitme/companion/internal/vercel"
	"gitme/companion/internal/workspace"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDeploymentTrackingPersistsSourceAndRejectsChangedScopes(t *testing.T) {
	store, err := workspace.Open(filepath.Join(t.TempDir(), "test.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	target := &vercel.Link{ConnectionID: "vercel_user_one", TeamID: "team_one", ProjectID: "prj_example", ProjectName: "example-web", ScopeSlug: "team_one"}
	service := map[string]any{"id": "s1", "name": "Web", "role": "frontend", "runtime": "Node.js", "folder": "C:\\fixture", "repository": "https://github.com/fixture-user/private-repo.git", "branch": "main", "healthPath": "", "stack": []string{}, "port": nil, "updatedAt": 1, "commands": map[string]string{"install": "", "develop": "", "build": "", "test": "", "start": ""}, "deployment": map[string]string{"provider": "Vercel", "method": "managed", "build": "", "start": "", "dockerfile": "", "image": ""}, "environments": map[string]any{"local": map[string]any{"url": "", "variables": []string{}}, "preview": map[string]any{"url": "", "variables": []string{}}, "production": map[string]any{"url": "", "variables": []string{}}}, "vercel": target}
	project := map[string]any{"id": "p1", "name": "Fixture", "initials": "F", "color": "blue", "description": "", "stage": "Building", "branch": "main", "folder": "C:\\fixture", "url": "", "local": "", "note": "", "stack": []string{}, "progress": 0, "services": []any{service}}
	data := map[string]any{"version": 1, "projects": []any{project}, "tasks": []any{}, "notes": []any{}, "sessions": []any{}, "drafts": []any{}, "focus": nil, "document": map[string]string{"title": "", "text": ""}}
	save := func() {
		t.Helper()
		snapshot, err := store.Load(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(data)
		if _, err = store.Save(t.Context(), snapshot.DatabaseID, snapshot.Revision, raw); err != nil {
			t.Fatal(err)
		}
	}
	save()
	manager := processes.New(store)
	defer manager.Close()
	cloud := fixtureVercel(t)
	if _, err = cloud.Connect(t.Context(), vercelFixtureToken, ""); err != nil {
		t.Fatal(err)
	}
	handler := newHandlerWithCloud(time.Now(), store, manager, fixtureGitHub(t), cloud)
	defer handler.(*gitHandler).Close()
	call := func(method, path, body, origin string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "http://"+address+path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-GitMe-Client", "workspace-v1")
		r.Header.Set("Origin", origin)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	pushed := localgit.PushResult{URL: "https://github.com/fixture-user/private-repo.git", Branch: "main", Commit: strings.Repeat("a", 40), Verified: true, Message: "Push accepted"}
	savePushRecord(t.Context(), store, localgit.Selection{ProjectID: "p1", ServiceID: "s1"}, target, &pushed)
	if pushed.ReleaseID == "" || pushed.TrackingWarning != "" {
		t.Fatal("push was not recorded")
	}
	path := "/releases/tracking?project_id=p1&service_id=s1"
	before, _ := store.Load(t.Context())
	w := call("GET", path, "", "")
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	var result struct {
		History []releaseSummary `json:"history"`
		Release *trackedRelease  `json:"release"`
	}
	if json.Unmarshal(w.Body.Bytes(), &result) != nil || result.Release == nil || result.Release.State != "matched" || len(result.History) != 1 || result.Release.Source != "push" || result.Release.Reading.Deployments[0].State != "BUILDING" {
		t.Fatal("tracking did not match", w.Body)
	}
	if strings.Contains(w.Body.String(), vercelSecretSentinel) || strings.Contains(w.Body.String(), vercelFixtureToken) {
		t.Fatal("tracking leaked credentials/metadata")
	}
	after, _ := store.Load(t.Context())
	if before.Revision != after.Revision {
		t.Fatal("tracking read rewrote workspace")
	}
	foreign, err := store.RecordRelease(t.Context(), releases.Record{ProjectID: "p1", ServiceID: "s2", Repository: pushed.URL, Branch: "main", Commit: pushed.Commit, Source: "push", Target: target})
	if err != nil {
		t.Fatal(err)
	}
	if call("GET", path+"&release_id="+foreign.ID, "", "").Code != 404 {
		t.Fatal("cross-service record exposed")
	}
	if call("GET", path, "", "https://evil.example").Code != 403 {
		t.Fatal("foreign origin read tracking")
	}
	if call("POST", "/releases/track-current", `{"project_id":"p1","service_id":"s1","command":"git push"}`, "").Code != 400 {
		t.Fatal("tracking accepted a command")
	}
	changed := *target
	changed.ProjectID = "prj_api"
	changed.ProjectName = "example-api"
	service["vercel"] = &changed
	save()
	w = call("GET", path, "", "")
	json.Unmarshal(w.Body.Bytes(), &result)
	if result.Release.State != "link-changed" || result.Release.Reading != nil {
		t.Fatal("old release silently retargeted", w.Body)
	}
	service["vercel"] = target
	save()
	if err = cloud.Disconnect(target.ConnectionID); err != nil {
		t.Fatal(err)
	}
	w = call("GET", path, "", "")
	json.Unmarshal(w.Body.Bytes(), &result)
	if result.Release.State != "unavailable" || result.Release.Reading != nil {
		t.Fatal("disconnected account exposed cached reading", w.Body)
	}
	service["repository"] = "https://github.com/fixture-user/another"
	save()
	w = call("GET", path, "", "")
	json.Unmarshal(w.Body.Bytes(), &result)
	if result.Release.State != "source-changed" || result.Release.Reading != nil {
		t.Fatal("old release moved repositories", w.Body)
	}
}
