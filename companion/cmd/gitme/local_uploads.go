package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"time"

	"gitme/companion/internal/localupload"
	"gitme/companion/internal/releases"
	"gitme/companion/internal/vercel"
	"gitme/companion/internal/workspace"
)

func localSource(ctx context.Context, store *workspace.Store, projectID, serviceID string) (localupload.Snapshot, vercel.Link, [32]byte, error) {
	s, err := gitSelection(ctx, store, projectID, serviceID)
	if err != nil {
		return localupload.Snapshot{}, vercel.Link{}, [32]byte{}, err
	}
	target, err := releaseTarget(ctx, store, projectID, serviceID)
	if err != nil || target == nil || !vercel.ValidLink(*target) {
		return localupload.Snapshot{}, vercel.Link{}, [32]byte{}, errors.New("connect this service to Vercel in Deploy Setup first")
	}
	files, err := localupload.Read(ctx, s.ProjectFolder, s.Folder)
	if err != nil {
		return files, *target, [32]byte{}, err
	}
	raw, _ := json.Marshal(struct {
		ProjectFolder string
		Folder        string
		Target        vercel.Link
		Digest        string
	}{s.ProjectFolder, s.Folder, *target, files.Digest})
	return files, *target, sha256.Sum256(raw), nil
}

func registerLocalUploads(mux *http.ServeMux, store *workspace.Store, cloud *vercel.Service, mu *sync.Mutex, busy map[string]bool) {
	type plan struct {
		projectID, serviceID string
		fingerprint          [32]byte
		provider             vercel.LocalReview
		digest               string
	}
	plans := map[string]plan{}
	type selection struct {
		ProjectID   string `json:"project_id"`
		ServiceID   string `json:"service_id"`
		Token       string `json:"token"`
		Environment string `json:"environment"`
	}
	for _, operation := range []string{"local-review", "local-execute", "local-cancel"} {
		mux.HandleFunc("POST /releases/"+operation, func(w http.ResponseWriter, r *http.Request) {
			var input selection
			if !processJSON(w, r, &input) {
				return
			}
			key := input.ProjectID + "\x00" + input.ServiceID
			ctx, cancel := context.WithTimeout(r.Context(), 100*time.Second)
			defer cancel()
			if operation == "local-cancel" {
				mu.Lock()
				if p, ok := plans[input.Token]; ok && p.projectID == input.ProjectID && p.serviceID == input.ServiceID {
					delete(plans, input.Token)
				}
				mu.Unlock()
				respondJSON(w, 200, map[string]bool{"canceled": true})
				return
			}
			if operation == "local-review" {
				source, link, fingerprint, err := localSource(ctx, store, input.ProjectID, input.ServiceID)
				if err != nil {
					http.Error(w, err.Error(), 409)
					return
				}
				provider, err := cloud.ReviewLocal(ctx, link, input.Environment)
				if err != nil {
					http.Error(w, err.Error(), 409)
					return
				}
				_, _, now, err := localSource(ctx, store, input.ProjectID, input.ServiceID)
				if err != nil || now != fingerprint {
					http.Error(w, "Service or files changed. Review again.", 409)
					return
				}
				random := make([]byte, 32)
				if _, err = rand.Read(random); err != nil {
					http.Error(w, "Could not prepare file review.", 500)
					return
				}
				token := hex.EncodeToString(random)
				mu.Lock()
				for k, p := range plans {
					if time.Now().After(p.provider.ExpiresAt) || p.projectID == input.ProjectID && p.serviceID == input.ServiceID {
						delete(plans, k)
					}
				}
				if len(plans) >= 64 || busy[key] {
					mu.Unlock()
					http.Error(w, "Another deployment is being submitted. Retry shortly.", 409)
					return
				}
				plans[token] = plan{input.ProjectID, input.ServiceID, fingerprint, provider, source.Digest}
				mu.Unlock()
				respondJSON(w, 200, struct {
					vercel.LocalReview
					Token  string               `json:"token"`
					Source localupload.Snapshot `json:"source"`
				}{provider, token, source})
				return
			}
			mu.Lock()
			p, ok := plans[input.Token]
			if !ok || p.projectID != input.ProjectID || p.serviceID != input.ServiceID || busy[key] {
				mu.Unlock()
				http.Error(w, "File review is missing, used or belongs to another service. Review again.", 409)
				return
			}
			delete(plans, input.Token)
			busy[key] = true
			mu.Unlock()
			defer func() { mu.Lock(); delete(busy, key); mu.Unlock() }()
			source, _, fingerprint, err := localSource(ctx, store, input.ProjectID, input.ServiceID)
			if err != nil || fingerprint != p.fingerprint || time.Now().After(p.provider.ExpiresAt) {
				http.Error(w, "Files, service settings or review changed. Review again.", 409)
				return
			}
			revalidate := func() error {
				_, _, current, err := localSource(ctx, store, input.ProjectID, input.ServiceID)
				if err != nil || current != p.fingerprint {
					return errors.New("local source or saved service changed; review again")
				}
				return nil
			}
			sum := sha256.Sum256([]byte(input.Token))
			action := releases.DeploymentAction{ID: hex.EncodeToString(sum[:]), Local: &releases.LocalSource{ProjectID: input.ProjectID, ServiceID: input.ServiceID, Target: p.provider.Link, Folder: source.Folder, Digest: source.Digest, Files: len(source.Files), Bytes: source.Bytes}, Environment: p.provider.Target, CreatedAt: time.Now().UTC(), Result: vercel.DeploymentResult{Outcome: "submitting", Message: "Source upload or deployment submission was not confirmed. Check Vercel before retrying."}}
			if err = store.BeginDeployment(ctx, action); err != nil {
				http.Error(w, "Could not save upload history. Nothing was uploaded.", 500)
				return
			}
			action.Result = cloud.CreateLocal(ctx, p.provider, source, revalidate)
			saveCtx, saveCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			defer saveCancel()
			warning := ""
			if err = store.FinishDeployment(saveCtx, action); err != nil {
				warning = "Upload outcome could not be saved. Check Vercel before retrying."
			}
			respondJSON(w, 200, struct {
				releases.DeploymentAction
				Warning string `json:"warning,omitempty"`
			}{action, warning})
		})
		mux.HandleFunc("OPTIONS /releases/"+operation, localUploadOptions)
	}
	mux.HandleFunc("GET /releases/local-status", func(w http.ResponseWriter, r *http.Request) {
		projectID, serviceID, id := r.URL.Query().Get("project_id"), r.URL.Query().Get("service_id"), r.URL.Query().Get("action_id")
		ctx, cancel := context.WithTimeout(r.Context(), 18*time.Second)
		defer cancel()
		target, err := releaseTarget(ctx, store, projectID, serviceID)
		if err != nil {
			http.Error(w, "Saved service is unavailable.", 409)
			return
		}
		actions, err := store.DeploymentActions(ctx, projectID, serviceID)
		if err != nil {
			http.Error(w, "Request history is unavailable.", 500)
			return
		}
		for _, action := range actions {
			if action.ID != id || action.Local == nil {
				continue
			}
			if target == nil || *target != action.Local.Target {
				http.Error(w, "Deployment connection changed. This request keeps its original target.", 409)
				return
			}
			if action.Result.Outcome != "accepted" {
				http.Error(w, "Deployment acceptance is unconfirmed. Check Vercel before retrying.", 409)
				return
			}
			reading, err := cloud.LocalStatus(ctx, action.Local.Target, action.Result.DeploymentID)
			if err != nil {
				http.Error(w, err.Error(), 409)
				return
			}
			respondJSON(w, 200, reading)
			return
		}
		http.Error(w, "Local deployment request not found.", 404)
	})
	mux.HandleFunc("OPTIONS /releases/local-status", localUploadOptions)
}
func localUploadOptions(w http.ResponseWriter, r *http.Request) {
	if m := r.Header.Get("Access-Control-Request-Method"); m != "" && m != "GET" && m != "POST" {
		http.Error(w, "Method not allowed", 405)
		return
	}
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-GitMe-Client")
	w.WriteHeader(204)
}
