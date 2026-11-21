package vercel

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

type CommitTracking struct {
	Deployments  []Deployment `json:"deployments"`
	ProductionID string       `json:"production_id"`
	Warnings     []string     `json:"warnings"`
	CheckedAt    time.Time    `json:"checked_at"`
}

func (s *Service) TrackCommit(ctx context.Context, link Link, repository, branch, sha string) (CommitTracking, error) {
	parts := strings.Split(repository, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return CommitTracking{}, errors.New("invalid GitHub repository for tracking")
	}
	if !ValidLink(link) || len(sha) != 40 && len(sha) != 64 || branch == "" || len(branch) > 200 {
		return CommitTracking{}, errors.New("invalid deployment tracking source")
	}
	for _, c := range sha {
		if !strings.ContainsRune("0123456789abcdef", c) {
			return CommitTracking{}, errors.New("invalid tracking commit")
		}
	}
	token, a, err := s.token(link.ConnectionID)
	if err != nil {
		return CommitTracking{}, err
	}
	key := fmt.Sprintf("%s|%s|%s|%s|%s|%s|%x", a.ID, link.TeamID, link.ProjectID, repository, branch, sha, sha256.Sum256([]byte(token)))
	s.mu.Lock()
	cached, exists := s.tracking[key]
	s.mu.Unlock()
	if exists && time.Since(cached.CheckedAt) < 10*time.Second {
		return cached, s.unchanged(a.ID, token)
	}
	p, err := s.project(ctx, token, a, link.TeamID, link.ProjectID)
	if err != nil {
		return CommitTracking{}, err
	}
	if p.ID != link.ProjectID {
		return CommitTracking{}, errors.New("Vercel project identity changed; relink it")
	}
	if p.Link != nil && (p.Link.Type != "github" || !strings.EqualFold(p.Link.Org+"/"+p.Link.Repo, repository)) {
		return CommitTracking{}, errors.New("this Vercel project is linked to a different Git repository")
	}
	q, _ := scopeQuery(link.TeamID, 0)
	q.Set("projectId", link.ProjectID)
	q.Set("sha", sha)
	q.Set("branch", branch)
	q.Set("limit", "20")
	var list struct {
		Deployments []struct {
			UID        string            `json:"uid"`
			URL        string            `json:"url"`
			State      string            `json:"state"`
			ReadyState string            `json:"readyState"`
			Target     string            `json:"target"`
			CreatedAt  int64             `json:"createdAt"`
			Created    int64             `json:"created"`
			Meta       map[string]string `json:"meta"`
		} `json:"deployments"`
	}
	if err = s.get(ctx, token, "/v7/deployments?"+q.Encode(), &list); err != nil {
		return CommitTracking{}, err
	}
	result := CommitTracking{Deployments: []Deployment{}, Warnings: []string{}, CheckedAt: time.Now().UTC()}
	if prod, ok := p.Targets["production"]; ok && ValidID(prod.ID) {
		result.ProductionID = prod.ID
	}
	if p.Link == nil {
		result.Warnings = append(result.Warnings, "Project Git integration was not reported; matches use exact GitHub commit metadata.")
	}
	for _, d := range list.Deployments {
		if !ValidID(d.UID) || d.Meta["githubCommitSha"] != sha || d.Meta["githubCommitRef"] != branch {
			continue
		}
		owner, repo := d.Meta["githubCommitOrg"], d.Meta["githubCommitRepo"]
		if owner != "" && !strings.EqualFold(owner, parts[0]) || repo != "" && !strings.EqualFold(repo, parts[1]) {
			continue
		}
		if d.State == "" {
			d.State = d.ReadyState
		}
		if d.CreatedAt == 0 {
			d.CreatedAt = d.Created
		}
		target := d.Target
		if target == "" {
			target = "preview"
		}
		if target != "preview" && target != "production" {
			target = "custom"
		}
		result.Deployments = append(result.Deployments, Deployment{ID: d.UID, URL: deploymentURL(d.URL), State: state(d.State), Target: target, CreatedAt: d.CreatedAt, Branch: branch, SHA: sha})
	}
	sort.Slice(result.Deployments, func(i, j int) bool { return result.Deployments[i].CreatedAt > result.Deployments[j].CreatedAt })
	if err = s.unchanged(a.ID, token); err != nil {
		return CommitTracking{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, readErr := s.factory(a.ID).Read()
	if readErr != nil || current != token {
		return CommitTracking{}, errors.New("Vercel connection changed; refresh it")
	}
	if len(s.tracking) >= 64 {
		for k := range s.tracking {
			delete(s.tracking, k)
			break
		}
	}
	s.tracking[key] = result
	return result, nil
}
