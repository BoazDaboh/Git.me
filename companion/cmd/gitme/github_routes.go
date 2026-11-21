package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"gitme/companion/internal/github"
	"gitme/companion/internal/workspace"
)

func registerGitHub(mux *http.ServeMux, store *workspace.Store, accounts *github.Connections) {
	mux.HandleFunc("GET /github/connection", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
		defer cancel()
		service, err := accounts.Select(r.URL.Query().Get("connection_id"))
		if err != nil {
			http.Error(w, err.Error(), 422)
			return
		}
		result, err := service.Connection(ctx, r.URL.Query().Get("refresh") == "1")
		if err != nil {
			http.Error(w, err.Error(), 503)
			return
		}
		respondJSON(w, 200, result)
	})
	mux.HandleFunc("POST /github/connect", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Token        string `json:"token"`
			ConnectionID string `json:"connection_id"`
		}
		if !processJSON(w, r, &input) {
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
		defer cancel()
		result, err := accounts.Connect(ctx, input.Token, input.ConnectionID)
		input.Token = ""
		if err != nil {
			http.Error(w, err.Error(), 422)
			return
		}
		respondJSON(w, 200, result)
	})
	mux.HandleFunc("POST /github/disconnect", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			ConnectionID string `json:"connection_id"`
		}
		if !processJSON(w, r, &input) {
			return
		}
		if err := accounts.Disconnect(input.ConnectionID); err != nil {
			http.Error(w, err.Error(), 503)
			return
		}
		respondJSON(w, 200, map[string]bool{"connected": false})
	})
	mux.HandleFunc("GET /github/repositories", func(w http.ResponseWriter, r *http.Request) {
		page := 1
		if raw := r.URL.Query().Get("page"); raw != "" {
			value, err := strconv.Atoi(raw)
			if err != nil {
				http.Error(w, "Invalid repository page", 400)
				return
			}
			page = value
		}
		ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
		defer cancel()
		service, err := accounts.Select(r.URL.Query().Get("connection_id"))
		if err != nil {
			http.Error(w, err.Error(), 422)
			return
		}
		result, err := service.Repositories(ctx, page)
		if err != nil {
			http.Error(w, err.Error(), 422)
			return
		}
		respondJSON(w, 200, result)
	})
	mux.HandleFunc("GET /github/repository", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
		defer cancel()
		service, err := accounts.Select(r.URL.Query().Get("connection_id"))
		if err != nil {
			http.Error(w, err.Error(), 422)
			return
		}
		repo, err := service.Repository(ctx, r.URL.Query().Get("full_name"))
		if err != nil {
			http.Error(w, err.Error(), 422)
			return
		}
		respondJSON(w, 200, repo)
	})
	mux.HandleFunc("GET /github/activity", func(w http.ResponseWriter, r *http.Request) {
		projectID, serviceID := r.URL.Query().Get("project_id"), r.URL.Query().Get("service_id")
		if projectID == "" || serviceID == "" || len(projectID) > 200 || len(serviceID) > 200 {
			http.Error(w, "Choose a saved service and its GitHub repository", 400)
			return
		}
		snapshot, err := store.Load(r.Context())
		if err != nil {
			http.Error(w, "Could not read repository link", 500)
			return
		}
		type link struct {
			ID           int64  `json:"id"`
			FullName     string `json:"fullName"`
			ConnectionID string `json:"connectionId"`
		}
		var data struct {
			Projects []struct {
				ID       string `json:"id"`
				Services []struct {
					ID     string `json:"id"`
					GitHub *link  `json:"github"`
				} `json:"services"`
			} `json:"projects"`
		}
		if json.Unmarshal(snapshot.Workspace, &data) != nil {
			http.Error(w, "Save the workspace first", 409)
			return
		}
		var selected *link
		for _, project := range data.Projects {
			if project.ID != projectID {
				continue
			}
			for _, profile := range project.Services {
				if profile.ID == serviceID {
					selected = profile.GitHub
					break
				}
			}
			break
		}
		if selected == nil {
			http.Error(w, "Link a repository first", 409)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
		defer cancel()
		service, err := accounts.Select(selected.ConnectionID)
		if err != nil {
			http.Error(w, err.Error(), 422)
			return
		}
		result, err := service.Activity(ctx, selected.FullName, selected.ID, r.URL.Query().Get("refresh") == "1")
		if err != nil {
			http.Error(w, err.Error(), 422)
			return
		}
		respondJSON(w, 200, result)
	})
	mux.HandleFunc("GET /github/connections", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
		defer cancel()
		result, err := accounts.List(ctx, r.URL.Query().Get("refresh") == "1")
		if err != nil {
			http.Error(w, err.Error(), 503)
			return
		}
		respondJSON(w, 200, map[string]any{"connections": result})
	})
	for _, path := range []string{"/github/connections", "/github/connection", "/github/connect", "/github/disconnect", "/github/repositories", "/github/repository", "/github/activity"} {
		mux.HandleFunc("OPTIONS "+path, func(w http.ResponseWriter, r *http.Request) {
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
