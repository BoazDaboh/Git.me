package main

import (
	"context"
	"encoding/json"
	"errors"
	"gitme/companion/internal/localgit"
	"gitme/companion/internal/releases"
	"gitme/companion/internal/vercel"
	"gitme/companion/internal/workspace"
	"net/http"
	"strings"
	"time"
)

func releaseTarget(ctx context.Context, store *workspace.Store, projectID, serviceID string) (*vercel.Link, error) {
	snapshot, err := store.Load(ctx)
	if err != nil {
		return nil, err
	}
	var data struct {
		Projects []struct {
			ID       string `json:"id"`
			Services []struct {
				ID     string       `json:"id"`
				Vercel *vercel.Link `json:"vercel"`
			} `json:"services"`
		} `json:"projects"`
	}
	if json.Unmarshal(snapshot.Workspace, &data) != nil {
		return nil, errors.New("save the workspace first")
	}
	for _, p := range data.Projects {
		if p.ID == projectID {
			for _, s := range p.Services {
				if s.ID == serviceID {
					return s.Vercel, nil
				}
			}
		}
	}
	return nil, errors.New("saved service was not found")
}
func savePushRecord(ctx context.Context, store *workspace.Store, selection localgit.Selection, target *vercel.Link, result *localgit.PushResult) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	defer cancel()
	record, err := store.RecordRelease(ctx, releases.Record{ProjectID: selection.ProjectID, ServiceID: selection.ServiceID, Repository: result.URL, Branch: result.Branch, Commit: result.Commit, Source: "push", RemoteVerified: result.Verified, Target: target})
	if err != nil {
		result.TrackingWarning = "Push was accepted, but its tracking record could not be saved. Use Track current GitHub commit after storage recovers."
		return
	}
	result.ReleaseID = record.ID
}

type trackedRelease struct {
	releases.Record
	State   string                 `json:"state"`
	Message string                 `json:"message"`
	Reading *vercel.CommitTracking `json:"reading"`
}
type releaseSummary struct {
	ID        string    `json:"id"`
	Commit    string    `json:"commit"`
	Branch    string    `json:"branch"`
	Source    string    `json:"source"`
	CreatedAt time.Time `json:"created_at"`
}

func sameTarget(a, b *vercel.Link) bool {
	return a != nil && b != nil && a.ConnectionID == b.ConnectionID && a.TeamID == b.TeamID && a.ProjectID == b.ProjectID
}
func registerDeploymentTracking(mux *http.ServeMux, store *workspace.Store, git *localgit.Manager, cloud *vercel.Service) {
	mux.HandleFunc("POST /releases/track-current", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			ProjectID string `json:"project_id"`
			ServiceID string `json:"service_id"`
		}
		if !processJSON(w, r, &input) {
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 100*time.Second)
		defer cancel()
		selection, err := gitSelection(ctx, store, input.ProjectID, input.ServiceID)
		if err != nil {
			http.Error(w, err.Error(), 409)
			return
		}
		target, err := releaseTarget(ctx, store, input.ProjectID, input.ServiceID)
		if err != nil {
			http.Error(w, "could not load service tracking settings", 409)
			return
		}
		plan, err := git.PreparePush(ctx, selection)
		if err != nil {
			http.Error(w, err.Error(), 409)
			return
		}
		if plan.Token != "" {
			git.CancelPush(selection, plan.Token)
		}
		if plan.Head != plan.RemoteHead || len(plan.Commits) > 0 {
			http.Error(w, "the current branch has unpushed commits; push them first, then track the GitHub commit", 409)
			return
		}
		current, err := gitSelection(ctx, store, input.ProjectID, input.ServiceID)
		if err != nil || current != selection {
			http.Error(w, "the saved service changed; retry tracking", 409)
			return
		}
		record, err := store.RecordRelease(ctx, releases.Record{ProjectID: input.ProjectID, ServiceID: input.ServiceID, Repository: plan.URL, Branch: plan.Branch, Commit: plan.Head, Source: "observed", RemoteVerified: true, Target: target})
		if err != nil {
			http.Error(w, "could not save the tracking record", 500)
			return
		}
		respondJSON(w, 200, record)
	})
	mux.HandleFunc("GET /releases/tracking", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 18*time.Second)
		defer cancel()
		projectID, serviceID := r.URL.Query().Get("project_id"), r.URL.Query().Get("service_id")
		selection, err := gitSelection(ctx, store, projectID, serviceID)
		if err != nil {
			http.Error(w, err.Error(), 409)
			return
		}
		target, err := releaseTarget(ctx, store, projectID, serviceID)
		if err != nil {
			http.Error(w, "could not read the saved Vercel link", 409)
			return
		}
		records, err := store.Releases(ctx, projectID, serviceID)
		if err != nil {
			http.Error(w, "could not read release history", 500)
			return
		}
		history := []releaseSummary{}
		var record *releases.Record
		requested := r.URL.Query().Get("release_id")
		for i := range records {
			v := records[i]
			history = append(history, releaseSummary{v.ID, v.Commit, v.Branch, v.Source, v.CreatedAt})
			if requested == v.ID || requested == "" && i == 0 {
				record = &records[i]
			}
		}
		if requested != "" && record == nil {
			http.Error(w, "release record was not found for this service", 404)
			return
		}
		var view *trackedRelease
		if record != nil {
			view = &trackedRelease{Record: *record, State: "waiting", Message: "Waiting for a deployment with this exact commit and branch."}
			source := selection.RepositoryURL
			if source == "" && selection.GitHubRepository != "" {
				source = "https://github.com/" + selection.GitHubRepository
			}
			switch {
			case !strings.EqualFold(releases.RepositoryName(source), releases.RepositoryName(record.Repository)) || releases.RepositoryName(source) == "":
				view.State = "source-changed"
				view.Message = "This service now points to another repository. Track its current GitHub commit explicitly."
			case record.Target == nil:
				view.State = "unlinked"
				view.Message = "No Vercel project was linked to this service for this record. Link one below, then track the current GitHub commit."
				if target != nil {
					view.Message = "This service is now linked. Use Track current GitHub commit to create a release record for its deployment target."
				}
			case !sameTarget(record.Target, target):
				view.State = "link-changed"
				view.Message = "The service's Vercel account, team or project changed. This record stays bound to its original target. Track the current GitHub commit to use the new link."
			default:
				reading, err := cloud.TrackCommit(ctx, *record.Target, releases.RepositoryName(record.Repository), record.Branch, record.Commit)
				if err != nil {
					view.State = "unavailable"
					view.Message = err.Error()
				} else {
					currentTarget, checkErr := releaseTarget(ctx, store, projectID, serviceID)
					latest, sourceErr := gitSelection(ctx, store, projectID, serviceID)
					if checkErr != nil || sourceErr != nil || !sameTarget(record.Target, currentTarget) || latest != selection {
						view.State = "link-changed"
						view.Message = "Service settings changed while tracking. Refresh to load the current link."
					} else {
						view.Reading = &reading
						if len(reading.Deployments) > 0 {
							view.State = "matched"
							view.Message = "Matched the exact GitHub commit and branch."
						} else if time.Since(record.CreatedAt) > 10*time.Minute {
							view.State = "not-observed"
							view.Message = "No matching deployment observed. Automatic deployment may be disabled, delayed or skipped; check Vercel's Git settings."
						}
					}
				}
			}
		}
		respondJSON(w, 200, struct {
			History []releaseSummary `json:"history"`
			Release *trackedRelease  `json:"release"`
		}{history, view})
	})
	for _, path := range []string{"tracking", "track-current"} {
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
