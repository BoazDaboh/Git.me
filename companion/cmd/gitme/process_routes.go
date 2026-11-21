package main

import (
	"encoding/json"
	"gitme/companion/internal/processes"
	"io"
	"mime"
	"net/http"
)

func processJSON(w http.ResponseWriter, r *http.Request, value any) bool {
	return processJSONLimit(w, r, value, 8192)
}
func processJSONLimit(w http.ResponseWriter, r *http.Request, value any, limit int64) bool {
	contentType, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if contentType != "application/json" || r.Header.Get("X-GitMe-Client") != "workspace-v1" {
		http.Error(w, "JSON and X-GitMe-Client: workspace-v1 required", 415)
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if decoder.Decode(value) != nil {
		http.Error(w, "Invalid service request", 400)
		return false
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		http.Error(w, "Expected one JSON object", 400)
		return false
	}
	return true
}
func respondJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func registerProcesses(mux *http.ServeMux, manager *processes.Manager) {
	type selection struct {
		ProjectID string `json:"project_id"`
		ServiceID string `json:"service_id"`
	}
	mux.HandleFunc("GET /services/runs", func(w http.ResponseWriter, r *http.Request) { respondJSON(w, 200, manager.List()) })
	mux.HandleFunc("GET /services/status", func(w http.ResponseWriter, r *http.Request) {
		project, service := r.URL.Query().Get("project_id"), r.URL.Query().Get("service_id")
		if project == "" || service == "" || len(project) > 200 || len(service) > 200 {
			http.Error(w, "Choose a project and service", 400)
			return
		}
		respondJSON(w, 200, manager.Status(project, service))
	})
	mux.HandleFunc("POST /services/prepare", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			selection
			Action string `json:"action"`
		}
		if !processJSON(w, r, &input) {
			return
		}
		plan, err := manager.Prepare(r.Context(), input.ProjectID, input.ServiceID, input.Action)
		if err != nil {
			http.Error(w, err.Error(), 409)
			return
		}
		respondJSON(w, 200, plan)
	})
	mux.HandleFunc("POST /services/run", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Token string `json:"token"`
		}
		if !processJSON(w, r, &input) {
			return
		}
		run, err := manager.Start(r.Context(), input.Token)
		if err != nil {
			http.Error(w, err.Error(), 409)
			return
		}
		respondJSON(w, 201, run)
	})
	mux.HandleFunc("POST /services/stop", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			selection
			RunID string `json:"run_id"`
		}
		if !processJSON(w, r, &input) {
			return
		}
		run, err := manager.Stop(input.ProjectID, input.ServiceID, input.RunID)
		if err != nil {
			http.Error(w, err.Error(), 409)
			return
		}
		respondJSON(w, 200, run)
	})
	mux.HandleFunc("POST /services/health", func(w http.ResponseWriter, r *http.Request) {
		var input selection
		if !processJSON(w, r, &input) {
			return
		}
		health, err := manager.Health(r.Context(), input.ProjectID, input.ServiceID)
		if err != nil {
			http.Error(w, err.Error(), 422)
			return
		}
		respondJSON(w, 200, health)
	})
	for _, path := range []string{"/services/prepare", "/services/run", "/services/stop", "/services/health", "/services/status", "/services/runs"} {
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
