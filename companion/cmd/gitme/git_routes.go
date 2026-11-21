package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"gitme/companion/internal/localgit"
	"gitme/companion/internal/workspace"
)

func gitSelection(ctx context.Context, store *workspace.Store, projectID, serviceID string) (localgit.Selection, error) {
	if projectID == "" || serviceID == "" || len(projectID) > 200 || len(serviceID) > 200 {
		return localgit.Selection{}, errors.New("choose a saved service")
	}
	snapshot, err := store.Load(ctx)
	if err != nil {
		return localgit.Selection{}, errors.New("could not read saved service")
	}
	var data struct {
		Projects []struct {
			ID       string `json:"id"`
			Folder   string `json:"folder"`
			Services []struct {
				ID         string `json:"id"`
				Folder     string `json:"folder"`
				Repository string `json:"repository"`
				GitHub     struct {
					FullName     string `json:"fullName"`
					ConnectionID string `json:"connectionId"`
				} `json:"github"`
			} `json:"services"`
		} `json:"projects"`
	}
	if json.Unmarshal(snapshot.Workspace, &data) != nil {
		return localgit.Selection{}, errors.New("save the workspace first")
	}
	for _, project := range data.Projects {
		if project.ID != projectID {
			continue
		}
		for _, service := range project.Services {
			if service.ID == serviceID {
				return localgit.Selection{ProjectID: projectID, ServiceID: serviceID, ProjectFolder: project.Folder, Folder: service.Folder, RepositoryURL: service.Repository, GitHubRepository: service.GitHub.FullName, GitHubConnection: service.GitHub.ConnectionID}, nil
			}
		}
		break
	}
	return localgit.Selection{}, errors.New("saved service was not found")
}

func registerLocalGit(mux *http.ServeMux, store *workspace.Store, manager *localgit.Manager) {
	type input struct {
		ProjectID string   `json:"project_id"`
		ServiceID string   `json:"service_id"`
		Version   string   `json:"version"`
		Path      string   `json:"path"`
		Staged    bool     `json:"staged"`
		Action    string   `json:"action"`
		Message   string   `json:"message"`
		Paths     []string `json:"paths"`
		Token     string   `json:"token"`
	}
	for _, action := range []string{"status", "diff", "prepare", "execute", "cancel", "push-review", "push-execute", "push-cancel"} {
		mux.HandleFunc("POST /git/"+action, func(w http.ResponseWriter, r *http.Request) {
			var request input
			if !processJSONLimit(w, r, &request, 16<<10) {
				return
			}
			duration := 20 * time.Second
			if action == "push-review" || action == "push-execute" {
				duration = 100 * time.Second
			}
			ctx, cancel := context.WithTimeout(r.Context(), duration)
			defer cancel()
			selection, err := gitSelection(ctx, store, request.ProjectID, request.ServiceID)
			if err != nil {
				http.Error(w, err.Error(), 409)
				return
			}
			var result any
			switch action {
			case "status":
				result, err = manager.Status(ctx, selection)
			case "diff":
				result, err = manager.Diff(ctx, selection, request.Version, request.Path, request.Staged)
			case "prepare":
				result, err = manager.Prepare(ctx, selection, request.Version, request.Action, request.Message, request.Paths)
			case "execute":
				result, err = manager.Execute(ctx, selection, request.Token)
			case "cancel":
				manager.Cancel(selection, request.Token)
				result = map[string]bool{"canceled": true}
			case "push-review":
				result, err = manager.PreparePush(ctx, selection)
			case "push-execute":
				target, _ := releaseTarget(ctx, store, request.ProjectID, request.ServiceID)
				var pushed localgit.PushResult
				pushed, err = manager.ExecutePush(ctx, selection, request.Token)
				if err == nil {
					savePushRecord(ctx, store, selection, target, &pushed)
				}
				result = pushed
			case "push-cancel":
				manager.CancelPush(selection, request.Token)
				result = map[string]bool{"canceled": true}
			}
			if err != nil {
				http.Error(w, err.Error(), 409)
				return
			}
			respondJSON(w, 200, result)
		})
		mux.HandleFunc("OPTIONS /git/"+action, func(w http.ResponseWriter, r *http.Request) {
			if method := r.Header.Get("Access-Control-Request-Method"); method != "" && method != "POST" {
				http.Error(w, "Method not allowed", 405)
				return
			}
			w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-GitMe-Client")
			w.WriteHeader(204)
		})
	}
}
