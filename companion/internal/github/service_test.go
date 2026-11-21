package github

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	"gitme/companion/internal/credentials"
)

type memoryVault struct {
	value string
	fail  bool
}

func (v *memoryVault) Read() (string, error) {
	if v.value == "" {
		return "", credentials.ErrMissing
	}
	return v.value, nil
}
func (v *memoryVault) Write(value string) error {
	if v.fail {
		return errors.New("vault unavailable")
	}
	v.value = value
	return nil
}
func (v *memoryVault) Delete() error { v.value = ""; return nil }

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

const fixtureToken = "github_pat_fixture_readonly_1234567890"

func fixture(t *testing.T) (*Service, *memoryVault, *[]string) {
	t.Helper()
	vault := &memoryVault{}
	calls := []string{}
	var mu sync.Mutex
	client := &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		if r.Method != "GET" || r.URL.Scheme != "https" || r.URL.Host != "api.github.com" || r.Header.Get("Authorization") != "Bearer "+fixtureToken || r.Header.Get("X-GitHub-Api-Version") != "2026-03-10" {
			t.Error("unexpected authenticated GitHub request")
		}
		mu.Lock()
		calls = append(calls, r.URL.Path)
		mu.Unlock()
		body := "{}"
		status := 200
		header := http.Header{}
		switch r.URL.Path {
		case "/user":
			body = `{"login":"fixture-user","name":"Fixture","id":1}`
		case "/user/repos":
			body = `[{"id":42,"full_name":"fixture-user/example","private":true,"default_branch":"main"}]`
			header.Set("Link", `<https://api.github.com/user/repos?page=2>; rel="next"`)
		case "/repos/fixture-user/example":
			body = `{"id":42,"full_name":"fixture-user/example","private":true,"default_branch":"main"}`
		case "/repos/fixture-user/example/branches":
			body = `[{"name":"main","protected":true}]`
		case "/repos/fixture-user/example/commits":
			body = `[{"sha":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","commit":{"message":"First line\nPrivate body not imported","author":{"name":"Fixture","date":"2026-11-18T10:00:00Z"}}}]`
		case "/repos/fixture-user/example/pulls":
			status = 403
			body = `{"message":"do not echo provider errors or tokens"}`
		default:
			status = 404
		}
		return &http.Response{StatusCode: status, Header: header, Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	return New(vault, client), vault, &calls
}

func TestConnectionReadOnlyActivityAndCaching(t *testing.T) {
	s, vault, calls := fixture(t)
	ctx := context.Background()
	connection, err := s.Connection(ctx, false)
	if err != nil || connection.Connected || connection.Stored || len(*calls) != 0 {
		t.Fatal("fresh connection made an external request")
	}
	connected, err := s.Connect(ctx, fixtureToken)
	if err != nil || !connected.Connected || connected.Account.Login != "fixture-user" || vault.value != fixtureToken {
		t.Fatal("connect failed")
	}
	raw, _ := json.Marshal(connected)
	if strings.Contains(string(raw), fixtureToken) {
		t.Fatal("token returned to browser")
	}
	page, err := s.Repositories(ctx, 1)
	if err != nil || !page.Next || len(page.Repositories) != 1 {
		t.Fatalf("repos: %+v %v", page, err)
	}
	activity, err := s.Activity(ctx, "fixture-user/example", 42, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(activity.Commits) != 1 || activity.Commits[0].Title != "First line" || len(activity.Branches) != 1 || len(activity.Warnings) != 1 || !strings.HasPrefix(activity.Warnings[0], "Pull requests:") {
		t.Fatalf("activity: %+v", activity)
	}
	count := len(*calls)
	if _, err = s.Activity(ctx, "fixture-user/example", 42, true); err != nil || len(*calls) != count {
		t.Fatal("refresh ignored the minimum cache interval")
	}
	if _, err = s.Activity(ctx, "fixture-user/example", 999, false); err == nil {
		t.Fatal("repository identity mismatch accepted")
	}
	if _, err = s.Repository(ctx, "../secrets"); err == nil {
		t.Fatal("repository traversal accepted")
	}
	if err = s.Disconnect(); err != nil {
		t.Fatal(err)
	}
	if vault.value != "" || len(s.cache) != 0 {
		t.Fatal("disconnect retained credentials or cache")
	}
	if _, err = s.Repositories(ctx, 1); err == nil {
		t.Fatal("disconnected reads accepted")
	}
}

func TestFailedValidationKeepsExistingCredentialAndDoesNotLeakToken(t *testing.T) {
	vault := &memoryVault{value: "previous-credential"}
	client := &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 401, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(fixtureToken))}, nil
	})}
	s := New(vault, client)
	_, err := s.Connect(context.Background(), fixtureToken)
	if err == nil || strings.Contains(err.Error(), fixtureToken) || vault.value != "previous-credential" {
		t.Fatal("failed validation changed the credential or exposed token")
	}
	if _, err = s.Connect(context.Background(), "my-account-password"); err == nil {
		t.Fatal("password accepted")
	}
}

func TestVaultFailureHasNoPlaintextFallback(t *testing.T) {
	s, vault, _ := fixture(t)
	vault.fail = true
	if _, err := s.Connect(context.Background(), fixtureToken); err == nil || vault.value != "" {
		t.Fatal("vault failure ignored")
	}
}

func TestNoRedirectFollowing(t *testing.T) {
	client := New(&memoryVault{}, nil).client
	if client.CheckRedirect == nil || !errors.Is(client.CheckRedirect(nil, nil), http.ErrUseLastResponse) {
		t.Fatal("GitHub credentials could follow redirects")
	}
}
