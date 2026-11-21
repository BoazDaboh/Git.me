package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"gitme/companion/internal/credentials"
	"gitme/companion/internal/github"
	"gitme/companion/internal/processes"
	"gitme/companion/internal/workspace"
)

type fixtureVault struct {
	mu    sync.Mutex
	value string
}

func (v *fixtureVault) Read() (string, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.value == "" {
		return "", credentials.ErrMissing
	}
	return v.value, nil
}
func (v *fixtureVault) Write(value string) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.value = value
	return nil
}
func (v *fixtureVault) Delete() error { v.mu.Lock(); defer v.mu.Unlock(); v.value = ""; return nil }

type fixtureTransport func(*http.Request) (*http.Response, error)

func (f fixtureTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func fixtureGitHub(t *testing.T) *github.Connections {
	t.Helper()
	legacy := github.New(&fixtureVault{}, &http.Client{Transport: fixtureTransport(func(r *http.Request) (*http.Response, error) {
		if r.Method != "GET" || r.URL.Host != "api.github.com" {
			t.Error("GitHub fixture received a write or unexpected host")
		}
		status := 200
		var value any
		headers := http.Header{}
		if !strings.Contains(r.Header.Get("Authorization"), "fixture_readonly_") {
			status = 401
			value = map[string]string{"message": "Bad credentials"}
		} else {
			login, accountID := "fixture-user", 123
			if strings.Contains(r.Header.Get("Authorization"), "secondary") {
				login, accountID = "fixture-second", 456
			}
			repository := func(id int, name string) map[string]any {
				return map[string]any{"id": id, "full_name": login + "/" + name, "private": id == 2, "default_branch": "main", "pushed_at": "2026-11-18T10:00:00Z"}
			}
			path := r.URL.Path
			if strings.HasPrefix(path, "/repos/") && !strings.HasPrefix(path, "/repos/"+login+"/") {
				path = "/denied"
			}
			switch strings.ReplaceAll(path, "fixture-second", "fixture-user") {
			case "/user":
				value = map[string]any{"id": accountID, "login": login, "name": "Fixture account"}
			case "/user/repos":
				if r.URL.Query().Get("page") == "2" {
					value = []any{repository(3, "third-repo")}
				} else {
					value = []any{repository(1, "example"), repository(2, "private-repo")}
					headers.Set("Link", `<https://api.github.com/user/repos?page=2>; rel="next"`)
				}
			case "/repos/fixture-user/example":
				value = repository(1, "example")
			case "/repos/fixture-user/private-repo":
				value = repository(2, "private-repo")
			case "/repos/fixture-user/third-repo":
				value = repository(3, "third-repo")
			case "/repos/fixture-user/example/branches", "/repos/fixture-user/private-repo/branches":
				value = []any{map[string]any{"name": "main", "protected": true}, map[string]any{"name": "feat/map", "protected": false}}
			case "/repos/fixture-user/example/commits", "/repos/fixture-user/private-repo/commits":
				value = []any{map[string]any{"sha": strings.Repeat("a", 40), "commit": map[string]any{"message": "Build <script>bad()</script>\nBody omitted", "author": map[string]string{"name": "Fixture author", "date": "2026-11-18T10:00:00Z"}}}}
			case "/repos/fixture-user/example/pulls":
				value = []any{map[string]any{"number": 7, "title": "Add map preferences", "draft": false, "user": map[string]string{"login": "fixture-user"}, "updated_at": "2026-11-18T10:00:00Z"}}
			case "/repos/fixture-user/private-repo/pulls":
				status = 403
				value = map[string]string{"message": "permission denied"}
			default:
				status = 404
				value = map[string]string{"message": "Not found"}
			}
		}
		data, _ := json.Marshal(value)
		return &http.Response{StatusCode: status, Header: headers, Body: io.NopCloser(strings.NewReader(string(data)))}, nil
	})})
	vaults := map[string]*fixtureVault{}
	return github.NewConnections(legacy, &fixtureVault{}, func(id string) credentials.Vault {
		if vaults[id] == nil {
			vaults[id] = &fixtureVault{}
		}
		return vaults[id]
	})
}

func TestGitHubRoutesAndCredentialIsolation(t *testing.T) {
	store, err := workspace.Open(filepath.Join(t.TempDir(), "test.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	manager := processes.New(store)
	defer manager.Close()
	handler := newHandlerWithCloud(time.Now(), store, manager, fixtureGitHub(t), fixtureVercel(t))
	defer handler.(*gitHandler).Close()
	token := "github_pat_fixture_readonly_1234567890"
	call := func(method, path, body, origin string, header bool) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, "http://"+address+path, bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", origin)
		if header {
			req.Header.Set("X-GitMe-Client", "workspace-v1")
		}
		result := httptest.NewRecorder()
		handler.ServeHTTP(result, req)
		return result
	}
	if response := call("POST", "/github/connect", `{"token":"`+token+`"}`, "https://example.com", true); response.Code != 403 {
		t.Fatal("untrusted website could connect")
	}
	if response := call("POST", "/github/connect", `{"token":"`+token+`"}`, "", false); response.Code != 415 {
		t.Fatal("missing client header accepted")
	}
	response := call("POST", "/github/connect", `{"token":"`+token+`"}`, "http://127.0.0.1:3015", true)
	if response.Code != 200 || strings.Contains(response.Body.String(), token) || !strings.Contains(response.Body.String(), `"connected":true`) {
		t.Fatal("connection failed or exposed credential")
	}
	if response = call("GET", "/github/repositories?page=1", "", "", false); response.Code != 200 || strings.Contains(response.Body.String(), token) {
		t.Fatal("repository list failed or exposed token")
	}
	if response = call("POST", "/github/connect", `{"token":"`+token+`","api_url":"https://example.com"}`, "", true); response.Code != 400 {
		t.Fatal("arbitrary credential destination accepted")
	}
	snapshot, err := store.Load(t.Context())
	if err != nil || snapshot.Revision != 0 || strings.Contains(string(snapshot.Workspace), token) {
		t.Fatal("connection wrote secret into workspace")
	}
	if response = call("POST", "/github/disconnect", "{}", "", true); response.Code != 200 {
		t.Fatal("disconnect failed")
	}
	if response = call("GET", "/github/connection", "", "", false); response.Code != 200 || strings.Contains(response.Body.String(), `"connected":true`) {
		t.Fatal("disconnect did not clear connection")
	}
	request := httptest.NewRequest("OPTIONS", "http://"+address+"/github/connect", nil)
	request.Header.Set("Origin", "http://127.0.0.1:3015")
	request.Header.Set("Access-Control-Request-Method", "POST")
	result := httptest.NewRecorder()
	handler.ServeHTTP(result, request)
	if result.Code != 204 {
		t.Fatal("GitHub preflight failed")
	}
}
