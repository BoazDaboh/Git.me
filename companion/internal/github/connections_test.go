package github

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"gitme/companion/internal/credentials"
)

const secondToken = "github_pat_fixture_secondary_1234567890"

func accountFixture(t *testing.T, existing bool) (*Connections, *memoryVault, *memoryVault, map[string]*memoryVault, func() *Connections) {
	t.Helper()
	legacy, index := &memoryVault{}, &memoryVault{}
	if existing {
		legacy.value = fixtureToken
	}
	vaults := map[string]*memoryVault{}
	client := &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		if r.Method != "GET" || r.URL.Scheme != "https" || r.URL.Host != "api.github.com" {
			t.Error("credential sent outside fixed read-only API")
		}
		id, login, status := int64(1), "first", 200
		switch r.Header.Get("Authorization") {
		case "Bearer " + fixtureToken, "Bearer " + fixtureToken + "_updated":
		case "Bearer " + secondToken:
			id, login = 2, "second"
		default:
			status = 401
		}
		var body any = map[string]any{}
		if status == 200 {
			switch r.URL.Path {
			case "/user":
				body = Account{ID: id, Login: login}
			case "/user/repos":
				body = []Repository{{ID: 42, FullName: login + "/repo", DefaultBranch: "main"}}
			case "/repos/shared/repo":
				body = Repository{ID: 42, FullName: "shared/repo", DefaultBranch: "main"}
			case "/repos/shared/repo/branches":
				body = []Branch{{Name: login}}
			case "/repos/shared/repo/commits":
				body = []any{map[string]any{"sha": strings.Repeat("a", 40), "commit": map[string]any{"message": login, "author": map[string]string{"name": login}}}}
			case "/repos/shared/repo/pulls":
				body = []Pull{}
			default:
				status = 404
			}
		}
		raw, _ := json.Marshal(body)
		return &http.Response{StatusCode: status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(string(raw)))}, nil
	})}
	open := func() *Connections {
		return NewConnections(New(legacy, client), index, func(id string) credentials.Vault {
			if vaults[id] == nil {
				vaults[id] = &memoryVault{}
			}
			return vaults[id]
		})
	}
	return open(), legacy, index, vaults, open
}

func TestMultipleAccountsPreserveLegacyAndSurviveRestart(t *testing.T) {
	m, legacy, index, _, open := accountFixture(t, true)
	ctx := context.Background()
	list, err := m.List(ctx, false)
	if err != nil || len(list) != 1 || list[0].ID != "1" || !list[0].Legacy || list[0].Account.Login != "first" {
		t.Fatal("existing connection not preserved")
	}
	second, err := m.Connect(ctx, secondToken, "")
	if err != nil || second.ID != "2" || legacy.value != fixtureToken {
		t.Fatal("second account replaced original")
	}
	for _, id := range []string{"legacy", "2"} {
		s, err := m.Select(id)
		if err != nil {
			t.Fatal(err)
		}
		page, err := s.Repositories(ctx, 1)
		if err != nil || len(page.Repositories) != 1 {
			t.Fatal("account repositories unavailable")
		}
		want := "first"
		if id == "2" {
			want = "second"
		}
		if page.Repositories[0].FullName != want+"/repo" {
			t.Fatal("repository read used another account")
		}
		activity, err := s.Activity(ctx, "shared/repo", 42, false)
		if err != nil || activity.Commits[0].Title != want {
			t.Fatal("private activity cache crossed accounts")
		}
	}
	m = open()
	list, err = m.List(ctx, false)
	if err != nil || len(list) != 2 {
		t.Fatal("accounts did not survive reopening storage")
	}
	if _, err = m.Connect(ctx, secondToken, "legacy"); err == nil || legacy.value != fixtureToken {
		t.Fatal("cross-account token replacement accepted")
	}
	if _, err = m.Connect(ctx, fixtureToken, ""); err != nil {
		t.Fatal(err)
	}
	list, err = m.List(ctx, false)
	if err != nil || len(list) != 2 {
		t.Fatal("same account duplicated")
	}
	raw, _ := json.Marshal(list)
	for _, token := range []string{fixtureToken, secondToken} {
		if strings.Contains(string(raw), token) || strings.Contains(index.value, token) {
			t.Fatal("credential exposed outside token vault")
		}
	}
	if err = m.Disconnect("2"); err != nil {
		t.Fatal(err)
	}
	if _, err = m.Select("2"); err == nil {
		t.Fatal("disconnected account fell back to another account")
	}
	first, _ := m.Select("")
	if _, err = first.Repositories(ctx, 1); err != nil {
		t.Fatal("disconnect broke other account")
	}
	if err = m.Disconnect("legacy"); err != nil {
		t.Fatal(err)
	}
	second, err = m.Connect(ctx, secondToken, "")
	if err != nil || second.ID != "2" {
		t.Fatal("new account adopted old default link")
	}
	first, _ = m.Select("")
	if _, err = first.Repositories(ctx, 1); err == nil {
		t.Fatal("legacy link silently used another account")
	}
	list, err = m.List(ctx, false)
	if err != nil || len(list) != 1 || list[0].ID != "2" {
		t.Fatal("disconnected legacy account was still listed")
	}
}

func TestAccountFailuresDoNotReplaceOrLeakCredentials(t *testing.T) {
	m, legacy, index, vaults, _ := accountFixture(t, false)
	ctx := context.Background()
	first, err := m.Connect(ctx, fixtureToken, "")
	if err != nil || first.ID != "1" {
		t.Fatal("first connection failed")
	}
	index.fail = true
	if _, err = m.Connect(ctx, fixtureToken+"_updated", "1"); err == nil || legacy.value != fixtureToken {
		t.Fatal("failed replacement did not restore previous token")
	}
	if _, err = m.Connect(ctx, secondToken, ""); err == nil || vaults["2"].value != "" || legacy.value != fixtureToken {
		t.Fatal("failed account index write retained new credential or changed existing account")
	}
	index.fail = false
	if _, err = m.Connect(ctx, "github_pat_invalid_fixture_1234567890", ""); err == nil || legacy.value != fixtureToken {
		t.Fatal("failed verification modified existing token")
	}
	for _, id := range []string{"../legacy", "01", "0", "+1", "9007199254740992"} {
		if _, err = m.Select(id); err == nil {
			t.Fatal("invalid connection selected")
		}
	}
	index.value = `[{"id":"../legacy"}]`
	m.loaded = false
	if _, err = m.List(ctx, false); err == nil {
		t.Fatal("invalid credential index accepted")
	}
}

func TestConnectionRefreshCannotResurrectDisconnectedAccount(t *testing.T) {
	m, _, _, _, _ := accountFixture(t, false)
	if _, err := m.Connect(t.Context(), fixtureToken, ""); err != nil {
		t.Fatal(err)
	}
	started, release := make(chan struct{}), make(chan struct{})
	m.legacy.mu.Lock()
	m.legacy.connection.CheckedAt = time.Now().Add(-time.Hour)
	m.legacy.mu.Unlock()
	original := m.legacy.client.Transport
	m.legacy.client.Transport = roundTrip(func(r *http.Request) (*http.Response, error) { close(started); <-release; return original.RoundTrip(r) })
	done := make(chan []SavedConnection, 1)
	go func() { result, _ := m.List(context.Background(), true); done <- result }()
	<-started
	if err := m.Disconnect("legacy"); err != nil {
		t.Fatal(err)
	}
	close(release)
	if result := <-done; len(result) != 0 {
		t.Fatal("old response restored disconnected account")
	}
}
