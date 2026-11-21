package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"

	"gitme/companion/internal/apitester"
	"gitme/companion/internal/workspace"
)

func registerAPIRequests(mux *http.ServeMux, store *workspace.Store) {
	requests := make(chan struct{}, 4)
	registerAPIChecks(mux, store, requests)
	reviews := apitester.NewReviews()
	registerAPIPosts(mux, store, requests, reviews)
	mux.HandleFunc("POST /api-test/send", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			ProjectID string             `json:"project_id"`
			ServiceID string             `json:"service_id"`
			URL       string             `json:"url"`
			Headers   []apitester.Header `json:"headers"`
		}
		if !processJSONLimit(w, r, &input, 24<<10) {
			return
		}
		if input.ServiceID != "" {
			if _, err := gitSelection(r.Context(), store, input.ProjectID, input.ServiceID); err != nil {
				http.Error(w, "Choose a saved service.", 409)
				return
			}
		} else {
			if !savedAPIProject(r.Context(), store, input.ProjectID) {
				http.Error(w, "Choose a saved project.", 409)
				return
			}
		}
		select {
		case requests <- struct{}{}:
			defer func() { <-requests }()
		default:
			http.Error(w, "Four requests are already running. Stop one or wait.", 429)
			return
		}
		result, err := (apitester.Runner{}).Run(r.Context(), apitester.Input{URL: input.URL, Headers: input.Headers})
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		respondJSON(w, 200, result)
	})
	mux.HandleFunc("OPTIONS /api-test/send", func(w http.ResponseWriter, r *http.Request) {
		if m := r.Header.Get("Access-Control-Request-Method"); m != "" && m != "POST" {
			http.Error(w, "Method not allowed", 405)
			return
		}
		w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-GitMe-Client")
		w.WriteHeader(204)
	})
}

func apiScope(ctx context.Context, store *workspace.Store, projectID, serviceID string) (string, error) {
	if projectID == "" || len(projectID) > 200 || len(serviceID) > 200 {
		return "", errors.New("choose a saved project and service")
	}
	snapshot, err := store.Load(ctx)
	if err != nil {
		return "", errors.New("saved workspace is unavailable")
	}
	var data struct {
		Projects []struct {
			ID       string            `json:"id"`
			Name     string            `json:"name"`
			Services []json.RawMessage `json:"services"`
		} `json:"projects"`
	}
	if json.Unmarshal(snapshot.Workspace, &data) != nil {
		return "", errors.New("save the workspace first")
	}
	for _, p := range data.Projects {
		if p.ID != projectID {
			continue
		}
		raw := []byte(p.ID + "\x00" + p.Name)
		if serviceID != "" {
			found := false
			for _, service := range p.Services {
				var s struct {
					ID string `json:"id"`
				}
				if json.Unmarshal(service, &s) == nil && s.ID == serviceID {
					raw = append(raw, service...)
					found = true
					break
				}
			}
			if !found {
				return "", errors.New("saved service is no longer available")
			}
		}
		sum := sha256.Sum256(raw)
		return hex.EncodeToString(sum[:]), nil
	}
	return "", errors.New("saved project is no longer available")
}

func registerAPIPosts(mux *http.ServeMux, store *workspace.Store, requests chan struct{}, reviews *apitester.Reviews) {
	for _, operation := range []string{"post-review", "post-execute", "post-cancel"} {
		mux.HandleFunc("POST /api-test/"+operation, func(w http.ResponseWriter, r *http.Request) {
			var selection struct {
				ProjectID string `json:"project_id"`
				ServiceID string `json:"service_id"`
				Token     string `json:"token"`
			}
			if operation == "post-review" {
				var input struct {
					ProjectID string             `json:"project_id"`
					ServiceID string             `json:"service_id"`
					URL       string             `json:"url"`
					Headers   []apitester.Header `json:"headers"`
					Body      string             `json:"body"`
				}
				if !processJSONLimit(w, r, &input, 256<<10) {
					return
				}
				fingerprint, err := apiScope(r.Context(), store, input.ProjectID, input.ServiceID)
				if err != nil {
					http.Error(w, err.Error(), 409)
					return
				}
				plan, err := reviews.Prepare(input.ProjectID+"\x00"+input.ServiceID, fingerprint, apitester.Input{URL: input.URL, Headers: input.Headers, Method: "POST", Body: input.Body})
				if err != nil {
					http.Error(w, err.Error(), 400)
					return
				}
				respondJSON(w, 200, plan)
				return
			}
			if !processJSON(w, r, &selection) {
				return
			}
			scope := selection.ProjectID + "\x00" + selection.ServiceID
			if operation == "post-cancel" {
				reviews.Cancel(scope, selection.Token)
				respondJSON(w, 200, map[string]bool{"canceled": true})
				return
			}
			fingerprint, err := apiScope(r.Context(), store, selection.ProjectID, selection.ServiceID)
			if err != nil {
				http.Error(w, err.Error(), 409)
				return
			}
			select {
			case requests <- struct{}{}:
				defer func() { <-requests }()
			default:
				http.Error(w, "Four requests are already running. Wait, then review again.", 429)
				return
			}
			result, err := reviews.Execute(r.Context(), scope, fingerprint, selection.Token, apitester.Runner{})
			if err != nil {
				http.Error(w, err.Error(), 409)
				return
			}
			respondJSON(w, 200, result)
		})
		mux.HandleFunc("OPTIONS /api-test/"+operation, func(w http.ResponseWriter, r *http.Request) {
			if m := r.Header.Get("Access-Control-Request-Method"); m != "" && m != "POST" {
				http.Error(w, "Method not allowed", 405)
				return
			}
			w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-GitMe-Client")
			w.WriteHeader(204)
		})
	}
}

func savedAPIProject(ctx context.Context, store *workspace.Store, id string) bool {
	if id == "" || len(id) > 200 {
		return false
	}
	snapshot, err := store.Load(ctx)
	if err != nil {
		return false
	}
	var data struct {
		Projects []struct {
			ID string `json:"id"`
		} `json:"projects"`
	}
	if json.Unmarshal(snapshot.Workspace, &data) != nil {
		return false
	}
	for _, p := range data.Projects {
		if p.ID == id {
			return true
		}
	}
	return false
}
