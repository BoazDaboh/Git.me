package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"gitme/companion/internal/vercel"
	"gitme/companion/internal/workspace"
)

func registerVercel(mux *http.ServeMux, store *workspace.Store, service *vercel.Service) {
	mux.HandleFunc("GET /vercel/connections", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
		defer cancel()
		result, err := service.Connections(ctx, r.URL.Query().Get("refresh") == "1")
		if err != nil {
			http.Error(w, err.Error(), 503)
			return
		}
		respondJSON(w, 200, map[string]any{"connections": result})
	})
	mux.HandleFunc("POST /vercel/connect", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Token        string `json:"token"`
			ConnectionID string `json:"connection_id"`
		}
		if !processJSON(w, r, &input) {
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
		defer cancel()
		result, err := service.Connect(ctx, input.Token, input.ConnectionID)
		input.Token = ""
		if err != nil {
			http.Error(w, err.Error(), 422)
			return
		}
		respondJSON(w, 200, result)
	})
	mux.HandleFunc("POST /vercel/disconnect", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			ConnectionID string `json:"connection_id"`
		}
		if !processJSON(w, r, &input) {
			return
		}
		if err := service.Disconnect(input.ConnectionID); err != nil {
			http.Error(w, err.Error(), 422)
			return
		}
		respondJSON(w, 200, map[string]bool{"connected": false})
	})
	page := func(w http.ResponseWriter, r *http.Request) (int64, bool) {
		raw := r.URL.Query().Get("until")
		if raw == "" {
			return 0, true
		}
		n, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || n < 0 {
			http.Error(w, "Invalid Vercel page", 400)
			return 0, false
		}
		return n, true
	}
	mux.HandleFunc("GET /vercel/teams", func(w http.ResponseWriter, r *http.Request) {
		until, ok := page(w, r)
		if !ok {
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
		defer cancel()
		result, err := service.Teams(ctx, r.URL.Query().Get("connection_id"), until, r.URL.Query().Get("from"))
		if err != nil {
			http.Error(w, err.Error(), 422)
			return
		}
		respondJSON(w, 200, result)
	})
	mux.HandleFunc("GET /vercel/projects", func(w http.ResponseWriter, r *http.Request) {
		until, ok := page(w, r)
		if !ok {
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
		defer cancel()
		q := r.URL.Query()
		result, err := service.Projects(ctx, q.Get("connection_id"), q.Get("team_id"), until, q.Get("from"), q.Get("search"))
		if err != nil {
			http.Error(w, err.Error(), 422)
			return
		}
		respondJSON(w, 200, result)
	})
	mux.HandleFunc("GET /vercel/project", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
		defer cancel()
		q := r.URL.Query()
		result, err := service.Project(ctx, q.Get("connection_id"), q.Get("team_id"), q.Get("name"))
		if err != nil {
			http.Error(w, err.Error(), 422)
			return
		}
		respondJSON(w, 200, result)
	})
	mux.HandleFunc("GET /vercel/activity", func(w http.ResponseWriter, r *http.Request) {
		projectID, serviceID := r.URL.Query().Get("project_id"), r.URL.Query().Get("service_id")
		if projectID == "" || len(projectID) > 200 || len(serviceID) > 200 {
			http.Error(w, "Choose a saved project", 400)
			return
		}
		snapshot, err := store.Load(r.Context())
		if err != nil {
			http.Error(w, "Could not read Vercel project link", 500)
			return
		}
		var data struct {
			Projects []struct {
				ID       string       `json:"id"`
				Vercel   *vercel.Link `json:"vercel"`
				Services []struct {
					ID     string       `json:"id"`
					Vercel *vercel.Link `json:"vercel"`
				} `json:"services"`
			} `json:"projects"`
		}
		if json.Unmarshal(snapshot.Workspace, &data) != nil {
			http.Error(w, "Save the workspace first", 409)
			return
		}
		var selected *vercel.Link
		for _, p := range data.Projects {
			if p.ID != projectID {
				continue
			}
			if serviceID == "" {
				selected = p.Vercel
			} else {
				for _, s := range p.Services {
					if s.ID == serviceID {
						selected = s.Vercel
						break
					}
				}
			}
			break
		}
		if selected == nil {
			http.Error(w, "Link a Vercel project first", 409)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
		defer cancel()
		result, err := service.Activity(ctx, *selected, r.URL.Query().Get("refresh") == "1")
		if err != nil {
			http.Error(w, err.Error(), 422)
			return
		}
		respondJSON(w, 200, result)
	})
	for _, path := range []string{"connections", "connect", "disconnect", "teams", "projects", "project", "activity"} {
		mux.HandleFunc("OPTIONS /vercel/"+path, func(w http.ResponseWriter, r *http.Request) {
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
