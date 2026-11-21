package vercel

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
		return errors.New("credential storage unavailable")
	}
	v.value = value
	return nil
}
func (v *memoryVault) Delete() error { v.value = ""; return nil }

type transport func(*http.Request) (*http.Response, error)

func (f transport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestPersistentAccountsFailuresAndScopedActivityCache(t *testing.T) {
	index := &memoryVault{}
	vaults := map[string]*memoryVault{}
	var mu sync.Mutex
	calls := 0
	client := &http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
		if r.Method != "GET" || r.URL.Host != "api.vercel.com" || r.URL.Scheme != "https" {
			t.Error("unexpected credential destination or write")
		}
		mu.Lock()
		calls++
		mu.Unlock()
		id := "account_one"
		status := 200
		if strings.Contains(r.Header.Get("Authorization"), "two") {
			id = "account_two"
		}
		if strings.Contains(r.Header.Get("Authorization"), "invalid") {
			status = 401
		}
		var body any = map[string]string{"secret": "never return provider payloads"}
		team := r.URL.Query().Get("teamId")
		ownerID := id
		if team != "" {
			ownerID = team
		}
		if status == 200 {
			switch r.URL.Path {
			case "/v2/user":
				body = map[string]any{"user": Account{ID: id, Username: id}}
			case "/v9/projects/prj_demo":
				body = map[string]string{"id": "prj_demo", "name": "demo", "accountId": ownerID}
			case "/v7/deployments":
				body = map[string]any{"deployments": []any{map[string]string{"uid": "dpl_demo", "state": "READY", "url": ownerID + ".vercel.app"}}}
			default:
				status = 404
			}
		}
		raw, _ := json.Marshal(body)
		return &http.Response{StatusCode: status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(string(raw)))}, nil
	})}
	open := func() *Service {
		return New(index, func(id string) credentials.Vault {
			if vaults[id] == nil {
				vaults[id] = &memoryVault{}
			}
			return vaults[id]
		}, client)
	}
	s := open()
	ctx := context.Background()
	first := "fixture_vercel_account_one_123"
	second := "fixture_vercel_account_two_123"
	if _, err := s.Connect(ctx, first, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Connect(ctx, second, ""); err != nil {
		t.Fatal(err)
	}
	for _, owner := range []string{"account_one", "account_two"} {
		for _, team := range []string{"team_first", "team_second"} {
			link := Link{ConnectionID: owner, TeamID: team, ScopeSlug: team, ProjectID: "prj_demo", ProjectName: "demo"}
			result, err := s.Activity(ctx, link, false)
			if err != nil || result.Deployments[0].URL != "https://"+team+".vercel.app" {
				t.Fatal("scope cache crossed accounts or teams", err)
			}
			mu.Lock()
			before := calls
			mu.Unlock()
			if _, err = s.Activity(ctx, link, true); err != nil {
				t.Fatal(err)
			}
			mu.Lock()
			after := calls
			mu.Unlock()
			if before != after {
				t.Fatal("manual refresh ignored minimum cache interval")
			}
		}
	}
	s = open()
	connections, err := s.Connections(ctx, false)
	if err != nil || len(connections) != 2 {
		t.Fatal("accounts did not survive restart")
	}
	if _, err = s.Connect(ctx, "fixture_vercel_invalid_123456", ""); err == nil || vaults["account_one"].value != first {
		t.Fatal("invalid token replaced saved credential")
	}
	if _, err = s.Connect(ctx, second, "account_one"); err == nil {
		t.Fatal("cross-account replacement accepted")
	}
	index.fail = true
	if _, err = s.Connect(ctx, first+"_replacement", "account_one"); err == nil || vaults["account_one"].value != first {
		t.Fatal("failed index save did not restore old token")
	}
	index.fail = false
	if err = s.Disconnect("account_one"); err != nil {
		t.Fatal(err)
	}
	if _, _, err = s.token("account_one"); err == nil {
		t.Fatal("missing account fell back to another")
	}
	if _, _, err = s.token("account_two"); err != nil {
		t.Fatal("disconnect affected another account")
	}
	for _, value := range []string{first, second} {
		if strings.Contains(index.value, value) {
			t.Fatal("index contains a token")
		}
	}
}

func TestReadTransportAndLinkValidation(t *testing.T) {
	s := New(&memoryVault{}, func(string) credentials.Vault { return &memoryVault{} }, nil)
	if s.client.CheckRedirect == nil || !errors.Is(s.client.CheckRedirect(nil, nil), http.ErrUseLastResponse) {
		t.Fatal("authenticated redirects are allowed")
	}
	for _, raw := range []string{
		`{"connectionId":"user_one","projectId":"prj_demo","projectName":"demo","teamId":"team_one","scopeSlug":"team-one"}`,
		`{"connectionId":"user_one","projectId":"prj_demo","projectName":"demo","teamId":"","scopeSlug":"user-one"}`,
	} {
		if err := ValidateLink(json.RawMessage(raw)); err != nil {
			t.Fatal("valid link rejected", err)
		}
	}
	for _, raw := range []string{
		`{"connectionId":"../user","projectId":"prj_demo","projectName":"demo","teamId":"team_one","scopeSlug":"one"}`,
		`{"connectionId":"user","projectId":"prj_demo","projectName":"demo","teamId":"team_one","scopeSlug":"javascript:alert(1)"}`,
		`{"connectionId":"user","projectId":"prj_demo","projectName":"demo","teamId":null,"scopeSlug":"one"}`,
		`{"connectionId":"user","projectId":"prj_demo","projectName":"demo","teamId":"team_one","scopeSlug":"one","token":"secret"}`,
	} {
		if err := ValidateLink(json.RawMessage(raw)); err == nil {
			t.Fatal("invalid or secret-bearing link accepted")
		}
	}
	for _, value := range []string{"javascript:alert(1)", "https://vercel.app.evil.test", "https://user:password@app.vercel.app", "https://app.vercel.app/?token=secret", "http://app.vercel.app", "https://app.vercel.app/path"} {
		if deploymentURL(value) != "" {
			t.Fatal("unsafe deployment URL accepted")
		}
	}
}
