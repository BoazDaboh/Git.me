package releases

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"gitme/companion/internal/discovery"
)

type Config struct {
	Route  string `json:"route"`
	Target string `json:"target"`
	Branch string `json:"branch"`
}

func Validate(raw json.RawMessage) error {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || len(fields) != 3 {
		return errors.New("invalid release workflow")
	}
	for _, key := range []string{"route", "target", "branch"} {
		var value *string
		if json.Unmarshal(fields[key], &value) != nil || value == nil {
			return errors.New("invalid release workflow")
		}
	}
	var config Config
	if json.Unmarshal(raw, &config) != nil {
		return errors.New("invalid release workflow")
	}
	if config.Route != "github" && config.Route != "vercel-remote" && config.Route != "vercel-local" {
		return errors.New("choose a supported release workflow")
	}
	if config.Target != "preview" && config.Target != "production" {
		return errors.New("choose preview or production")
	}
	if len(config.Branch) > 100 || strings.ContainsAny(config.Branch, "\x00\r\n\t \\~^:?*[") || strings.Contains(config.Branch, "..") || strings.Contains(config.Branch, "@{") || strings.HasPrefix(config.Branch, "-") {
		return errors.New("invalid release branch")
	}
	return nil
}

type Inspection struct {
	Folder           string        `json:"folder"`
	FolderReady      bool          `json:"folder_ready"`
	FolderError      string        `json:"folder_error"`
	RepositoryFolder string        `json:"repository_folder"`
	Git              discovery.Git `json:"git"`
	GitLocated       bool          `json:"git_located"`
	VercelLocated    bool          `json:"vercel_located"`
	CheckedAt        time.Time     `json:"checked_at"`
}

func located(name string) bool {
	path, err := exec.LookPath(name)
	if err != nil || !filepath.IsAbs(path) {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}
func Inspect(ctx context.Context, projectFolder, folder string) Inspection {
	result := Inspection{Folder: folder, GitLocated: located("git"), VercelLocated: located("vercel"), CheckedAt: time.Now().UTC()}
	git, err := discovery.InspectGit(ctx, folder)
	if err != nil {
		result.FolderError = err.Error()
		return result
	}
	result.FolderReady = true
	result.Git = git
	if git.Found {
		result.RepositoryFolder = folder
		return result
	}
	base, err := filepath.EvalSymlinks(projectFolder)
	if err != nil || !filepath.IsAbs(base) || filepath.Dir(base) == base {
		return result
	}
	current, err := filepath.EvalSymlinks(folder)
	if err != nil {
		return result
	}
	rel, err := filepath.Rel(base, current)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return result
	}
	for i := 0; i < 12 && current != base; i++ {
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
		git, err = discovery.InspectGit(ctx, current)
		if err != nil {
			break
		}
		if git.Found {
			result.Git = git
			result.RepositoryFolder = current
			break
		}
	}
	return result
}
