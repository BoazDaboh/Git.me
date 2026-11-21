package vercel

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"gitme/companion/internal/localupload"
)

type LocalReview struct {
	Link          Link      `json:"link"`
	Target        string    `json:"target"`
	RootDirectory string    `json:"root_directory"`
	ExpiresAt     time.Time `json:"expires_at"`
	credential    [32]byte
	project       [32]byte
}

func (s *Service) ReviewLocal(ctx context.Context, link Link, target string) (LocalReview, error) {
	if !ValidLink(link) || target != "preview" && target != "production" {
		return LocalReview{}, errors.New("choose a linked Vercel project and Preview or Production")
	}
	token, account, err := s.token(link.ConnectionID)
	if err != nil {
		return LocalReview{}, err
	}
	identity, err := s.user(ctx, token)
	if err != nil {
		return LocalReview{}, err
	}
	if identity.ID != account.ID {
		return LocalReview{}, errors.New("Vercel account identity changed; reconnect it")
	}
	p, err := s.project(ctx, token, account, link.TeamID, link.ProjectID)
	if err != nil {
		return LocalReview{}, err
	}
	if p.ID != link.ProjectID || p.Name != link.ProjectName {
		return LocalReview{}, errors.New("Vercel project changed; reconnect it")
	}
	if p.RootDirectory != "" && p.RootDirectory != "." {
		return LocalReview{}, errors.New("this Vercel project has a nested Root Directory. Local upload currently requires the project root to match the saved service folder; use GitHub deployment for this project")
	}
	if err = s.unchanged(account.ID, token); err != nil {
		return LocalReview{}, err
	}
	return LocalReview{Link: link, Target: target, RootDirectory: p.RootDirectory, ExpiresAt: time.Now().UTC().Add(5 * time.Minute), credential: sha256.Sum256([]byte(token)), project: deploymentProjectKey(p)}, nil
}
func (s *Service) CreateLocal(ctx context.Context, review LocalReview, source localupload.Snapshot, revalidate func() error) DeploymentResult {
	notSent := func(m string) DeploymentResult { return DeploymentResult{Outcome: "not-sent", Message: m} }
	if time.Now().After(review.ExpiresAt) {
		return notSent("Local upload review expired. Review again.")
	}
	current, err := s.ReviewLocal(ctx, review.Link, review.Target)
	if err != nil {
		return notSent(err.Error())
	}
	if current.credential != review.credential || current.project != review.project {
		return notSent("Vercel account or project changed. Review again.")
	}
	if err = revalidate(); err != nil {
		return notSent(err.Error())
	}
	token, _, err := s.token(review.Link.ConnectionID)
	if err != nil || sha256.Sum256([]byte(token)) != review.credential {
		return notSent("Vercel connection changed. Review again.")
	}
	client := *s.client
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	q, _ := scopeQuery(review.Link.TeamID, 0)
	uploaded := false
	for _, file := range source.Files {
		if ctx.Err() != nil || time.Now().After(review.ExpiresAt) {
			return notSent("Upload stopped before a deployment was requested. Some source files may already be stored on Vercel.")
		}
		if err = s.unchanged(review.Link.ConnectionID, token); err != nil {
			return notSent("Vercel connection changed. Source upload stopped.")
		}
		req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.vercel.com/v2/files?"+q.Encode(), bytes.NewReader(file.Data))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/octet-stream")
		req.Header.Set("x-vercel-digest", file.SHA)
		req.ContentLength = int64(file.Size)
		res, e := client.Do(req)
		if e != nil {
			return notSent("Source upload was interrupted. Some files may be stored on Vercel; no deployment request was sent.")
		}
		_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 4096))
		res.Body.Close()
		if res.StatusCode != 200 {
			return notSent(fmt.Sprintf("Vercel stopped source upload (HTTP %d). Some files may be stored; no deployment request was sent.", res.StatusCode))
		}
		uploaded = true
	}
	if !uploaded {
		return notSent("No source files were uploaded.")
	}
	if err = revalidate(); err != nil {
		return notSent("Service settings or files changed during upload. Files may be stored on Vercel; no deployment request was sent. Review again.")
	}
	final, err := s.ReviewLocal(ctx, review.Link, review.Target)
	if err != nil || final.credential != review.credential || final.project != review.project || time.Now().After(review.ExpiresAt) {
		return notSent("Vercel connection, project or review changed during upload. No deployment request was sent.")
	}
	refs := []map[string]any{}
	for _, file := range source.Files {
		refs = append(refs, map[string]any{"file": file.Path, "sha": file.SHA, "size": file.Size})
	}
	payload := map[string]any{"name": review.Link.ProjectName, "project": review.Link.ProjectID, "files": refs}
	if review.Target == "production" {
		payload["target"] = "production"
	}
	body, _ := json.Marshal(payload)
	q.Set("forceNew", "1")
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.vercel.com/v13/deployments?"+q.Encode(), bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	response, err := client.Do(req)
	unknown := DeploymentResult{Outcome: "unconfirmed", Message: "Vercel may have accepted the deployment. Check Activity and Vercel before retrying; Git/Me will not resend it."}
	if err != nil {
		return unknown
	}
	defer response.Body.Close()
	if response.StatusCode != 200 && response.StatusCode != 201 {
		if response.StatusCode >= 400 && response.StatusCode < 500 && response.StatusCode != 408 {
			return DeploymentResult{Outcome: "rejected", Message: fmt.Sprintf("Vercel rejected the deployment (HTTP %d). Uploaded files may remain stored. Check its settings before reviewing again.", response.StatusCode)}
		}
		return unknown
	}
	var data struct {
		ID        string `json:"id"`
		URL       string `json:"url"`
		State     string `json:"readyState"`
		ProjectID string `json:"projectId"`
	}
	raw, e := io.ReadAll(io.LimitReader(response.Body, (4<<20)+1))
	if e != nil || len(raw) > 4<<20 || json.Unmarshal(raw, &data) != nil || !ValidID(data.ID) || !strings.HasPrefix(data.ID, "dpl_") || data.ProjectID != "" && data.ProjectID != review.Link.ProjectID {
		return unknown
	}
	s.mu.Lock()
	s.clearActivity(review.Link.ConnectionID)
	s.mu.Unlock()
	return DeploymentResult{Outcome: "accepted", DeploymentID: data.ID, URL: deploymentURL(data.URL), State: state(data.State), Message: "Vercel accepted the local source deployment. Follow its build in Activity."}
}

type LocalReading struct {
	State      string `json:"state"`
	URL        string `json:"url"`
	Production bool   `json:"production"`
}

func (s *Service) LocalStatus(ctx context.Context, link Link, id string) (LocalReading, error) {
	if !ValidLink(link) || !ValidID(id) || !strings.HasPrefix(id, "dpl_") {
		return LocalReading{}, errors.New("invalid deployment identity")
	}
	token, account, err := s.token(link.ConnectionID)
	if err != nil {
		return LocalReading{}, err
	}
	p, err := s.project(ctx, token, account, link.TeamID, link.ProjectID)
	if err != nil {
		return LocalReading{}, err
	}
	if p.ID != link.ProjectID || p.Name != link.ProjectName {
		return LocalReading{}, errors.New("Vercel project changed")
	}
	q, _ := scopeQuery(link.TeamID, 0)
	var d struct {
		ID        string `json:"id"`
		ProjectID string `json:"projectId"`
		URL       string `json:"url"`
		State     string `json:"readyState"`
	}
	if err = s.get(ctx, token, "/v13/deployments/"+id+"?"+q.Encode(), &d); err != nil {
		return LocalReading{}, err
	}
	if d.ID != id || d.ProjectID != link.ProjectID {
		return LocalReading{}, errors.New("deployment does not belong to the saved project")
	}
	if err = s.unchanged(account.ID, token); err != nil {
		return LocalReading{}, err
	}
	return LocalReading{State: state(d.State), URL: deploymentURL(d.URL), Production: p.Targets["production"].ID == id}, nil
}
