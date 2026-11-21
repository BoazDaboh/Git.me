package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"gitme/companion/internal/releases"
	"gitme/companion/internal/vercel"
	"gitme/companion/internal/workspace"
)

func deploymentSource(ctx context.Context, store *workspace.Store, projectID, serviceID, releaseID string) (releases.Record, [32]byte, error) {
	selection, err := gitSelection(ctx, store, projectID, serviceID)
	if err != nil {
		return releases.Record{}, [32]byte{}, err
	}
	target, err := releaseTarget(ctx, store, projectID, serviceID)
	if err != nil {
		return releases.Record{}, [32]byte{}, err
	}
	records, err := store.Releases(ctx, projectID, serviceID)
	if err != nil {
		return releases.Record{}, [32]byte{}, errors.New("could not read recorded GitHub commits")
	}
	for _, record := range records {
		if record.ID != releaseID {
			continue
		}
		if !sameTarget(record.Target, target) {
			return record, [32]byte{}, errors.New("the service deployment link changed; track its GitHub commit again")
		}
		repository := releases.RepositoryName(selection.RepositoryURL)
		if repository == "" && selection.RepositoryURL == "" {
			repository = selection.GitHubRepository
		}
		if !strings.EqualFold(repository, releases.RepositoryName(record.Repository)) || selection.GitHubRepository != "" && !strings.EqualFold(repository, selection.GitHubRepository) {
			return record, [32]byte{}, errors.New("the service repository changed; review its GitHub link")
		}
		raw, _ := json.Marshal(struct {
			Record    releases.Record
			Selection any
			Target    *vercel.Link
		}{record, selection, target})
		return record, sha256.Sum256(raw), nil
	}
	return releases.Record{}, [32]byte{}, errors.New("choose a recorded GitHub commit for this service")
}

func registerDeploymentActions(mux *http.ServeMux, store *workspace.Store, cloud *vercel.Service) {
	type review struct {
		Record      releases.Record
		Fingerprint [32]byte
		Provider    vercel.DeploymentReview
	}
	var mu sync.Mutex
	plans := map[string]review{}
	busy := map[string]bool{}
	registerLocalUploads(mux, store, cloud, &mu, busy)
	type selection struct {
		ProjectID string `json:"project_id"`
		ServiceID string `json:"service_id"`
	}
	mux.HandleFunc("POST /releases/deploy-review", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			selection
			ReleaseID   string `json:"release_id"`
			Environment string `json:"environment"`
		}
		if !processJSON(w, r, &input) {
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
		defer cancel()
		record, fingerprint, err := deploymentSource(ctx, store, input.ProjectID, input.ServiceID, input.ReleaseID)
		if err != nil {
			http.Error(w, err.Error(), 409)
			return
		}
		provider, err := cloud.ReviewDeployment(ctx, *record.Target, releases.RepositoryName(record.Repository), record.Branch, record.Commit, input.Environment)
		if err != nil {
			http.Error(w, err.Error(), 409)
			return
		}
		_, now, err := deploymentSource(ctx, store, input.ProjectID, input.ServiceID, input.ReleaseID)
		if err != nil || now != fingerprint {
			http.Error(w, "Service settings changed. Review again.", 409)
			return
		}
		keyBytes := make([]byte, 32)
		if _, err = rand.Read(keyBytes); err != nil {
			http.Error(w, "Could not prepare deployment review.", 500)
			return
		}
		token := hex.EncodeToString(keyBytes)
		mu.Lock()
		for k, p := range plans {
			if time.Now().After(p.Provider.ExpiresAt) || p.Record.ProjectID == record.ProjectID && p.Record.ServiceID == record.ServiceID {
				delete(plans, k)
			}
		}
		if len(plans) >= 64 || busy[input.ProjectID+"\x00"+input.ServiceID] {
			mu.Unlock()
			http.Error(w, "Another deployment request is being reviewed or submitted. Retry shortly.", 409)
			return
		}
		plans[token] = review{record, fingerprint, provider}
		mu.Unlock()
		respondJSON(w, 200, struct {
			vercel.DeploymentReview
			Token     string `json:"token"`
			ReleaseID string `json:"release_id"`
		}{provider, token, record.ID})
	})
	mux.HandleFunc("POST /releases/deploy-cancel", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			selection
			Token string `json:"token"`
		}
		if !processJSON(w, r, &input) {
			return
		}
		mu.Lock()
		if p, ok := plans[input.Token]; ok && p.Record.ProjectID == input.ProjectID && p.Record.ServiceID == input.ServiceID {
			delete(plans, input.Token)
		}
		mu.Unlock()
		respondJSON(w, 200, map[string]bool{"canceled": true})
	})
	mux.HandleFunc("POST /releases/deploy-execute", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			selection
			Token string `json:"token"`
		}
		if !processJSON(w, r, &input) {
			return
		}
		key := input.ProjectID + "\x00" + input.ServiceID
		mu.Lock()
		plan, ok := plans[input.Token]
		if !ok || plan.Record.ProjectID != input.ProjectID || plan.Record.ServiceID != input.ServiceID || busy[key] {
			mu.Unlock()
			http.Error(w, "Deployment review is missing, used or belongs to another service. Review again.", 409)
			return
		}
		delete(plans, input.Token)
		busy[key] = true
		mu.Unlock()
		defer func() { mu.Lock(); delete(busy, key); mu.Unlock() }()
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		revalidate := func() error {
			_, fingerprint, err := deploymentSource(ctx, store, input.ProjectID, input.ServiceID, plan.Record.ID)
			if err != nil || fingerprint != plan.Fingerprint {
				return errors.New("saved service or release changed; review deployment again")
			}
			return nil
		}
		if err := revalidate(); err != nil || time.Now().After(plan.Provider.ExpiresAt) {
			http.Error(w, "Deployment review expired or service changed. Review again.", 409)
			return
		}
		sum := sha256.Sum256([]byte(input.Token))
		action := releases.DeploymentAction{ID: hex.EncodeToString(sum[:]), Release: plan.Record, Environment: plan.Provider.Target, CreatedAt: time.Now().UTC(), Result: vercel.DeploymentResult{Outcome: "submitting", Message: "Request submission has not been confirmed. Check Vercel before retrying."}}
		if err := store.BeginDeployment(ctx, action); err != nil {
			http.Error(w, "Could not save the deployment request. Nothing was submitted.", 500)
			return
		}
		action.Result = cloud.CreateDeployment(ctx, plan.Provider, revalidate)
		saveCtx, saveCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer saveCancel()
		warning := ""
		if err := store.FinishDeployment(saveCtx, action); err != nil {
			warning = "The request outcome could not be saved. Check Vercel before retrying."
		}
		respondJSON(w, 200, struct {
			releases.DeploymentAction
			Warning string `json:"warning,omitempty"`
		}{action, warning})
	})
	mux.HandleFunc("GET /releases/deploy-actions", func(w http.ResponseWriter, r *http.Request) {
		projectID, serviceID := r.URL.Query().Get("project_id"), r.URL.Query().Get("service_id")
		if _, err := gitSelection(r.Context(), store, projectID, serviceID); err != nil {
			http.Error(w, err.Error(), 409)
			return
		}
		actions, err := store.DeploymentActions(r.Context(), projectID, serviceID)
		if err != nil {
			http.Error(w, "Could not read deployment request history.", 500)
			return
		}
		respondJSON(w, 200, map[string]any{"actions": actions})
	})
	for _, path := range []string{"deploy-review", "deploy-execute", "deploy-cancel", "deploy-actions"} {
		mux.HandleFunc("OPTIONS /releases/"+path, func(w http.ResponseWriter, r *http.Request) {
			method := r.Header.Get("Access-Control-Request-Method")
			if method != "" && method != "GET" && method != "POST" {
				http.Error(w, "Method not allowed", 405)
				return
			}
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-GitMe-Client")
			w.WriteHeader(204)
		})
	}
}
