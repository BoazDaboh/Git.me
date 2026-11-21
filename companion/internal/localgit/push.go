package localgit

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

type PushCommit struct {
	SHA   string `json:"sha"`
	Title string `json:"title"`
}
type PushPlan struct {
	Token       string       `json:"token"`
	Repository  string       `json:"repository"`
	URL         string       `json:"url"`
	Branch      string       `json:"branch"`
	Head        string       `json:"head"`
	RemoteHead  string       `json:"remote_head"`
	Commits     []PushCommit `json:"commits"`
	Files       []string     `json:"files"`
	Uncommitted int          `json:"uncommitted"`
	ExpiresAt   time.Time    `json:"expires_at"`
}
type PushResult struct {
	ReleaseID       string `json:"release_id,omitempty"`
	TrackingWarning string `json:"tracking_warning,omitempty"`
	URL             string `json:"url"`
	Branch          string `json:"branch"`
	Commit          string `json:"commit"`
	Verified        bool   `json:"verified"`
	Message         string `json:"message"`
}
type pushRecord struct {
	PushPlan
	selection Selection
	source    snapshot
}
type pushSession struct {
	manager *Manager
	dir     string
	objects string
	helper  string
}

var repoName = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,100}/[A-Za-z0-9_.-]{1,100}$`)

func githubURL(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host != "github.com" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.RawPath != "" {
		return "", errors.New("push currently supports a plain HTTPS github.com repository URL; use the terminal for SSH or other hosts")
	}
	name := strings.TrimSuffix(strings.TrimSuffix(strings.TrimPrefix(u.Path, "/"), "/"), ".git")
	if !repoName.MatchString(name) {
		return "", errors.New("choose a GitHub owner/repository URL without credentials")
	}
	for _, part := range strings.Split(name, "/") {
		if part == "." || part == ".." {
			return "", errors.New("invalid GitHub repository")
		}
	}
	return "https://github.com/" + name + ".git", nil
}

func (m *Manager) pushDestination(ctx context.Context, s snapshot, selection Selection) (string, error) {
	expected := selection.RepositoryURL
	if expected == "" && selection.GitHubRepository != "" {
		expected = "https://github.com/" + selection.GitHubRepository
	}
	expected, err := githubURL(expected)
	if err != nil {
		return "", err
	}
	if selection.GitHubRepository != "" && !strings.EqualFold(strings.TrimSuffix(expected, ".git"), "https://github.com/"+selection.GitHubRepository) {
		return "", errors.New("the saved repository and linked GitHub repository differ; correct the service settings first")
	}
	raw, err := m.run(ctx, s, "", nil, "config", "--null", "--get-all", "remote.origin.url")
	if err != nil {
		return "", errors.New("configure an HTTPS origin for this repository in your terminal first")
	}
	values := bytes.Split(bytes.TrimSuffix(raw, []byte{0}), []byte{0})
	if len(values) != 1 {
		return "", errors.New("multiple origin URLs require terminal handling")
	}
	origin, err := githubURL(string(values[0]))
	if err != nil {
		return "", err
	}
	if !strings.EqualFold(origin, expected) {
		return "", errors.New("local origin does not match this service's saved GitHub repository")
	}
	pushURLs, pushErr := m.run(ctx, s, "", nil, "config", "--null", "--get-all", "remote.origin.pushurl")
	if pushErr == nil {
		values = bytes.Split(bytes.TrimSuffix(pushURLs, []byte{0}), []byte{0})
		if len(values) != 1 {
			return "", errors.New("multiple push destinations require terminal handling")
		}
		pushURL, err := githubURL(string(values[0]))
		if err != nil || !strings.EqualFold(pushURL, origin) {
			return "", errors.New("origin has a different push URL; resolve it in the terminal first")
		}
	}
	return origin, nil
}

func credentialManager(git string) string {
	root := filepath.Dir(filepath.Dir(git))
	for _, sub := range []string{"ucrt64/bin", "mingw64/bin", "mingw32/bin"} {
		candidate := filepath.Join(root, filepath.FromSlash(sub), "git-credential-manager.exe")
		if info, err := os.Stat(candidate); err == nil && info.Mode().IsRegular() {
			return candidate
		}
	}
	return ""
}

func pushEnv(objects string) []string {
	result := []string{}
	for _, entry := range env("") {
		name, _, _ := strings.Cut(entry, "=")
		upper := strings.ToUpper(name)
		if strings.HasPrefix(upper, "GCM_") || strings.HasPrefix(upper, "GIT_") {
			continue
		}
		result = append(result, entry)
	}
	return append(result, "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_ATTR_NOSYSTEM=1", "GIT_TERMINAL_PROMPT=0", "GIT_NO_LAZY_FETCH=1", "GIT_NO_REPLACE_OBJECTS=1", "GIT_ALTERNATE_OBJECT_DIRECTORIES="+strconv.Quote(filepath.ToSlash(objects)), "GCM_INTERACTIVE=never")
}

func (m *Manager) newPushSession(ctx context.Context, s snapshot) (*pushSession, error) {
	dir, err := os.MkdirTemp("", "gitme-push-")
	if err != nil {
		return nil, errors.New("could not create the temporary push context")
	}
	p := &pushSession{manager: m, dir: dir, objects: filepath.Join(s.Repository, ".git", "objects"), helper: credentialManager(m.executable)}
	format := "sha1"
	if s.shaSize == 64 {
		format = "sha256"
	}
	if _, err = p.run(ctx, "init", "--bare", "--template=", "--object-format="+format, dir); err != nil {
		p.close()
		return nil, errors.New("could not prepare isolated Git context")
	}
	return p, nil
}
func (p *pushSession) close() { _ = os.RemoveAll(p.dir) }
func (p *pushSession) run(ctx context.Context, args ...string) ([]byte, error) {
	options := []string{"--git-dir=" + p.dir, "--no-pager", "-c", "core.hooksPath=" + p.manager.hooks, "-c", "core.fsmonitor=false", "-c", "gc.auto=0", "-c", "maintenance.auto=false", "-c", "protocol.allow=never", "-c", "protocol.https.allow=always", "-c", "http.followRedirects=false", "-c", "http.sslVerify=true", "-c", "http.sslBackend=schannel", "-c", "credential.helper=", "-c", "credential.interactive=never", "-c", "push.followTags=false", "-c", "push.gpgSign=false", "-c", "push.recurseSubmodules=no"}
	if p.helper != "" {
		options = append(options, "-c", `credential.helper=!"`+filepath.ToSlash(p.helper)+`"`)
	}
	cmd := exec.Command(p.manager.executable, append(options, args...)...)
	cmd.Dir = p.dir
	cmd.Env = pushEnv(p.objects)
	cmd.WaitDelay = 2 * time.Second
	stdout := &limitBuffer{limit: maxOutput}
	stderr := &limitBuffer{limit: 16 << 10}
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	err := runOwned(ctx, cmd)
	if stdout.overflow || stderr.overflow {
		return nil, errors.New("Git output limit")
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err != nil {
		return nil, err
	}
	return stdout.Bytes(), nil
}
func (m *Manager) network(ctx context.Context, p *pushSession, args ...string) ([]byte, error) {
	if m.networkRunner != nil {
		return m.networkRunner(ctx, p, args...)
	}
	if p.helper == "" {
		return nil, errors.New("install Git for Windows with Git Credential Manager and sign in from your terminal first")
	}
	return p.run(ctx, args...)
}
func (m *Manager) remoteHead(ctx context.Context, p *pushSession, destination, ref string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 18*time.Second)
	defer cancel()
	raw, err := m.network(ctx, p, "ls-remote", "--heads", "--", destination, ref)
	if err != nil {
		return "", errors.New("could not read the GitHub branch using your PC's Git sign-in; check internet, repository access and Git Credential Manager in the terminal")
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return "", nil
	}
	parts := strings.Fields(string(raw))
	if len(parts) != 2 || !shaPattern.MatchString(parts[0]) || parts[1] != ref {
		return "", errors.New("remote branch response could not be verified")
	}
	return parts[0], nil
}
func samePushSource(a, b snapshot) bool {
	return a.Repository == b.Repository && a.scope == b.scope && a.Head == b.Head && a.Branch == b.Branch && a.config == b.config
}

func (m *Manager) PreparePush(ctx context.Context, selection Selection) (PushPlan, error) {
	if !m.pushMu.TryLock() {
		return PushPlan{}, errors.New("another push review or push is running; retry shortly")
	}
	defer m.pushMu.Unlock()
	m.mu.Lock()
	s, err := m.status(ctx, selection)
	count := len(m.pushPlans)
	m.mu.Unlock()
	if err != nil {
		return PushPlan{}, err
	}
	if count >= 8 {
		return PushPlan{}, errors.New("close existing push reviews or wait two minutes")
	}
	if s.Branch == "" || s.Head == "" {
		return PushPlan{}, errors.New("commit on a local branch before reviewing a push")
	}
	if !regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]{0,199}$`).MatchString(s.Branch) {
		return PushPlan{}, errors.New("this branch name requires terminal handling")
	}
	destination, err := m.pushDestination(ctx, s, selection)
	if err != nil {
		return PushPlan{}, err
	}
	p, err := m.newPushSession(ctx, s)
	if err != nil {
		return PushPlan{}, err
	}
	defer p.close()
	ref := "refs/heads/" + s.Branch
	if _, err = p.run(ctx, "check-ref-format", ref); err != nil {
		return PushPlan{}, errors.New("unsupported branch name; use the terminal")
	}
	remote, err := m.remoteHead(ctx, p, destination, ref)
	if err != nil {
		return PushPlan{}, err
	}
	args := []string{"log", "--reverse", "--topo-order", "--max-count=201", "--format=%H%x00%s%x00", s.Head}
	if remote != "" {
		if _, err = p.run(ctx, "cat-file", "-e", remote+"^{commit}"); err != nil {
			return PushPlan{}, errors.New("remote commits are missing locally; run git fetch origin in the terminal, then review again")
		}
		if _, err = p.run(ctx, "merge-base", "--is-ancestor", remote, s.Head); err != nil {
			return PushPlan{}, errors.New("the remote branch has changes outside your local history; pull and resolve them in the terminal before pushing")
		}
		args = append(args, "^"+remote)
	}
	args = append(args, "--")
	raw, err := p.run(ctx, args...)
	if err != nil {
		return PushPlan{}, errors.New("could not inspect outgoing history")
	}
	entries := bytes.Split(raw, []byte{0})
	if (len(entries)-1)/2 > 200 {
		return PushPlan{}, errors.New("more than 200 outgoing commits; review and push this history in the terminal")
	}
	plan := PushPlan{Repository: s.Repository, URL: destination, Branch: s.Branch, Head: s.Head, RemoteHead: remote, Commits: []PushCommit{}, Files: []string{}, Uncommitted: len(s.Files) + s.OutsideStaged, ExpiresAt: time.Now().Add(2 * time.Minute).UTC()}
	for i := 0; i+1 < len(entries); i += 2 {
		id := strings.TrimSpace(string(entries[i]))
		if !shaPattern.MatchString(id) {
			return PushPlan{}, errors.New("invalid outgoing commit")
		}
		plan.Commits = append(plan.Commits, PushCommit{SHA: id, Title: strings.TrimSpace(string(entries[i+1]))})
	}
	if len(plan.Commits) > 0 {
		fileArgs := []string{"log", "--format=", "--name-only", "-z", "--no-renames", "--root", "-m", s.Head}
		if remote != "" {
			fileArgs = append(fileArgs, "^"+remote)
		}
		fileArgs = append(fileArgs, "--")
		paths, err := p.run(ctx, fileArgs...)
		if err != nil {
			return PushPlan{}, errors.New("outgoing file list exceeds the review limit; use the terminal")
		}
		seen := map[string]bool{}
		for _, path := range bytes.Split(paths, []byte{0}) {
			if len(path) == 0 {
				continue
			}
			name := string(path)
			if !validPath(name) {
				return PushPlan{}, errors.New("an outgoing file path requires terminal handling")
			}
			if privatePath(name) != "" {
				return PushPlan{}, errors.New("outgoing history includes a private or generated file path; inspect the committed history in your terminal before pushing")
			}
			seen[name] = true
		}
		if len(seen) > 1000 {
			return PushPlan{}, errors.New("more than 1000 outgoing file paths; use the terminal")
		}
		for path := range seen {
			plan.Files = append(plan.Files, path)
		}
		sort.Strings(plan.Files)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	fresh, err := m.status(ctx, selection)
	if err != nil || !samePushSource(s, fresh) {
		return PushPlan{}, errors.New("repository changed during push review; review again")
	}
	if len(plan.Commits) == 0 {
		return plan, nil
	}
	data := make([]byte, 24)
	if _, err = rand.Read(data); err != nil {
		return PushPlan{}, errors.New("could not create a push review")
	}
	plan.Token = hex.EncodeToString(data)
	m.pushPlans[plan.Token] = pushRecord{PushPlan: plan, selection: selection, source: s}
	token := plan.Token
	time.AfterFunc(2*time.Minute, func() { m.mu.Lock(); delete(m.pushPlans, token); m.mu.Unlock() })
	return plan, nil
}

func (m *Manager) CancelPush(selection Selection, token string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if record, ok := m.pushPlans[token]; ok && record.selection == selection {
		delete(m.pushPlans, token)
	}
}
func (m *Manager) ExecutePush(ctx context.Context, selection Selection, token string) (PushResult, error) {
	if !m.pushMu.TryLock() {
		return PushResult{}, errors.New("another push operation is running")
	}
	defer m.pushMu.Unlock()
	m.mu.Lock()
	record, ok := m.pushPlans[token]
	delete(m.pushPlans, token)
	if !ok || time.Now().After(record.ExpiresAt) || record.selection != selection {
		m.mu.Unlock()
		return PushResult{}, errors.New("push review expired, was already used or the saved service changed; review again")
	}
	current, err := m.status(ctx, selection)
	m.mu.Unlock()
	if err != nil {
		return PushResult{}, err
	}
	if !samePushSource(current, record.source) {
		return PushResult{}, errors.New("branch, commit or repository configuration changed; review again")
	}
	destination, err := m.pushDestination(ctx, current, selection)
	if err != nil || destination != record.URL {
		return PushResult{}, errors.New("push destination changed; review again")
	}
	p, err := m.newPushSession(ctx, current)
	if err != nil {
		return PushResult{}, err
	}
	defer p.close()
	ref := "refs/heads/" + record.Branch
	remote, err := m.remoteHead(ctx, p, destination, ref)
	if err != nil {
		return PushResult{}, err
	}
	if remote != record.RemoteHead {
		return PushResult{}, errors.New("the GitHub branch changed since review; review again")
	}
	if remote != "" {
		if _, err = p.run(ctx, "merge-base", "--is-ancestor", remote, record.Head); err != nil {
			return PushResult{}, errors.New("push would replace remote history; use the terminal to reconcile branches")
		}
	}
	pushCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	_, pushErr := m.network(pushCtx, p, "push", "--porcelain", "--no-verify", "--no-follow-tags", "--recurse-submodules=no", "--signed=false", "--", destination, record.Head+":"+ref)
	cancel()
	if pushErr != nil {
		return PushResult{}, errors.New("push was not confirmed; it may have reached GitHub. Review again to check the branch before retrying. Check PC Git sign-in, branch protection and network access in the terminal")
	}
	verified, verifyErr := m.remoteHead(ctx, p, destination, ref)
	result := PushResult{URL: destination, Branch: record.Branch, Commit: record.Head, Verified: verifyErr == nil && verified == record.Head, Message: fmt.Sprintf("Push accepted for %s at %s.", record.Branch, record.Head[:7])}
	if result.Verified {
		result.Message = "Push complete. GitHub's branch matches the reviewed commit."
	} else {
		result.Message += " A follow-up branch check was unavailable or the branch moved again. Refresh GitHub activity."
	}
	return result, nil
}
