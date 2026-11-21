package localupload

import (
	"context"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

const MaxFiles = 2000
const MaxBytes = 32 << 20
const MaxFileBytes = 8 << 20

type File struct {
	Path string `json:"path"`
	Size int    `json:"size"`
	SHA  string `json:"sha"`
	Data []byte `json:"-"`
}
type Exclusion struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}
type Snapshot struct {
	Folder        string      `json:"folder"`
	Files         []File      `json:"files"`
	Bytes         int         `json:"bytes"`
	Digest        string      `json:"digest"`
	Excluded      []Exclusion `json:"excluded"`
	ExcludedCount int         `json:"excluded_count"`
}
type rule struct {
	base      string
	pattern   string
	anchored  bool
	directory bool
}

var secret = regexp.MustCompile(`(?i)(-----BEGIN [A-Z ]*PRIVATE KEY-----|\b(?:gh[pousr]_[a-z0-9_]{20,}|github_pat_[a-z0-9_]{20,}|sb_secret_[a-z0-9_-]{12,}|AKIA[A-Z0-9]{16})\b|(?:api[_-]?key|secret[_-]?key|access[_-]?token|password)\s*["']?\s*[:=]\s*["'][^"'\r\n\s]{8,}["'])`)
var excludedDirectories = map[string]bool{".git": true, ".hg": true, ".svn": true, ".ssh": true, ".aws": true, ".azure": true, ".kube": true, ".vercel": true, ".next": true, ".nuxt": true, ".output": true, "node_modules": true, "vendor": true, "dist": true, "build": true, "out": true, "coverage": true, ".turbo": true, ".cache": true, ".idea": true, ".vscode": true, "venv": true, ".venv": true, "__pycache__": true, "backups": true, "secrets": true, "credentials": true}

func private(name string, dir bool) bool {
	n := strings.ToLower(name)
	if dir {
		return excludedDirectories[n] || strings.HasPrefix(n, ".env")
	}
	if strings.HasPrefix(n, ".env") || n == ".npmrc" || n == ".yarnrc" || n == ".yarnrc.yml" || n == ".netrc" || n == ".gitconfig" || n == ".git-credentials" || n == "credentials" || n == "id_rsa" || n == "id_ed25519" || strings.Contains(n, "service-account") || strings.Contains(n, "serviceaccount") || strings.Contains(n, "credential") || strings.Contains(n, "secret") {
		return true
	}
	for _, ext := range []string{".pem", ".key", ".p12", ".pfx", ".keystore", ".jks", ".sqlite", ".sqlite3", ".db", ".sql", ".dump", ".bak", ".log", ".exe", ".dll", ".tsbuildinfo", ".zip", ".tgz", ".tar", ".gz", ".sqlite-journal", ".sqlite-wal", ".sqlite-shm", ".sqlite3-journal", ".sqlite3-wal", ".sqlite3-shm", ".db-journal", ".db-wal", ".db-shm"} {
		if strings.HasSuffix(n, ext) {
			return true
		}
	}
	return false
}

// Ignore files add exclusions. Reinclusions cannot undo a safety or earlier ignore exclusion.
func rules(raw []byte, base string) ([]rule, error) {
	result := []rule{}
	for _, line := range strings.Split(string(raw), "\n") {
		p := strings.TrimSpace(line)
		if p == "" || strings.HasPrefix(p, "#") || strings.HasPrefix(p, "!") {
			continue
		}
		if strings.Contains(p, "\\") {
			return nil, errors.New("escaped ignore patterns are not supported for local upload; use the GitHub flow or simplify the ignore file")
		}
		if len(p) > 500 || strings.Count(p, "/") > 64 {
			return nil, errors.New("ignore pattern limit exceeded")
		}
		r := rule{base: base, directory: strings.HasSuffix(p, "/"), anchored: strings.HasPrefix(p, "/")}
		r.pattern = strings.Trim(p, "/")
		if r.pattern == "" {
			continue
		}
		for _, segment := range strings.Split(r.pattern, "/") {
			if segment != "**" {
				if _, err := path.Match(segment, ""); err != nil {
					return nil, errors.New("an ignore pattern could not be read safely")
				}
			}
		}
		result = append(result, r)
		if len(result) > 2000 {
			return nil, errors.New("ignore rule limit exceeded")
		}
	}
	return result, nil
}
func match(pattern, name []string) bool {
	row := make([]bool, len(name)+1)
	row[0] = true
	for _, part := range pattern {
		next := make([]bool, len(name)+1)
		if part == "**" {
			next[0] = row[0]
			for j := 1; j <= len(name); j++ {
				next[j] = row[j] || next[j-1]
			}
		} else {
			for j := 1; j <= len(name); j++ {
				ok, _ := path.Match(part, name[j-1])
				next[j] = row[j-1] && ok
			}
		}
		row = next
	}
	return row[len(name)]
}
func ignored(rs []rule, name string, dir bool) bool {
	for _, r := range rs {
		rel := name
		if r.base != "" {
			if !strings.HasPrefix(name, r.base+"/") {
				continue
			}
			rel = strings.TrimPrefix(name, r.base+"/")
		}
		if r.directory && !dir {
			continue
		}
		if !r.anchored && !strings.Contains(r.pattern, "/") {
			if ok, _ := path.Match(r.pattern, path.Base(rel)); ok {
				return true
			}
		} else if match(strings.Split(r.pattern, "/"), strings.Split(rel, "/")) {
			return true
		}
	}
	return false
}
func localFolder(folder string) (string, error) {
	if !filepath.IsAbs(folder) || strings.HasPrefix(folder, `\\`) || strings.HasPrefix(folder, "//") || filepath.Dir(filepath.Clean(folder)) == filepath.Clean(folder) {
		return "", errors.New("choose an absolute local service folder, not a drive or network root")
	}
	folder = filepath.Clean(folder)
	for current := folder; filepath.Dir(current) != current; current = filepath.Dir(current) {
		info, err := os.Lstat(current)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return "", errors.New("local upload requires an accessible folder without linked parent directories")
		}
	}
	return folder, nil
}
func Read(ctx context.Context, projectFolder, folder string) (Snapshot, error) {
	folder, err := localFolder(folder)
	if err != nil {
		return Snapshot{}, err
	}
	base, err := localFolder(projectFolder)
	if err != nil {
		return Snapshot{}, err
	}
	rel, err := filepath.Rel(base, folder)
	if err != nil || !filepath.IsLocal(rel) {
		return Snapshot{}, errors.New("the service folder must be inside its saved project folder")
	}
	root, err := os.OpenRoot(folder)
	if err != nil {
		return Snapshot{}, errors.New("service folder cannot be opened")
	}
	defer root.Close()
	result := Snapshot{Folder: folder, Files: []File{}, Excluded: []Exclusion{}}
	visited := 0
	exclude := func(p, reason string) {
		result.ExcludedCount++
		result.Excluded = append(result.Excluded, Exclusion{p, reason})
	}
	read := func(p string, limit int) ([]byte, error) {
		info, e := root.Lstat(p)
		if e != nil || !info.Mode().IsRegular() || info.Size() > int64(limit) {
			return nil, errors.New("file is linked, unreadable or exceeds the upload limit")
		}
		f, e := root.Open(p)
		if e != nil {
			return nil, errors.New("file cannot be read")
		}
		defer f.Close()
		opened, e := f.Stat()
		if e != nil || !os.SameFile(info, opened) || !singleFile(f) {
			return nil, errors.New("file changed during review")
		}
		raw, e := io.ReadAll(io.LimitReader(f, int64(limit)+1))
		if e != nil || len(raw) > limit {
			return nil, errors.New("file exceeds the upload limit")
		}
		return raw, nil
	}
	var walk func(string, []rule, int) error
	walk = func(dir string, inherited []rule, depth int) error {
		if ctx.Err() != nil {
			return errors.New("file review stopped or timed out")
		}
		if depth > 32 {
			return errors.New("folder depth limit exceeded")
		}
		rs := append([]rule{}, inherited...)
		for _, name := range []string{".gitignore", ".vercelignore"} {
			p := path.Join(dir, name)
			if _, e := root.Lstat(p); os.IsNotExist(e) {
				continue
			}
			raw, e := read(p, 64<<10)
			if e != nil {
				return fmt.Errorf("%s cannot be read safely", p)
			}
			add, e := rules(raw, dir)
			if e != nil {
				return e
			}
			rs = append(rs, add...)
			if len(rs) > 4000 {
				return errors.New("ignore rule limit exceeded")
			}
		}
		d, e := root.Open(func() string {
			if dir == "" {
				return "."
			}
			return dir
		}())
		if e != nil {
			return errors.New("folder cannot be read")
		}
		defer d.Close()
		for {
			entries, e := d.ReadDir(128)
			if e != nil && e != io.EOF {
				return errors.New("folder cannot be listed")
			}
			for _, entry := range entries {
				visited++
				if visited > 10000 {
					return errors.New("file review limit exceeded; choose a smaller service")
				}
				if ctx.Err() != nil {
					return errors.New("file review stopped or timed out")
				}
				name := entry.Name()
				p := path.Join(dir, name)
				if len(p) > 500 || strings.ContainsAny(p, "\x00\r\n\\:") {
					return errors.New("a file path is not supported for upload")
				}
				info, e := root.Lstat(p)
				if e != nil {
					return errors.New("file list changed during review")
				}
				if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() && !info.Mode().IsRegular() {
					exclude(p, "Linked or special file")
					continue
				}
				if private(name, info.IsDir()) {
					exclude(p, "Private or generated")
					continue
				}
				if ignored(rs, p, info.IsDir()) {
					exclude(p, "Ignore rule")
					continue
				}
				if info.IsDir() {
					if e := walk(p, rs, depth+1); e != nil {
						return e
					}
					continue
				}
				raw, e := read(p, MaxFileBytes)
				if e != nil {
					return fmt.Errorf("%s: %s", p, e.Error())
				}
				if secret.Match(raw) {
					exclude(p, "Possible credential content")
					continue
				}
				result.Bytes += len(raw)
				if len(result.Files) >= MaxFiles || result.Bytes > MaxBytes {
					return errors.New("local upload supports up to 2,000 files, 32 MiB total and 8 MiB per file")
				}
				sum := sha1.Sum(raw)
				result.Files = append(result.Files, File{Path: p, Size: len(raw), SHA: hex.EncodeToString(sum[:]), Data: raw})
			}
			if e == io.EOF {
				break
			}
		}
		return nil
	}
	if err = walk("", nil, 0); err != nil {
		return Snapshot{}, err
	}
	if len(result.Files) == 0 {
		return Snapshot{}, errors.New("no uploadable source files were found")
	}
	// Directory enumeration order must not affect review identity.
	sortFiles(&result)
	raw, _ := json.Marshal(result.Files)
	digest := sha256.New()
	digest.Write(raw)
	for _, file := range result.Files {
		digest.Write(file.Data)
	}
	result.Digest = hex.EncodeToString(digest.Sum(nil))
	if len(result.Excluded) > 300 {
		result.Excluded = result.Excluded[:300]
	}
	return result, nil
}
