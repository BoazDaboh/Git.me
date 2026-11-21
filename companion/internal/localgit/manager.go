package localgit

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"gitme/companion/internal/releases"
)

const maxOutput = 1 << 20
const maxIndex = 16 << 20
const maxFile = 2 << 20
const maxPatch = 64 << 10

type Selection struct {
	ProjectID        string `json:"project_id"`
	ServiceID        string `json:"service_id"`
	ProjectFolder    string `json:"-"`
	Folder           string `json:"-"`
	RepositoryURL    string `json:"-"`
	GitHubRepository string `json:"-"`
	GitHubConnection string `json:"-"`
}
type File struct {
	Path     string `json:"path"`
	Index    string `json:"index"`
	Worktree string `json:"worktree"`
	Blocked  string `json:"blocked"`
	Mode     string `json:"-"`
}
type Status struct {
	Folder        string    `json:"folder"`
	Repository    string    `json:"repository"`
	Branch        string    `json:"branch"`
	Head          string    `json:"head"`
	Version       string    `json:"version"`
	Files         []File    `json:"files"`
	OutsideStaged int       `json:"outside_staged"`
	CheckedAt     time.Time `json:"checked_at"`
}
type Diff struct {
	Path      string `json:"path"`
	Staged    bool   `json:"staged"`
	Text      string `json:"text"`
	Truncated bool   `json:"truncated"`
	Binary    bool   `json:"binary"`
}
type Plan struct {
	Token     string    `json:"token"`
	Action    string    `json:"action"`
	Branch    string    `json:"branch"`
	Files     []string  `json:"files"`
	Message   string    `json:"message,omitempty"`
	Author    string    `json:"author,omitempty"`
	Diff      string    `json:"diff"`
	Truncated bool      `json:"truncated"`
	ExpiresAt time.Time `json:"expires_at"`
}
type Result struct {
	Action string `json:"action"`
	Branch string `json:"branch"`
	Commit string `json:"commit,omitempty"`
	Status Status `json:"status"`
}
type snapshot struct {
	Status
	index   []byte
	config  string
	scope   string
	args    []string
	shaSize int
}
type planRecord struct {
	Plan
	selection   Selection
	snapshot    snapshot
	content     map[string][]byte
	deleted     map[string]bool
	attrs       string
	authorName  string
	authorEmail string
}
type Manager struct {
	mu            sync.Mutex
	executable    string
	hooks         string
	plans         map[string]planRecord
	pushMu        sync.Mutex
	pushPlans     map[string]pushRecord
	networkRunner func(context.Context, *pushSession, ...string) ([]byte, error)
}

func New() *Manager {
	executable := ""
	candidates := []string{filepath.Join(os.Getenv("ProgramFiles"), "Git", "cmd", "git.exe"), filepath.Join(os.Getenv("ProgramFiles"), "Git", "bin", "git.exe")}
	if path, err := exec.LookPath("git"); err == nil {
		candidates = append(candidates, path)
	}
	for _, path := range candidates {
		if !filepath.IsAbs(path) {
			continue
		}
		info, err := os.Stat(path)
		if err == nil && info.Mode().IsRegular() {
			executable = path
			break
		}
	}
	hooks, _ := os.MkdirTemp("", "gitme-git-hooks-")
	return &Manager{executable: executable, hooks: hooks, plans: map[string]planRecord{}, pushPlans: map[string]pushRecord{}}
}
func (m *Manager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.plans = map[string]planRecord{}
	m.pushPlans = map[string]pushRecord{}
	if m.hooks != "" {
		_ = os.Remove(m.hooks)
	}
}
func env(index string) []string {
	out := []string{}
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		if !strings.HasPrefix(strings.ToUpper(name), "GIT_") {
			out = append(out, entry)
		}
	}
	out = append(out, "GIT_TERMINAL_PROMPT=0", "GIT_OPTIONAL_LOCKS=0", "GIT_NO_LAZY_FETCH=1", "GIT_NO_REPLACE_OBJECTS=1", "GIT_PAGER=cat")
	if index != "" {
		out = append(out, "GIT_INDEX_FILE="+index)
	}
	return out
}

type limitBuffer struct {
	bytes.Buffer
	limit    int
	overflow bool
}

func (b *limitBuffer) Write(p []byte) (int, error) {
	n := len(p)
	if b.Len()+n > b.limit {
		remaining := b.limit - b.Len()
		if remaining > 0 {
			b.Buffer.Write(p[:remaining])
		}
		b.overflow = true
		return n, errors.New("Git output limit")
	}
	return b.Buffer.Write(p)
}
func (m *Manager) run(ctx context.Context, s snapshot, index string, input []byte, args ...string) ([]byte, error) {
	all := append(append([]string{}, s.args...), args...)
	cmd := exec.CommandContext(ctx, m.executable, all...)
	cmd.Dir = s.Repository
	cmd.Env = env(index)
	hide(cmd)
	cmd.Stdin = bytes.NewReader(input)
	output := &limitBuffer{limit: maxOutput}
	stderr := &limitBuffer{limit: 4096}
	cmd.Stdout = output
	cmd.Stderr = stderr
	err := cmd.Run()
	if output.overflow || stderr.overflow {
		return nil, errors.New("Git output exceeded the local limit; use a smaller repository or the terminal")
	}
	if ctx.Err() != nil {
		return nil, errors.New("Git operation timed out or stopped")
	}
	if err != nil {
		return nil, errors.New("Git could not complete this operation; check repository state and local Git configuration")
	}
	return output.Bytes(), nil
}
func baseArgs(root, hooks string) []string {
	return []string{"--literal-pathspecs", "--no-optional-locks", "--git-dir=" + filepath.Join(root, ".git"), "--work-tree=" + root, "-c", "core.hooksPath=" + hooks, "-c", "core.fsmonitor=false", "-c", "core.untrackedCache=false", "-c", "core.bare=false", "-c", "gc.auto=0", "-c", "maintenance.auto=false", "-c", "commit.gpgSign=false", "-c", "core.pager=", "-c", "diff.external="}
}

var configKey = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,200}$`)
var shaPattern = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)

func readFile(root, path string, limit int) ([]byte, error) {
	r, err := os.OpenRoot(root)
	if err != nil {
		return nil, errors.New("repository folder cannot be opened")
	}
	defer r.Close()
	parts := strings.Split(filepath.ToSlash(path), "/")
	current := ""
	for i, part := range parts {
		current = filepath.Join(current, part)
		info, err := r.Lstat(current)
		if err != nil {
			return nil, err
		}
		if info.Mode()&os.ModeSymlink != 0 || i < len(parts)-1 && !info.IsDir() {
			return nil, errors.New("linked paths must be handled in the terminal")
		}
		if i == len(parts)-1 && (!info.Mode().IsRegular() || info.Size() > int64(limit)) {
			return nil, errors.New("file is not regular or exceeds the local size limit")
		}
	}
	f, err := r.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, int64(limit+1)))
	if err != nil || len(raw) > limit {
		return nil, errors.New("file exceeds the local read limit")
	}
	return raw, nil
}
func indexBytes(root string) ([]byte, error) {
	raw, err := readFile(root, filepath.Join(".git", "index"), maxIndex)
	if os.IsNotExist(err) {
		return nil, nil
	}
	return raw, err
}
func (m *Manager) resolve(ctx context.Context, selection Selection) (snapshot, error) {
	if m.executable == "" || m.hooks == "" {
		return snapshot{}, errors.New("Git is unavailable; install Git and restart the companion")
	}
	inspected := releases.Inspect(ctx, selection.ProjectFolder, selection.Folder)
	if !inspected.FolderReady {
		return snapshot{}, fmt.Errorf("saved service folder is unavailable: %s. Check this service's folder in Source & local setup", inspected.FolderError)
	}
	if inspected.RepositoryFolder == "" {
		return snapshot{}, errors.New("no local Git repository was found for this saved service; check its folder in Source & local setup and use the folder containing .git, or a service inside that repository")
	}
	root, err := filepath.EvalSymlinks(inspected.RepositoryFolder)
	if err != nil {
		return snapshot{}, errors.New("repository folder cannot be resolved")
	}
	scope, err := filepath.EvalSymlinks(selection.Folder)
	if err != nil {
		return snapshot{}, errors.New("service folder cannot be resolved")
	}
	rel, err := filepath.Rel(root, scope)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return snapshot{}, errors.New("service folder is outside its repository")
	}
	for _, path := range []string{filepath.Join(root, ".git"), filepath.Join(root, ".git", "objects"), filepath.Join(root, ".git", "refs")} {
		info, err := os.Lstat(path)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return snapshot{}, errors.New("linked Git metadata is not supported; use the terminal")
		}
	}
	for _, path := range []string{"commondir", "objects/info/alternates", "objects/info/http-alternates"} {
		raw, err := readFile(root, filepath.Join(".git", path), 4096)
		if err != nil && !os.IsNotExist(err) {
			return snapshot{}, errors.New("shared or unreadable Git metadata requires terminal handling")
		}
		if len(bytes.TrimSpace(raw)) > 0 {
			return snapshot{}, errors.New("shared or alternate Git object stores require terminal handling")
		}
	}
	s := snapshot{Status: Status{Folder: scope, Repository: root, Files: []File{}, CheckedAt: time.Now().UTC()}, scope: filepath.ToSlash(rel), args: baseArgs(root, m.hooks), shaSize: 40}
	if s.scope == "." {
		s.scope = ""
	}
	keys, err := m.run(ctx, s, "", nil, "config", "--name-only", "--get-regexp", `^filter\..*\.(clean|process|required)$`)
	if err == nil {
		for _, key := range strings.Fields(string(keys)) {
			if !configKey.MatchString(key) {
				return snapshot{}, errors.New("unsupported Git filter configuration")
			}
			value := ""
			if strings.HasSuffix(key, ".required") {
				value = "false"
			}
			s.args = append(s.args, "-c", key+"="+value)
		}
	}
	config, err := m.run(ctx, s, "", nil, "config", "--null", "--list")
	if err != nil {
		return snapshot{}, err
	}
	s.config = hash(config)
	branch, err := m.run(ctx, s, "", nil, "symbolic-ref", "--quiet", "--short", "HEAD")
	if err == nil {
		s.Branch = strings.TrimSpace(string(branch))
	}
	head, err := m.run(ctx, s, "", nil, "rev-parse", "--verify", "HEAD")
	if err == nil {
		s.Head = strings.TrimSpace(string(head))
		if !shaPattern.MatchString(s.Head) {
			return snapshot{}, errors.New("Git HEAD could not be verified")
		}
		s.shaSize = len(s.Head)
	} else {
		format, _ := m.run(ctx, s, "", nil, "rev-parse", "--show-object-format")
		if strings.TrimSpace(string(format)) == "sha256" {
			s.shaSize = 64
		}
	}
	s.index, err = indexBytes(root)
	if err != nil {
		return snapshot{}, errors.New("Git index cannot be read safely")
	}
	return s, nil
}
func hash(raw []byte) string { h := sha256.Sum256(raw); return hex.EncodeToString(h[:]) }
func privatePath(path string) string {
	for _, part := range strings.Split(strings.ToLower(path), "/") {
		if part == ".git" || part == ".ssh" || part == ".aws" || part == ".kube" || part == ".vercel" || part == "node_modules" || part == "vendor" || part == ".next" {
			return "Private or generated files are handled in the terminal."
		}
	}
	name := strings.ToLower(filepath.Base(path))
	if name != ".env.example" && name != ".env.sample" && (name == ".env" || strings.HasPrefix(name, ".env.") || name == ".envrc") {
		return "Private environment files cannot be previewed or staged here."
	}
	for _, suffix := range []string{".pem", ".key", ".p12", ".pfx", ".ppk", ".kdbx", ".sqlite", ".sqlite3", ".db"} {
		if strings.HasSuffix(name, suffix) {
			return "Private key or data files are handled in the terminal."
		}
	}
	if strings.HasPrefix(name, "id_rsa") || strings.HasPrefix(name, "id_ed25519") || name == "credentials.json" || strings.Contains(name, "service-account") || strings.Contains(name, "serviceaccount") {
		return "Credential files are handled in the terminal."
	}
	return ""
}
func inScope(scope, path string) bool { return scope == "" || strings.HasPrefix(path, scope+"/") }
func validPath(path string) bool {
	if path == "" || len(path) > 512 || !utf8.ValidString(path) || strings.ContainsAny(path, "\x00\r\n\\") || filepath.IsAbs(path) {
		return false
	}
	for _, part := range strings.Split(path, "/") {
		if part == "" || part == "." || part == ".." {
			return false
		}
	}
	return true
}
func (m *Manager) attributes(ctx context.Context, s snapshot, paths []string) (map[string]string, string, error) {
	if len(paths) == 0 {
		return map[string]string{}, "", nil
	}
	input := []byte(strings.Join(paths, "\x00") + "\x00")
	raw, err := m.run(ctx, s, "", input, "check-attr", "-z", "--stdin", "filter")
	if err != nil {
		return nil, "", err
	}
	parts := bytes.Split(raw, []byte{0})
	result := map[string]string{}
	for i := 0; i+2 < len(parts); i += 3 {
		result[string(parts[i])] = string(parts[i+2])
	}
	return result, hash(raw), nil
}
func (m *Manager) status(ctx context.Context, selection Selection) (snapshot, error) {
	s, err := m.resolve(ctx, selection)
	if err != nil {
		return s, err
	}
	raw, err := m.run(ctx, s, "", nil, "status", "--porcelain=v1", "-z", "--no-renames", "--untracked-files=all", "--ignore-submodules=all")
	if err != nil {
		return s, err
	}
	entries := bytes.Split(raw, []byte{0})
	if len(entries) > 501 {
		return s, errors.New("more than 500 changed paths; use a smaller service or the terminal")
	}
	modes := map[string]string{}
	listed, err := m.run(ctx, s, "", nil, "ls-files", "--stage", "-z")
	if err != nil {
		return s, err
	}
	for _, entry := range bytes.Split(listed, []byte{0}) {
		meta, path, ok := strings.Cut(string(entry), "\t")
		if !ok {
			continue
		}
		parts := strings.Fields(meta)
		if len(parts) == 3 {
			modes[path] = parts[0]
			if parts[2] != "0" {
				return s, errors.New("resolve Git merge conflicts in the terminal first")
			}
		}
	}
	paths := []string{}
	for _, entry := range entries {
		if len(entry) == 0 {
			continue
		}
		if len(entry) < 4 {
			return s, errors.New("Git returned an invalid status")
		}
		path := string(entry[3:])
		if !validPath(path) {
			return s, errors.New("a changed path is not supported by Git/Me; use the terminal")
		}
		staged := entry[0] != ' ' && entry[0] != '?'
		if !inScope(s.scope, path) {
			if staged {
				s.OutsideStaged++
			}
			continue
		}
		file := File{Path: path, Index: string(entry[0]), Worktree: string(entry[1]), Mode: modes[path], Blocked: privatePath(path)}
		if file.Mode != "" && file.Mode != "100644" && file.Mode != "100755" {
			file.Blocked = "Symlinks and submodules require terminal handling."
		}
		s.Files = append(s.Files, file)
		if file.Blocked == "" {
			paths = append(paths, path)
		}
	}
	attrs, _, err := m.attributes(ctx, s, paths)
	if err != nil {
		return s, err
	}
	for i, file := range s.Files {
		if value := attrs[file.Path]; value != "" && value != "unspecified" && value != "unset" {
			s.Files[i].Blocked = "This file uses a Git filter (for example LFS); use the terminal."
		}
	}
	identity := fmt.Sprintf("%s\x00%s\x00%s\x00%s\x00%s\x00%s\x00%s", selection.ProjectID, selection.ServiceID, s.Repository, s.scope, s.Branch, s.Head, s.config)
	s.Version = hash(append(append([]byte(identity), raw...), s.index...))
	return s, nil
}
func (m *Manager) Status(ctx context.Context, selection Selection) (Status, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, err := m.status(ctx, selection)
	return s.Status, err
}
func find(s snapshot, path string) (File, error) {
	for _, file := range s.Files {
		if file.Path == path {
			if file.Blocked != "" {
				return file, errors.New(file.Blocked)
			}
			return file, nil
		}
	}
	return File{}, errors.New("this path is no longer changed; refresh Git status")
}
func capPatch(raw []byte) (string, bool) {
	truncated := len(raw) > maxPatch
	if truncated {
		raw = raw[:maxPatch]
	}
	text := strings.ToValidUTF8(string(raw), "�")
	text = strings.ReplaceAll(text, "\x1b", "?")
	return text, truncated
}
func (m *Manager) diff(ctx context.Context, s snapshot, path string, staged bool) (Diff, error) {
	file, err := find(s, path)
	if err != nil {
		return Diff{}, err
	}
	result := Diff{Path: path, Staged: staged}
	if file.Index == "?" {
		if staged {
			return result, errors.New("this file is not staged")
		}
		raw, err := readFile(s.Repository, path, maxFile)
		if err != nil {
			return result, err
		}
		if bytes.IndexByte(raw, 0) >= 0 || !utf8.Valid(raw) {
			result.Binary = true
			return result, nil
		}
		patch := []byte("--- /dev/null\n+++ b/" + path + "\n")
		for _, line := range strings.Split(string(raw), "\n") {
			patch = append(patch, []byte("+"+line+"\n")...)
			if len(patch) > maxPatch {
				break
			}
		}
		result.Text, result.Truncated = capPatch(patch)
		return result, nil
	}
	args := []string{"diff", "--no-ext-diff", "--no-textconv", "--no-color", "--no-renames", "--ignore-submodules=all"}
	if staged {
		args = append(args, "--cached")
	}
	args = append(args, "--", path)
	raw, err := m.run(ctx, s, "", nil, args...)
	if err != nil {
		return result, err
	}
	result.Text, result.Truncated = capPatch(raw)
	result.Binary = bytes.Contains(raw, []byte("Binary files"))
	return result, nil
}
func (m *Manager) Diff(ctx context.Context, selection Selection, version, path string, staged bool) (Diff, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, err := m.status(ctx, selection)
	if err != nil {
		return Diff{}, err
	}
	if s.Version != version {
		return Diff{}, errors.New("repository changed; refresh Git status")
	}
	return m.diff(ctx, s, path, staged)
}

func (m *Manager) Prepare(ctx context.Context, selection Selection, version, action, message string, paths []string) (Plan, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	for token, record := range m.plans {
		if now.After(record.ExpiresAt) {
			delete(m.plans, token)
		}
	}
	if len(m.plans) >= 8 {
		return Plan{}, errors.New("too many open Git reviews; close them or wait two minutes")
	}
	s, err := m.status(ctx, selection)
	if err != nil {
		return Plan{}, err
	}
	if s.Version != version {
		return Plan{}, errors.New("repository changed; refresh Git status")
	}
	if s.Branch == "" {
		return Plan{}, errors.New("detached HEAD is read-only here; switch branches in the terminal")
	}
	if action != "stage" && action != "unstage" && action != "commit" {
		return Plan{}, errors.New("choose stage, unstage or commit")
	}
	record := planRecord{selection: selection, snapshot: s, content: map[string][]byte{}, deleted: map[string]bool{}}
	record.Action = action
	record.Branch = s.Branch
	record.Message = strings.TrimSpace(message)
	record.ExpiresAt = now.Add(2 * time.Minute)
	if action == "commit" {
		if s.OutsideStaged > 0 {
			return Plan{}, errors.New("files outside this service are staged; handle the repository-wide commit in the terminal")
		}
		if record.Message == "" || len(record.Message) > 2000 || strings.ContainsRune(record.Message, 0) {
			return Plan{}, errors.New("enter a commit message of up to 2000 characters")
		}
		paths = nil
		for _, file := range s.Files {
			if file.Index != " " && file.Index != "?" {
				paths = append(paths, file.Path)
			}
		}
		author, err := m.run(ctx, s, "", nil, "var", "GIT_AUTHOR_IDENT")
		if err != nil {
			return Plan{}, errors.New("configure Git user.name and user.email in the terminal first")
		}
		ident := strings.TrimSpace(string(author))
		end := strings.LastIndex(ident, ">")
		start := strings.LastIndex(ident, "<")
		if start < 1 || end < start {
			return Plan{}, errors.New("Git author identity could not be verified")
		}
		record.authorName = strings.TrimSpace(ident[:start])
		record.authorEmail = ident[start+1 : end]
		record.Author = ident[:end+1]
	} else {
		record.Message = ""
	}
	limit := 24
	if action == "commit" {
		limit = 500
	}
	if len(paths) == 0 || len(paths) > limit {
		return Plan{}, errors.New("choose staged changes to commit, or 1 to 24 files to stage/unstage")
	}
	sort.Strings(paths)
	seen := map[string]bool{}
	total := 0
	var patch strings.Builder
	for _, path := range paths {
		if seen[path] {
			return Plan{}, errors.New("duplicate selected file")
		}
		seen[path] = true
		file, err := find(s, path)
		if err != nil {
			return Plan{}, err
		}
		if action == "unstage" && (file.Index == " " || file.Index == "?") {
			return Plan{}, errors.New("choose staged files to unstage")
		}
		if action == "stage" {
			if file.Index != "?" && file.Worktree == " " {
				return Plan{}, errors.New("choose working-tree changes to stage")
			}
			raw, err := readFile(s.Repository, path, maxFile)
			if os.IsNotExist(err) && file.Worktree == "D" {
				record.deleted[path] = true
			} else if err != nil {
				return Plan{}, errors.New("selected file is linked, unreadable or too large; use the terminal")
			} else {
				total += len(raw)
				if total > 8<<20 {
					return Plan{}, errors.New("selected content exceeds 8 MiB; choose fewer files")
				}
				record.content[path] = raw
			}
		}
		if patch.Len() < maxPatch {
			diff, err := m.diff(ctx, s, path, action != "stage")
			if err != nil {
				return Plan{}, err
			}
			patch.WriteString(diff.Text)
			record.Truncated = record.Truncated || diff.Truncated
		} else {
			record.Truncated = true
		}
	}
	record.Files = append([]string(nil), paths...)
	text, truncated := capPatch([]byte(patch.String()))
	record.Diff = text
	record.Truncated = record.Truncated || truncated
	_, record.attrs, err = m.attributes(ctx, s, paths)
	if err != nil {
		return Plan{}, err
	}
	if action == "stage" {
		current, err := m.status(ctx, selection)
		if err != nil || current.Version != s.Version {
			return Plan{}, errors.New("repository changed while preparing the review; refresh")
		}
		for path, raw := range record.content {
			value, err := readFile(s.Repository, path, maxFile)
			if err != nil || !bytes.Equal(value, raw) {
				return Plan{}, errors.New("selected content changed while preparing the review; refresh")
			}
		}
	}
	data := make([]byte, 24)
	if _, err := rand.Read(data); err != nil {
		return Plan{}, errors.New("Git review could not be created")
	}
	record.Token = hex.EncodeToString(data)
	m.plans[record.Token] = record
	token := record.Token
	time.AfterFunc(2*time.Minute, func() { m.mu.Lock(); delete(m.plans, token); m.mu.Unlock() })
	return record.Plan, nil
}

func (m *Manager) Cancel(selection Selection, token string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if record, ok := m.plans[token]; ok && record.selection == selection {
		delete(m.plans, token)
	}
}

func (m *Manager) Execute(ctx context.Context, selection Selection, token string) (Result, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	record, ok := m.plans[token]
	delete(m.plans, token)
	if !ok || time.Now().After(record.ExpiresAt) {
		return Result{}, errors.New("Git review expired or was already used; review again")
	}
	if record.selection != selection {
		return Result{}, errors.New("saved service changed; review again")
	}
	current, err := m.status(ctx, selection)
	if err != nil {
		return Result{}, err
	}
	if current.Version != record.snapshot.Version {
		return Result{}, errors.New("repository changed since review; review again")
	}
	_, attrs, err := m.attributes(ctx, current, record.Files)
	if err != nil || attrs != record.attrs {
		return Result{}, errors.New("Git attributes changed; review again")
	}
	for path, raw := range record.content {
		now, err := readFile(current.Repository, path, maxFile)
		if err != nil || !bytes.Equal(now, raw) {
			return Result{}, errors.New("a selected file changed since review; review again")
		}
	}
	for path := range record.deleted {
		if _, err := readFile(current.Repository, path, maxFile); !os.IsNotExist(err) {
			return Result{}, errors.New("a selected file changed since review; review again")
		}
	}
	file, err := os.CreateTemp(filepath.Join(current.Repository, ".git"), "gitme-index-")
	if err != nil {
		return Result{}, errors.New("temporary Git index could not be created")
	}
	alternate := file.Name()
	defer os.Remove(alternate)
	defer os.Remove(alternate + ".lock")
	if len(current.index) > 0 {
		_, err = file.Write(current.index)
	}
	closeErr := file.Close()
	if err != nil || closeErr != nil {
		return Result{}, errors.New("temporary Git index could not be written")
	}
	if len(current.index) == 0 {
		if err = os.Remove(alternate); err != nil {
			return Result{}, err
		}
	}
	if record.Action == "stage" {
		var info bytes.Buffer
		for _, path := range record.Files {
			if record.deleted[path] {
				fmt.Fprintf(&info, "0 %s\t%s\x00", strings.Repeat("0", current.shaSize), path)
				continue
			}
			blob, err := m.run(ctx, current, "", record.content[path], "hash-object", "-w", "--stdin", "--path="+path)
			if err != nil {
				return Result{}, err
			}
			id := strings.TrimSpace(string(blob))
			if !shaPattern.MatchString(id) {
				return Result{}, errors.New("staged content could not be verified")
			}
			mode := "100644"
			entry, _ := find(current, path)
			if entry.Mode == "100755" {
				mode = "100755"
			}
			fmt.Fprintf(&info, "%s %s\t%s\x00", mode, id, path)
		}
		if _, err = m.run(ctx, current, alternate, info.Bytes(), "update-index", "-z", "--index-info"); err != nil {
			return Result{}, err
		}
	} else if record.Action == "unstage" {
		args := []string{"reset", "--quiet", "HEAD", "--"}
		if current.Head == "" {
			args = []string{"update-index", "--force-remove", "--"}
		}
		args = append(args, record.Files...)
		if _, err = m.run(ctx, current, alternate, nil, args...); err != nil {
			return Result{}, err
		}
	}
	lockPath := filepath.Join(current.Repository, ".git", "index.lock")
	lock, err := os.OpenFile(lockPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return Result{}, errors.New("Git is busy in another terminal; retry after it finishes")
	}
	ownedLock := true
	defer func() {
		if ownedLock {
			lock.Close()
			os.Remove(lockPath)
		}
	}()
	fresh, err := m.status(ctx, selection)
	if err != nil || fresh.Version != current.Version {
		return Result{}, errors.New("repository changed during the operation; review again")
	}
	result := Result{Action: record.Action, Branch: current.Branch}
	if record.Action == "commit" {
		tree, err := m.run(ctx, current, alternate, nil, "write-tree")
		if err != nil {
			return Result{}, err
		}
		treeID := strings.TrimSpace(string(tree))
		if !shaPattern.MatchString(treeID) {
			return Result{}, errors.New("staged tree could not be verified")
		}
		args := []string{"commit-tree", treeID}
		if current.Head != "" {
			args = append(args, "-p", current.Head)
		}
		args = append(args, "-F", "-")
		commit, err := m.run(ctx, current, alternate, []byte(record.Message+"\n"), args...)
		if err != nil {
			return Result{}, err
		}
		id := strings.TrimSpace(string(commit))
		if !shaPattern.MatchString(id) {
			return Result{}, errors.New("commit object could not be verified")
		}
		old := current.Head
		if old == "" {
			old = strings.Repeat("0", len(id))
		}
		subject := strings.Split(record.Message, "\n")[0]
		if _, err = m.run(ctx, current, "", nil, "update-ref", "-m", "commit (GitMe): "+subject, "refs/heads/"+current.Branch, id, old); err != nil {
			return Result{}, errors.New("branch changed before the commit was saved; refresh Git status before retrying")
		}
		result.Commit = id
	} else {
		raw, err := readFile(current.Repository, filepath.Join(".git", filepath.Base(alternate)), maxIndex)
		if err != nil {
			return Result{}, err
		}
		if _, err = lock.Write(raw); err != nil {
			return Result{}, errors.New("Git index could not be saved")
		}
		if err = lock.Sync(); err != nil {
			return Result{}, err
		}
		if err = lock.Close(); err != nil {
			return Result{}, err
		}
		if err = os.Rename(lockPath, filepath.Join(current.Repository, ".git", "index")); err != nil {
			return Result{}, errors.New("Git index could not be replaced; working files were not changed")
		}
		ownedLock = false
	}
	if ownedLock {
		lock.Close()
		os.Remove(lockPath)
		ownedLock = false
	}
	updated, err := m.status(ctx, selection)
	if err != nil {
		return result, errors.New("Git action completed, but status refresh failed; check the terminal before retrying")
	}
	result.Status = updated.Status
	return result, nil
}
