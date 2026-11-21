package discovery

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

func InspectGit(ctx context.Context, folder string) (Git, error) {
	if !filepath.IsAbs(folder) || filepath.Dir(filepath.Clean(folder)) == filepath.Clean(folder) || strings.HasPrefix(folder, `\\`) || strings.HasPrefix(folder, "//") {
		return Git{}, errors.New("choose an absolute local service folder")
	}
	resolved, err := filepath.EvalSymlinks(folder)
	if err != nil || filepath.Dir(resolved) == resolved || strings.HasPrefix(resolved, `\\`) {
		return Git{}, errors.New("service folder was not found or cannot be opened")
	}
	root, err := os.OpenRoot(resolved)
	if err != nil {
		return Git{}, errors.New("service folder cannot be opened")
	}
	defer root.Close()
	s := &scanner{root: root, ctx: ctx}
	result := s.git(".")
	return result, ctx.Err()
}

var commitPattern = regexp.MustCompile(`^(?:[0-9a-fA-F]{40}|[0-9a-fA-F]{64})$`)
var refPattern = regexp.MustCompile(`^[A-Za-z0-9_./-]{1,100}$`)

func safeRef(value string) bool {
	if !refPattern.MatchString(value) || strings.Contains(value, "..") || strings.Contains(value, "//") {
		return false
	}
	for _, part := range strings.Split(value, "/") {
		if part == "" || strings.HasPrefix(part, ".") || strings.HasSuffix(part, ".lock") || strings.HasSuffix(part, ".") {
			return false
		}
	}
	return true
}
func (s *scanner) plainParents(path string) bool {
	for parent := filepath.Dir(path); parent != "."; parent = filepath.Dir(parent) {
		info, err := s.root.Lstat(parent)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return false
		}
	}
	return true
}
