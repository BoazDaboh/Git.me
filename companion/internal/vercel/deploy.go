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
	"regexp"
	"strings"
	"time"
)

var deploymentCommit = regexp.MustCompile(`^[0-9a-f]{40}$`)
var deploymentRepository = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,100}/[A-Za-z0-9_.-]{1,100}$`)

type DeploymentReview struct {
	Link       Link      `json:"link"`
	Repository string    `json:"repository"`
	Branch     string    `json:"branch"`
	Commit     string    `json:"commit"`
	Target     string    `json:"target"`
	ExpiresAt  time.Time `json:"expires_at"`
	credential [32]byte
	project    [32]byte
}

type DeploymentResult struct {
	Outcome      string `json:"outcome"`
	DeploymentID string `json:"deployment_id"`
	URL          string `json:"url"`
	State        string `json:"state"`
	Message      string `json:"message"`
}

func deploymentProjectKey(p projectResponse) [32]byte {
	raw, _ := json.Marshal(struct {
		Project       Project
		Link          any
		RootDirectory string
	}{p.Project, p.Link, p.RootDirectory})
	return sha256.Sum256(raw)
}

func (s *Service) ReviewDeployment(ctx context.Context, link Link, repository, branch, commit, target string) (DeploymentReview, error) {
	if !ValidLink(link) || !deploymentRepository.MatchString(repository) || strings.Contains(repository, "..") || branch == "" || len(branch) > 200 || strings.ContainsAny(branch, "\x00\r\n") || !deploymentCommit.MatchString(commit) || target != "preview" && target != "production" {
		return DeploymentReview{}, errors.New("choose a recorded GitHub commit and Preview or Production")
	}
	token, account, err := s.token(link.ConnectionID)
	if err != nil {
		return DeploymentReview{}, err
	}
	identity, err := s.user(ctx, token)
	if err != nil {
		return DeploymentReview{}, err
	}
	if identity.ID != account.ID {
		return DeploymentReview{}, errors.New("Vercel account identity changed; reconnect it")
	}
	p, err := s.project(ctx, token, account, link.TeamID, link.ProjectID)
	if err != nil {
		return DeploymentReview{}, err
	}
	if p.ID != link.ProjectID || p.Name != link.ProjectName || p.Link == nil || p.Link.Type != "github" || !strings.EqualFold(p.Link.Org+"/"+p.Link.Repo, repository) {
		return DeploymentReview{}, errors.New("the linked Vercel project must use this exact GitHub repository; review its deployment link")
	}
	if err = s.unchanged(account.ID, token); err != nil {
		return DeploymentReview{}, err
	}
	return DeploymentReview{Link: link, Repository: repository, Branch: branch, Commit: commit, Target: target, ExpiresAt: time.Now().UTC().Add(5 * time.Minute), credential: sha256.Sum256([]byte(token)), project: deploymentProjectKey(p)}, nil
}

// CreateDeployment sends one reviewed request. An uncertain response must never be retried automatically.
func (s *Service) CreateDeployment(ctx context.Context, review DeploymentReview, revalidate func() error) DeploymentResult {
	notSent := func(message string) DeploymentResult { return DeploymentResult{Outcome: "not-sent", Message: message} }
	if time.Now().After(review.ExpiresAt) {
		return notSent("Deployment review expired. Review again.")
	}
	current, err := s.ReviewDeployment(ctx, review.Link, review.Repository, review.Branch, review.Commit, review.Target)
	if err != nil {
		return notSent(err.Error())
	}
	if current.credential != review.credential || current.project != review.project {
		return notSent("Vercel account or project changed. Review again.")
	}
	if err = revalidate(); err != nil {
		return notSent(err.Error())
	}
	if time.Now().After(review.ExpiresAt) || ctx.Err() != nil {
		return notSent("Deployment review expired or stopped before submission. Review again.")
	}
	token, _, err := s.token(review.Link.ConnectionID)
	if err != nil || sha256.Sum256([]byte(token)) != review.credential {
		return notSent("Vercel connection changed. Review again.")
	}
	parts := strings.Split(review.Repository, "/")
	target := ""
	if review.Target == "production" {
		target = "production"
	}
	body, _ := json.Marshal(struct {
		Name      string            `json:"name"`
		Project   string            `json:"project"`
		Target    string            `json:"target,omitempty"`
		GitSource map[string]string `json:"gitSource"`
	}{review.Link.ProjectName, review.Link.ProjectID, target, map[string]string{"type": "github", "org": parts[0], "repo": parts[1], "ref": review.Branch, "sha": review.Commit}})
	q, _ := scopeQuery(review.Link.TeamID, 0)
	q.Set("forceNew", "1")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.vercel.com/v13/deployments?"+q.Encode(), bytes.NewReader(body))
	if err != nil {
		return notSent("Could not prepare the deployment request.")
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "GitMe-local/0.12")
	client := *s.client
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	response, err := client.Do(req)
	unknown := DeploymentResult{Outcome: "unconfirmed", Message: "Vercel may have accepted the request, but confirmation was unavailable. Check Vercel before retrying; Git/Me will not resend it."}
	if err != nil {
		return unknown
	}
	defer response.Body.Close()
	if response.StatusCode != 200 && response.StatusCode != 201 {
		if response.StatusCode >= 400 && response.StatusCode < 500 && response.StatusCode != 408 {
			message := fmt.Sprintf("Vercel rejected the deployment request (HTTP %d). Check the project and token permissions, then review again.", response.StatusCode)
			if response.StatusCode == 429 {
				message = "Vercel's deployment limit was reached. Wait before reviewing another request."
			}
			return DeploymentResult{Outcome: "rejected", Message: message}
		}
		return unknown
	}
	var data struct {
		ID        string `json:"id"`
		URL       string `json:"url"`
		State     string `json:"readyState"`
		ProjectID string `json:"projectId"`
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, (4<<20)+1))
	if err != nil || len(raw) > 4<<20 || json.Unmarshal(raw, &data) != nil || !ValidID(data.ID) || !strings.HasPrefix(data.ID, "dpl_") || data.ProjectID != "" && data.ProjectID != review.Link.ProjectID {
		return unknown
	}
	s.mu.Lock()
	s.clearActivity(review.Link.ConnectionID)
	s.mu.Unlock()
	return DeploymentResult{Outcome: "accepted", DeploymentID: data.ID, URL: deploymentURL(data.URL), State: state(data.State), Message: "Vercel accepted the request. Follow this deployment ID in Deployment tracking; acceptance does not mean the build is live."}
}
