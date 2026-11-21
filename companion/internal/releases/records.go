package releases

import (
	"crypto/sha256"
	"encoding/hex"
	"gitme/companion/internal/vercel"
	"net/url"
	"regexp"
	"strings"
	"time"
)

type Record struct {
	ID             string       `json:"id"`
	ProjectID      string       `json:"project_id"`
	ServiceID      string       `json:"service_id"`
	Repository     string       `json:"repository"`
	Branch         string       `json:"branch"`
	Commit         string       `json:"commit"`
	Source         string       `json:"source"`
	RemoteVerified bool         `json:"remote_verified"`
	CreatedAt      time.Time    `json:"created_at"`
	Target         *vercel.Link `json:"target"`
}

func (r Record) Key() string {
	target := ""
	if r.Target != nil {
		target = r.Target.ConnectionID + "|" + r.Target.TeamID + "|" + r.Target.ProjectID
	}
	sum := sha256.Sum256([]byte(strings.Join([]string{r.ProjectID, r.ServiceID, strings.ToLower(r.Repository), r.Branch, r.Commit, target}, "\x00")))
	return hex.EncodeToString(sum[:])
}

var commitID = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)

func RepositoryName(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host != "github.com" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return ""
	}
	name := strings.TrimSuffix(strings.TrimSuffix(strings.TrimPrefix(u.Path, "/"), "/"), ".git")
	if !regexp.MustCompile(`^[A-Za-z0-9_.-]{1,100}/[A-Za-z0-9_.-]{1,100}$`).MatchString(name) {
		return ""
	}
	for _, part := range strings.Split(name, "/") {
		if part == "." || part == ".." {
			return ""
		}
	}
	return name
}
func (r Record) Valid() bool {
	return r.ProjectID != "" && len(r.ProjectID) <= 200 && r.ServiceID != "" && len(r.ServiceID) <= 200 && RepositoryName(r.Repository) != "" && r.Branch != "" && len(r.Branch) <= 200 && !strings.ContainsAny(r.Branch, "\x00\r\n") && commitID.MatchString(r.Commit) && (r.Source == "push" || r.Source == "observed") && (r.Target == nil || vercel.ValidLink(*r.Target))
}
