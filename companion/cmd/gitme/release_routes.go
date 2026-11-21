package main

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"gitme/companion/internal/releases"
	"gitme/companion/internal/workspace"
)

func registerReleases(mux *http.ServeMux, store *workspace.Store) {
	mux.HandleFunc("POST /releases/inspect", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			ProjectID string `json:"project_id"`
			ServiceID string `json:"service_id"`
		}
		if !processJSON(w, r, &input) {
			return
		}
		if input.ProjectID == "" || input.ServiceID == "" || len(input.ProjectID) > 200 || len(input.ServiceID) > 200 {
			http.Error(w, "Choose a saved service", 400)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		snapshot, err := store.Load(ctx)
		if err != nil {
			http.Error(w, "Could not read the saved service", 500)
			return
		}
		var data struct {
			Projects []struct {
				ID       string `json:"id"`
				Folder   string `json:"folder"`
				Services []struct {
					ID     string `json:"id"`
					Folder string `json:"folder"`
				} `json:"services"`
			} `json:"projects"`
		}
		if json.Unmarshal(snapshot.Workspace, &data) != nil {
			http.Error(w, "Save the workspace first", 409)
			return
		}
		for _, project := range data.Projects {
			if project.ID != input.ProjectID {
				continue
			}
			for _, service := range project.Services {
				if service.ID != input.ServiceID {
					continue
				}
				respondJSON(w, 200, releases.Inspect(ctx, project.Folder, service.Folder))
				return
			}
			break
		}
		http.Error(w, "Choose a saved service", 404)
	})
	mux.HandleFunc("OPTIONS /releases/inspect", func(w http.ResponseWriter, r *http.Request) {
		if method := r.Header.Get("Access-Control-Request-Method"); method != "" && method != "POST" {
			http.Error(w, "Method not allowed", 405)
			return
		}
		w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-GitMe-Client")
		w.WriteHeader(204)
	})
}
