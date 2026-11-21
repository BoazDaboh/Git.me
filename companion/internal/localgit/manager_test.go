package localgit

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func fixture(t *testing.T) (*Manager, Selection) {
	t.Helper()
	m := New()
	if m.executable == "" {
		t.Fatal("Git is required for local Git tests")
	}
	t.Cleanup(m.Close)
	root := t.TempDir()
	selection := Selection{ProjectID: "test", ServiceID: "api", ProjectFolder: root, Folder: root}
	git(t, m, root, "init", "-b", "main")
	git(t, m, root, "config", "user.name", "GitMe fixture")
	git(t, m, root, "config", "user.email", "fixture@example.test")
	git(t, m, root, "config", "core.autocrlf", "false")
	return m, selection
}
func git(t *testing.T, m *Manager, root string, args ...string) string {
	t.Helper()
	cmd := exec.Command(m.executable, args...)
	cmd.Dir = root
	cmd.Env = append(env(""), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull)
	hide(cmd)
	raw, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("fixture Git command failed: %s", raw)
	}
	return string(raw)
}
func put(t *testing.T, root, path, text string) {
	t.Helper()
	full := filepath.Join(root, path)
	if err := os.MkdirAll(filepath.Dir(full), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestSavedServiceFolderErrorsExplainRecovery(t *testing.T) {
	m := New()
	defer m.Close()
	root := t.TempDir()
	selection := Selection{ProjectID: "test", ServiceID: "web", ProjectFolder: root, Folder: filepath.Join(root, "missing")}
	if _, err := m.Status(t.Context(), selection); err == nil || !strings.Contains(err.Error(), "saved service folder is unavailable") {
		t.Fatal("missing folder error lacks context", err)
	}
	selection.Folder = root
	if _, err := m.Status(t.Context(), selection); err == nil || !strings.Contains(err.Error(), "no local Git repository was found") {
		t.Fatal("non-repository error lacks context", err)
	}
}
func perform(t *testing.T, m *Manager, s Selection, action, message string, paths ...string) Result {
	t.Helper()
	status, err := m.Status(t.Context(), s)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := m.Prepare(t.Context(), s, status.Version, action, message, paths)
	if err != nil {
		t.Fatal(err)
	}
	result, err := m.Execute(t.Context(), s, plan.Token)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = m.Execute(t.Context(), s, plan.Token); err == nil {
		t.Fatal("review token replay accepted")
	}
	return result
}

func TestSelectedStageUnstageAndReviewedCommit(t *testing.T) {
	m, s := fixture(t)
	put(t, s.Folder, "one.txt", "one\n")
	put(t, s.Folder, "two.txt", "two\n")
	result := perform(t, m, s, "stage", "", "one.txt")
	if len(result.Status.Files) != 2 {
		t.Fatal("status omitted changed files")
	}
	if got := git(t, m, s.Folder, "diff", "--cached", "--name-only"); strings.TrimSpace(got) != "one.txt" {
		t.Fatal("unselected file was staged")
	}
	perform(t, m, s, "unstage", "", "one.txt")
	if got := git(t, m, s.Folder, "diff", "--cached", "--name-only"); got != "" {
		t.Fatal("file was not unstaged")
	}
	if raw, _ := os.ReadFile(filepath.Join(s.Folder, "one.txt")); string(raw) != "one\n" {
		t.Fatal("unstage changed working content")
	}
	perform(t, m, s, "stage", "", "one.txt")
	result = perform(t, m, s, "commit", "feat: first local commit")
	if result.Commit == "" || strings.TrimSpace(git(t, m, s.Folder, "rev-parse", "HEAD")) != result.Commit {
		t.Fatal("commit was not saved")
	}
	if strings.TrimSpace(git(t, m, s.Folder, "show", "HEAD:one.txt")) != "one" {
		t.Fatal("wrong staged content committed")
	}
	put(t, s.Folder, "one.txt", "staged\n")
	perform(t, m, s, "stage", "", "one.txt")
	put(t, s.Folder, "one.txt", "later unstaged edit\n")
	result = perform(t, m, s, "commit", "fix: staged snapshot only")
	if strings.TrimSpace(git(t, m, s.Folder, "show", "HEAD:one.txt")) != "staged" {
		t.Fatal("unstaged working content was committed")
	}
	if raw, _ := os.ReadFile(filepath.Join(s.Folder, "one.txt")); string(raw) != "later unstaged edit\n" {
		t.Fatal("commit overwrote the working file")
	}
	if _, err := os.Stat(filepath.Join(s.Folder, ".git", "index.lock")); !os.IsNotExist(err) {
		t.Fatal("Git lock was left behind")
	}
}

func TestStaleReviewPrivateFilesAndServiceScope(t *testing.T) {
	m, s := fixture(t)
	put(t, s.Folder, "web/file.txt", "first\n")
	put(t, s.Folder, "api/file.txt", "other\n")
	put(t, s.Folder, "web/.env", "PRIVATE_VALUE=do-not-preview")
	s.Folder = filepath.Join(s.ProjectFolder, "web")
	status, err := m.Status(t.Context(), s)
	if err != nil {
		t.Fatal(err)
	}
	if len(status.Files) != 2 {
		t.Fatal("other service files leaked into this service list")
	}
	if _, err = m.Diff(t.Context(), s, status.Version, "web/.env", false); err == nil {
		t.Fatal("private file preview allowed")
	}
	if _, err = m.Prepare(t.Context(), s, status.Version, "stage", "", []string{"web/.env"}); err == nil {
		t.Fatal("private file staging allowed")
	}
	plan, err := m.Prepare(t.Context(), s, status.Version, "stage", "", []string{"web/file.txt"})
	if err != nil {
		t.Fatal(err)
	}
	put(t, s.Folder, "file.txt", "changed after review\n")
	if _, err = m.Execute(t.Context(), s, plan.Token); err == nil {
		t.Fatal("stale working content review accepted")
	}
	perform(t, m, s, "stage", "", "web/file.txt")
	git(t, m, s.ProjectFolder, "add", "api/file.txt")
	status, err = m.Status(t.Context(), s)
	if err != nil || status.OutsideStaged != 1 {
		t.Fatal("outside-service staging not detected")
	}
	if _, err = m.Prepare(t.Context(), s, status.Version, "commit", "do not include other service", nil); err == nil {
		t.Fatal("commit included another service's staged file")
	}
}

func TestGitFiltersAndHooksDoNotRun(t *testing.T) {
	m, s := fixture(t)
	put(t, s.Folder, "normal.txt", "normal\n")
	put(t, s.Folder, "filtered.txt", "original before filter\n")
	perform(t, m, s, "stage", "", "normal.txt", "filtered.txt")
	perform(t, m, s, "commit", "initial")
	marker := filepath.ToSlash(filepath.Join(s.Folder, "executed"))
	put(t, s.Folder, ".git/hooks/pre-commit", "#!/bin/sh\necho hook > '"+marker+"'\n")
	os.Chmod(filepath.Join(s.Folder, ".git/hooks/pre-commit"), 0700)
	git(t, m, s.Folder, "config", "filter.attack.clean", "sh -c \"echo filter > '"+marker+"'; cat\"")
	git(t, m, s.Folder, "config", "filter.attack.process", "sh -c \"echo filter > '"+marker+"'\"")
	git(t, m, s.Folder, "config", "core.fsmonitor", "sh -c \"echo monitor > '"+marker+"'\"")
	git(t, m, s.Folder, "config", "diff.external", "sh -c \"echo diff > '"+marker+"'\"")
	git(t, m, s.Folder, "config", "commit.gpgsign", "true")
	git(t, m, s.Folder, "config", "gpg.program", "sh -c \"echo signer > '"+marker+"'\"")
	put(t, s.Folder, ".gitattributes", "filtered.txt filter=attack\n")
	put(t, s.Folder, "filtered.txt", "never invoke filter\n")
	put(t, s.Folder, "normal.txt", "changed\n")
	status, err := m.Status(t.Context(), s)
	if err != nil {
		t.Fatal(err)
	}
	blocked := false
	for _, file := range status.Files {
		if file.Path == "filtered.txt" && file.Blocked != "" {
			blocked = true
		}
	}
	if !blocked {
		t.Fatal("filtered path not blocked")
	}
	perform(t, m, s, "stage", "", "normal.txt")
	perform(t, m, s, "commit", "hooks are not executed")
	if _, err := os.Stat(filepath.Join(s.Folder, "executed")); !os.IsNotExist(err) {
		t.Fatal("a Git filter or hook executed")
	}
}

func TestConcurrentIndexChangeInvalidatesCommit(t *testing.T) {
	m, s := fixture(t)
	put(t, s.Folder, "one.txt", "one\n")
	perform(t, m, s, "stage", "", "one.txt")
	status, _ := m.Status(context.Background(), s)
	plan, err := m.Prepare(t.Context(), s, status.Version, "commit", "reviewed message", nil)
	if err != nil {
		t.Fatal(err)
	}
	put(t, s.Folder, "two.txt", "new\n")
	git(t, m, s.Folder, "add", "two.txt")
	if _, err = m.Execute(t.Context(), s, plan.Token); err == nil {
		t.Fatal("changed index committed without fresh review")
	}
	if got := git(t, m, s.Folder, "diff", "--cached", "--name-only"); !strings.Contains(got, "one.txt") || !strings.Contains(got, "two.txt") {
		t.Fatal("external staging was lost")
	}
}

func TestAlternateObjectStoresAreRejected(t *testing.T) {
	m, s := fixture(t)
	put(t, s.Folder, ".git/objects/info/alternates", strings.Repeat("outside\n", 1000))
	if _, err := m.Status(t.Context(), s); err == nil {
		t.Fatal("unreadable alternate store configuration was ignored")
	}
}
