package releases

import (
	"gitme/companion/internal/vercel"
	"time"
)

type DeploymentAction struct {
	ID          string                  `json:"id"`
	Release     Record                  `json:"release"`
	Environment string                  `json:"environment"`
	CreatedAt   time.Time               `json:"created_at"`
	Result      vercel.DeploymentResult `json:"result"`
	Local       *LocalSource            `json:"local,omitempty"`
}

type LocalSource struct {
	ProjectID string      `json:"project_id"`
	ServiceID string      `json:"service_id"`
	Target    vercel.Link `json:"target"`
	Folder    string      `json:"folder"`
	Digest    string      `json:"digest"`
	Files     int         `json:"files"`
	Bytes     int         `json:"bytes"`
}

func (a DeploymentAction) Scope() (string, string) {
	if a.Local != nil {
		return a.Local.ProjectID, a.Local.ServiceID
	}
	return a.Release.ProjectID, a.Release.ServiceID
}
func (a DeploymentAction) Valid() bool {
	if a.Local == nil {
		return a.Release.Valid() && a.Release.Target != nil
	}
	l := a.Local
	return l.ProjectID != "" && len(l.ProjectID) <= 200 && l.ServiceID != "" && len(l.ServiceID) <= 200 && vercel.ValidLink(l.Target) && l.Folder != "" && len(l.Digest) == 64 && l.Files > 0 && l.Files <= 2000 && l.Bytes >= 0 && l.Bytes <= 32<<20
}
