package workspace

import (
	"encoding/json"
	"errors"
	"fmt"
	"gitme/companion/internal/github"
	"net/url"
	"strings"
)

type serviceGitHubLink struct {
	ID           int64  `json:"id"`
	FullName     string `json:"fullName"`
	ConnectionID string `json:"connectionId,omitempty"`
}

func linkIdentity(link serviceGitHubLink) string {
	account := link.ConnectionID
	if account == "" || account == "legacy" {
		account = "legacy"
	}
	return fmt.Sprintf("%s:%d:%s", account, link.ID, strings.ToLower(link.FullName))
}
func serviceRepository(raw string) string {
	name := ""
	if strings.HasPrefix(raw, "git@github.com:") {
		name = strings.TrimPrefix(raw, "git@github.com:")
	} else {
		u, err := url.Parse(raw)
		if err != nil || !strings.EqualFold(u.Hostname(), "github.com") || u.User != nil || u.RawPath != "" {
			return ""
		}
		name = strings.TrimPrefix(u.Path, "/")
	}
	name = strings.TrimSuffix(strings.TrimSuffix(name, "/"), ".git")
	if !github.ValidRepository(name) {
		return ""
	}
	return name
}
func migrateProjectGitHub(raw json.RawMessage) (json.RawMessage, bool, error) {
	var data map[string]json.RawMessage
	if json.Unmarshal(raw, &data) != nil {
		return nil, false, errors.New("invalid workspace")
	}
	var projects []map[string]json.RawMessage
	if json.Unmarshal(data["projects"], &projects) != nil {
		return nil, false, errors.New("invalid projects")
	}
	changed := false
	for _, project := range projects {
		original, exists := project["github"]
		if !exists {
			continue
		}
		if err := validateGitHub(original); err != nil {
			return nil, false, err
		}
		var legacy serviceGitHubLink
		json.Unmarshal(original, &legacy)
		var services []map[string]json.RawMessage
		if value, ok := project["services"]; ok && json.Unmarshal(value, &services) != nil {
			return nil, false, errors.New("invalid services")
		}
		assigned := false
		for _, service := range services {
			if value, ok := service["github"]; ok {
				var current serviceGitHubLink
				if json.Unmarshal(value, &current) != nil {
					return nil, false, errors.New("invalid service GitHub link")
				}
				if linkIdentity(current) == linkIdentity(legacy) {
					assigned = true
				}
				continue
			}
			var repository string
			json.Unmarshal(service["repository"], &repository)
			if strings.EqualFold(serviceRepository(repository), legacy.FullName) {
				service["github"] = original
				assigned = true
			}
		}
		var pending []json.RawMessage
		if value, ok := project["unassignedGitHubLinks"]; ok && json.Unmarshal(value, &pending) != nil {
			return nil, false, errors.New("invalid saved repository links")
		}
		duplicate := false
		for _, value := range pending {
			var current serviceGitHubLink
			if json.Unmarshal(value, &current) != nil {
				return nil, false, errors.New("invalid saved repository link")
			}
			if linkIdentity(current) == linkIdentity(legacy) {
				duplicate = true
			}
		}
		if !assigned && !duplicate {
			pending = append(pending, original)
		}
		if len(pending) > 0 {
			project["unassignedGitHubLinks"], _ = json.Marshal(pending)
		}
		if _, ok := project["services"]; ok {
			project["services"], _ = json.Marshal(services)
		}
		delete(project, "github")
		changed = true
	}
	if !changed {
		return raw, false, nil
	}
	data["projects"], _ = json.Marshal(projects)
	result, err := json.Marshal(data)
	return result, true, err
}
