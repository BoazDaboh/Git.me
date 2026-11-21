package vercel

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"gitme/companion/internal/credentials"
)

func TestReviewedDeploymentExactSourceAndUncertainWrites(t *testing.T) {
	index, vault := &memoryVault{}, &memoryVault{}
	posts := 0
	mode := "accepted"
	badRepository := false
	service := New(index, func(string) credentials.Vault { return vault }, &http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Scheme != "https" || r.URL.Host != "api.vercel.com" {
			t.Fatal("credential sent outside Vercel")
		}
		status := 200
		var body any
		switch r.URL.Path {
		case "/v2/user":
			body = map[string]any{"user": Account{ID: "account_one", Username: "fixture"}}
		case "/v9/projects/prj_demo":
			repo := "repo"
			if badRepository {
				repo = "other"
			}
			body = map[string]any{"id": "prj_demo", "name": "demo", "accountId": "team_one", "link": map[string]string{"type": "github", "org": "fixture", "repo": repo}}
		case "/v13/deployments":
			posts++
			if r.Method != "POST" || r.URL.Query().Get("teamId") != "team_one" || r.URL.Query().Get("forceNew") != "1" {
				t.Fatal("wrong scoped write")
			}
			var payload map[string]json.RawMessage
			if json.NewDecoder(r.Body).Decode(&payload) != nil || len(payload) != 3 {
				t.Fatal("unexpected deployment fields")
			}
			var source map[string]string
			json.Unmarshal(payload["gitSource"], &source)
			if source["sha"] != strings.Repeat("a", 40) || source["ref"] != "main" || source["org"] != "fixture" || source["repo"] != "repo" || source["type"] != "github" {
				t.Fatal("branch tip used instead of reviewed commit")
			}
			if _, exists := payload["target"]; exists || string(payload["project"]) != `"prj_demo"` {
				t.Fatal("target drift")
			}
			switch mode {
			case "network":
				return nil, errors.New("SECRET_PROVIDER_PAYLOAD")
			case "rejected":
				status = 403
			case "redirect":
				status = 307
			case "server":
				status = 500
			case "malformed":
				body = map[string]any{"env": "SECRET_PROVIDER_PAYLOAD"}
			default:
				body = map[string]any{"id": "dpl_requested", "projectId": "prj_demo", "readyState": "QUEUED", "url": "fixture.vercel.app", "env": "SECRET_PROVIDER_PAYLOAD"}
			}
		default:
			t.Fatal("unexpected request", r.Method, r.URL.Path)
		}
		raw, _ := json.Marshal(body)
		return &http.Response{StatusCode: status, Header: http.Header{"Location": []string{"https://evil.example"}}, Body: io.NopCloser(strings.NewReader(string(raw)))}, nil
	})})
	if _, err := service.Connect(t.Context(), "fixture_vercel_account_one_123", ""); err != nil {
		t.Fatal(err)
	}
	link := Link{ConnectionID: "account_one", ProjectID: "prj_demo", ProjectName: "demo", TeamID: "team_one", ScopeSlug: "fixture"}
	review, err := service.ReviewDeployment(t.Context(), link, "fixture/repo", "main", strings.Repeat("a", 40), "preview")
	if err != nil || posts != 0 {
		t.Fatal("review wrote to provider", err)
	}
	raw, _ := json.Marshal(review)
	if strings.Contains(string(raw), "credential") || strings.Contains(string(raw), vault.value) {
		t.Fatal("review exposes credential")
	}
	for _, value := range []string{"accepted", "network", "rejected", "redirect", "server", "malformed"} {
		mode = value
		before := posts
		result := service.CreateDeployment(t.Context(), review, func() error { return nil })
		want := "unconfirmed"
		if value == "accepted" {
			want = "accepted"
		}
		if value == "rejected" {
			want = "rejected"
		}
		if result.Outcome != want || posts != before+1 {
			t.Fatal(value, "retried or misclassified", result)
		}
		raw, _ = json.Marshal(result)
		if strings.Contains(string(raw), "SECRET_PROVIDER_PAYLOAD") {
			t.Fatal("raw provider error leaked")
		}
	}
	before := posts
	if result := service.CreateDeployment(t.Context(), review, func() error { return errors.New("service changed") }); result.Outcome != "not-sent" {
		t.Fatal("ignored service revalidation")
	}
	expired := review
	expired.ExpiresAt = time.Now().Add(-time.Second)
	if result := service.CreateDeployment(t.Context(), expired, func() error { return nil }); result.Outcome != "not-sent" {
		t.Fatal("expired write")
	}
	badRepository = true
	if result := service.CreateDeployment(t.Context(), review, func() error { return nil }); result.Outcome != "not-sent" {
		t.Fatal("repository drift")
	}
	badRepository = false
	vault.value = "replacement_vercel_token_account_one"
	if result := service.CreateDeployment(context.Background(), review, func() error { return nil }); result.Outcome != "not-sent" {
		t.Fatal("token drift")
	}
	if posts != before {
		t.Fatal("invalid reviews sent writes")
	}
}
