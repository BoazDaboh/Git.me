package vercel

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"gitme/companion/internal/credentials"
	"gitme/companion/internal/localupload"
)

func TestLocalUploadExactBytesTargetsAndNoRetries(t *testing.T) {
	index, vault := &memoryVault{}, &memoryVault{}
	uploads, deploys := 0, 0
	mode := "accepted"
	root := ""
	production := false
	data := []byte("export default 'local uncommitted code';")
	sum := sha1.Sum(data)
	sha := hex.EncodeToString(sum[:])
	source := localupload.Snapshot{Files: []localupload.File{{Path: "src/page.tsx", Size: len(data), SHA: sha, Data: data}}}
	service := New(index, func(string) credentials.Vault { return vault }, &http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Scheme != "https" || r.URL.Host != "api.vercel.com" {
			t.Fatal("credential escaped provider")
		}
		status := 200
		var body any
		switch r.URL.Path {
		case "/v2/user":
			body = map[string]any{"user": Account{ID: "account_one", Username: "fixture"}}
		case "/v9/projects/prj_demo":
			body = map[string]any{"id": "prj_demo", "name": "demo", "accountId": "team_one", "rootDirectory": root, "targets": map[string]any{"production": map[string]string{"id": "dpl_local"}}}
		case "/v2/files":
			uploads++
			raw, _ := io.ReadAll(r.Body)
			if r.Method != "POST" || string(raw) != string(data) || r.Header.Get("x-vercel-digest") != sha || r.ContentLength != int64(len(data)) || r.URL.Query().Get("teamId") != "team_one" {
				t.Fatal("upload bytes or scope changed")
			}
			if mode == "upload-error" {
				return nil, errors.New("SENTINEL_PRIVATE_PROVIDER")
			}
			body = map[string]any{}
		case "/v13/deployments":
			deploys++
			var payload map[string]json.RawMessage
			if json.NewDecoder(r.Body).Decode(&payload) != nil || len(payload) != map[bool]int{false: 3, true: 4}[production] {
				t.Fatal("unexpected provider overrides")
			}
			var refs []struct {
				File, SHA string
				Size      int
			}
			json.Unmarshal(payload["files"], &refs)
			if len(refs) != 1 || refs[0].File != "src/page.tsx" || refs[0].SHA != sha || refs[0].Size != len(data) || string(payload["project"]) != `"prj_demo"` || r.URL.Query().Get("forceNew") != "1" {
				t.Fatal("source drift")
			}
			if production && string(payload["target"]) != `"production"` {
				t.Fatal("production target missing")
			}
			switch mode {
			case "network":
				return nil, errors.New("SENTINEL_PRIVATE_PROVIDER")
			case "rejected":
				status = 403
			case "redirect":
				status = 307
			case "malformed":
				body = map[string]any{"env": "SENTINEL_PRIVATE_PROVIDER"}
			default:
				body = map[string]any{"id": "dpl_local", "projectId": "prj_demo", "readyState": "QUEUED", "url": "local.vercel.app", "env": "SENTINEL_PRIVATE_PROVIDER"}
			}
		case "/v13/deployments/dpl_local":
			body = map[string]any{"id": "dpl_local", "projectId": "prj_demo", "readyState": "READY", "url": "local.vercel.app"}
		default:
			t.Fatal("unexpected request", r.URL.Path)
		}
		raw, _ := json.Marshal(body)
		return &http.Response{StatusCode: status, Header: http.Header{"Location": []string{"https://evil.example"}}, Body: io.NopCloser(strings.NewReader(string(raw)))}, nil
	})})
	if _, err := service.Connect(t.Context(), "fixture_vercel_account_one_123", ""); err != nil {
		t.Fatal(err)
	}
	link := Link{ConnectionID: "account_one", ProjectID: "prj_demo", ProjectName: "demo", TeamID: "team_one", ScopeSlug: "fixture"}
	plan, err := service.ReviewLocal(t.Context(), link, "preview")
	if err != nil || uploads != 0 || deploys != 0 {
		t.Fatal("review wrote", err)
	}
	for _, m := range []string{"accepted", "network", "rejected", "redirect", "malformed", "upload-error"} {
		mode = m
		beforeU, beforeD := uploads, deploys
		result := service.CreateLocal(t.Context(), plan, source, func() error { return nil })
		want := "unconfirmed"
		if m == "accepted" {
			want = "accepted"
		}
		if m == "rejected" {
			want = "rejected"
		}
		if m == "upload-error" {
			want = "not-sent"
		}
		if result.Outcome != want || uploads != beforeU+1 || deploys != beforeD+map[bool]int{true: 0, false: 1}[m == "upload-error"] {
			t.Fatal(m, result, "retry or missing write")
		}
		raw, _ := json.Marshal(result)
		if strings.Contains(string(raw), "SENTINEL") {
			t.Fatal("provider leak")
		}
	}
	mode = "accepted"
	production = true
	prod, err := service.ReviewLocal(t.Context(), link, "production")
	if err != nil {
		t.Fatal(err)
	}
	if result := service.CreateLocal(t.Context(), prod, source, func() error { return nil }); result.Outcome != "accepted" {
		t.Fatal(result)
	}
	before := uploads
	plan.ExpiresAt = time.Now().Add(-time.Second)
	if service.CreateLocal(t.Context(), plan, source, func() error { return nil }).Outcome != "not-sent" || uploads != before {
		t.Fatal("expired upload executed")
	}
	plan, _ = service.ReviewLocal(t.Context(), link, "preview")
	if service.CreateLocal(t.Context(), plan, source, func() error { return errors.New("source changed") }).Outcome != "not-sent" || uploads != before {
		t.Fatal("changed source uploaded")
	}
	root = "apps/web"
	if _, err = service.ReviewLocal(t.Context(), link, "preview"); err == nil {
		t.Fatal("nested root accepted")
	}
	root = ""
	reading, err := service.LocalStatus(t.Context(), link, "dpl_local")
	if err != nil || reading.State != "READY" || !reading.Production {
		t.Fatal("exact ID not tracked", reading, err)
	}
}
