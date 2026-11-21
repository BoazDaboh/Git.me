package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"gitme/companion/internal/apitester"
	"gitme/companion/internal/workspace"
)

type savedGETCheck struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Method    string `json:"method"`
	URL       string `json:"url"`
	ServiceID string `json:"serviceId"`
	Expected  *int   `json:"expectedStatus"`
}

func savedCheck(ctx context.Context, store *workspace.Store, projectID, requestID string) (savedGETCheck, error) {
	if projectID == "" || len(projectID) > 200 || requestID == "" || len(requestID) > 100 {
		return savedGETCheck{}, errors.New("choose a saved GET request")
	}
	snapshot, err := store.Load(ctx)
	if err != nil {
		return savedGETCheck{}, errors.New("saved requests are unavailable")
	}
	var data struct {
		Projects []struct {
			ID       string          `json:"id"`
			Requests []savedGETCheck `json:"apiRequests"`
			Services []struct {
				ID string `json:"id"`
			} `json:"services"`
		} `json:"projects"`
	}
	if json.Unmarshal(snapshot.Workspace, &data) != nil {
		return savedGETCheck{}, errors.New("save the workspace first")
	}
	for _, project := range data.Projects {
		if project.ID != projectID {
			continue
		}
		for _, request := range project.Requests {
			if request.ID != requestID {
				continue
			}
			if request.Method != "GET" {
				return request, errors.New("POST is excluded from saved GET checks; open it and review separately")
			}
			if request.ServiceID != "" {
				found := false
				for _, service := range project.Services {
					if service.ID == request.ServiceID {
						found = true
					}
				}
				if !found {
					return request, errors.New("the linked service was removed; edit the saved request before running it")
				}
			}
			return request, nil
		}
	}
	return savedGETCheck{}, errors.New("saved request was removed or belongs to another project")
}
func registerAPIChecks(mux *http.ServeMux, store *workspace.Store, requests chan struct{}) {
	mux.HandleFunc("POST /api-test/check", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			ProjectID string `json:"project_id"`
			RequestID string `json:"request_id"`
		}
		if !processJSON(w, r, &input) {
			return
		}
		request, err := savedCheck(r.Context(), store, input.ProjectID, input.RequestID)
		if err != nil {
			http.Error(w, err.Error(), 409)
			return
		}
		select {
		case requests <- struct{}{}:
			defer func() { <-requests }()
		default:
			http.Error(w, "Four API requests are already running. No check was sent.", 429)
			return
		}
		expected := 200
		if request.Expected != nil {
			expected = *request.Expected
		}
		if expected < 200 || expected > 599 {
			http.Error(w, "Invalid expected HTTP status. Edit this request before running it.", 409)
			return
		}
		reading, err := (apitester.Runner{}).Run(r.Context(), apitester.Input{URL: request.URL, Method: "GET", StatusOnly: true})
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		respondJSON(w, 200, struct {
			RequestID string    `json:"request_id"`
			ServiceID string    `json:"service_id"`
			Name      string    `json:"name"`
			URL       string    `json:"url"`
			Expected  int       `json:"expected_status"`
			Actual    int       `json:"actual_status"`
			Passed    bool      `json:"passed"`
			Duration  int64     `json:"duration_ms"`
			Message   string    `json:"message"`
			CheckedAt time.Time `json:"checked_at"`
		}{request.ID, request.ServiceID, request.Name, request.URL, expected, reading.Status, reading.Status == expected, reading.Duration, reading.Message, time.Now().UTC()})
	})
	mux.HandleFunc("OPTIONS /api-test/check", func(w http.ResponseWriter, r *http.Request) {
		if m := r.Header.Get("Access-Control-Request-Method"); m != "" && m != "POST" {
			http.Error(w, "Method not allowed", 405)
			return
		}
		w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-GitMe-Client")
		w.WriteHeader(204)
	})
}
