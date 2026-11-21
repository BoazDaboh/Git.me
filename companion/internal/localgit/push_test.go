package localgit

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func pushFixture(t *testing.T) (*Manager, Selection, string, *int) {
	t.Helper()
	m, s := fixture(t)
	s.RepositoryURL = "https://github.com/fixture-user/repository"
	s.GitHubRepository = "fixture-user/repository"
	s.GitHubConnection = "fixture-account"
	git(t, m, s.Folder, "remote", "add", "origin", s.RepositoryURL)
	remote := t.TempDir()
	git(t, m, remote, "init", "--bare", "-b", "main")
	writes := new(int)
	m.networkRunner = func(ctx context.Context, p *pushSession, args ...string) ([]byte, error) {
		if args[0] == "push" {
			*writes++
		}
		copyArgs := append([]string{"-c", "protocol.file.allow=always"}, args...)
		for i, v := range copyArgs {
			if v == s.RepositoryURL+".git" {
				copyArgs[i] = remote
			}
		}
		return p.run(ctx, copyArgs...)
	}
	put(t, s.Folder, "file.txt", "first\n")
	git(t, m, s.Folder, "add", "file.txt")
	git(t, m, s.Folder, "commit", "-m", "feat: first")
	return m, s, remote, writes
}
func TestReviewedPushNewBranchAndUpdates(t *testing.T) {
	m, s, remote, writes := pushFixture(t)
	marker := filepath.Join(t.TempDir(), "must-not-run")
	git(t, m, s.Folder, "config", "credential.helper", "!echo unsafe > "+filepath.ToSlash(marker))
	git(t, m, s.Folder, "config", "http.extraHeader", "PRIVATE_SENTINEL=never-forward")
	git(t, m, s.Folder, "config", "url.https://evil.example/.insteadOf", "https://github.com/")
	put(t, s.Folder, ".git/hooks/pre-push", "#!/bin/sh\necho unsafe > '"+filepath.ToSlash(marker)+"'\n")
	plan, err := m.PreparePush(t.Context(), s)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Commits) != 1 || plan.RemoteHead != "" || plan.Token == "" || *writes != 0 || len(plan.Files) != 1 || plan.Files[0] != "file.txt" {
		t.Fatal("incorrect new branch review")
	}
	if _, err = os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("source helper ran")
	}
	result, err := m.ExecutePush(t.Context(), s, plan.Token)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Verified || *writes != 1 || strings.TrimSpace(git(t, m, remote, "rev-parse", "refs/heads/main")) != plan.Head {
		t.Fatal("reviewed commit was not pushed")
	}
	if _, err = m.ExecutePush(t.Context(), s, plan.Token); err == nil || *writes != 1 {
		t.Fatal("push review replay accepted")
	}
	if _, err = os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("hook/helper ran")
	}
	upToDate, err := m.PreparePush(t.Context(), s)
	if err != nil || len(upToDate.Commits) != 0 || upToDate.Token != "" {
		t.Fatal("up-to-date branch offered a write", err)
	}
	put(t, s.Folder, "file.txt", "second\n")
	git(t, m, s.Folder, "add", "file.txt")
	git(t, m, s.Folder, "commit", "-m", "feat: second")
	put(t, s.Folder, "uncommitted.txt", "not part of push\n")
	plan, err = m.PreparePush(t.Context(), s)
	if err != nil || len(plan.Commits) != 1 || plan.Uncommitted != 1 {
		t.Fatal("incremental review", err)
	}
	result, err = m.ExecutePush(t.Context(), s, plan.Token)
	if err != nil || !result.Verified {
		t.Fatal("incremental push", err)
	}
	if got := git(t, m, remote, "ls-tree", "--name-only", "main"); strings.Contains(got, "uncommitted") {
		t.Fatal("working file uploaded")
	}
}
func TestPushRejectsStaleCanceledExpiredAndWrongService(t *testing.T) {
	m, s, _, writes := pushFixture(t)
	plan, err := m.PreparePush(t.Context(), s)
	if err != nil {
		t.Fatal(err)
	}
	m.CancelPush(s, plan.Token)
	if _, err = m.ExecutePush(t.Context(), s, plan.Token); err == nil {
		t.Fatal("canceled plan executed")
	}
	plan, err = m.PreparePush(t.Context(), s)
	if err != nil {
		t.Fatal(err)
	}
	m.mu.Lock()
	r := m.pushPlans[plan.Token]
	r.ExpiresAt = time.Now().Add(-time.Second)
	m.pushPlans[plan.Token] = r
	m.mu.Unlock()
	if _, err = m.ExecutePush(t.Context(), s, plan.Token); err == nil {
		t.Fatal("expired plan executed")
	}
	plan, err = m.PreparePush(t.Context(), s)
	if err != nil {
		t.Fatal(err)
	}
	other := s
	other.ServiceID = "other"
	if _, err = m.ExecutePush(t.Context(), other, plan.Token); err == nil {
		t.Fatal("other service accepted")
	}
	plan, err = m.PreparePush(t.Context(), s)
	if err != nil {
		t.Fatal(err)
	}
	git(t, m, s.Folder, "commit", "--allow-empty", "-m", "new unreviewed commit")
	if _, err = m.ExecutePush(t.Context(), s, plan.Token); err == nil {
		t.Fatal("changed HEAD accepted")
	}
	if *writes != 0 {
		t.Fatal("stale review pushed")
	}
}
func TestPushRejectsDestinationAndPrivateHistory(t *testing.T) {
	m, s, _, writes := pushFixture(t)
	for _, raw := range []string{"git@github.com:fixture-user/repository.git", "https://evil.example/repo", "https://token@github.com/fixture-user/repository", "https://github.com/fixture-user/repository?token=secret", "https://github.com/fixture-user/repository#x", "https://github.com/../repository"} {
		git(t, m, s.Folder, "config", "remote.origin.url", raw)
		if _, err := m.PreparePush(t.Context(), s); err == nil {
			t.Fatal("unsafe remote accepted")
		}
	}
	git(t, m, s.Folder, "config", "remote.origin.url", s.RepositoryURL)
	git(t, m, s.Folder, "config", "remote.origin.pushurl", "https://github.com/other/repo")
	if _, err := m.PreparePush(t.Context(), s); err == nil {
		t.Fatal("different push URL accepted")
	}
	git(t, m, s.Folder, "config", "--unset", "remote.origin.pushurl")
	bad := s
	bad.GitHubRepository = "other/repository"
	if _, err := m.PreparePush(t.Context(), bad); err == nil {
		t.Fatal("mismatched service link accepted")
	}
	put(t, s.Folder, ".env", "PRIVATE_SENTINEL=secret")
	git(t, m, s.Folder, "add", "-f", ".env")
	git(t, m, s.Folder, "commit", "-m", "private file in history")
	git(t, m, s.Folder, "rm", ".env")
	git(t, m, s.Folder, "commit", "-m", "remove private file")
	_, err := m.PreparePush(t.Context(), s)
	if err == nil || !strings.Contains(err.Error(), "private or generated") || strings.Contains(err.Error(), "SENTINEL") {
		t.Fatal("private history not blocked", err)
	}
	if *writes != 0 {
		t.Fatal("destination/private review wrote remotely")
	}
}
func TestPushRejectsRemoteChangesAndDoesNotForce(t *testing.T) {
	m, s, remote, writes := pushFixture(t)
	first, err := m.PreparePush(t.Context(), s)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = m.ExecutePush(t.Context(), s, first.Token); err != nil {
		t.Fatal(err)
	}
	base := first.Head
	git(t, m, s.Folder, "commit", "--allow-empty", "-m", "second")
	plan, err := m.PreparePush(t.Context(), s)
	if err != nil {
		t.Fatal(err)
	}
	git(t, m, remote, "update-ref", "-d", "refs/heads/main")
	if _, err = m.ExecutePush(t.Context(), s, plan.Token); err == nil {
		t.Fatal("changed remote accepted")
	}
	git(t, m, remote, "update-ref", "refs/heads/main", base)
	git(t, m, s.Folder, "checkout", "-b", "divergent", base)
	git(t, m, s.Folder, "commit", "--allow-empty", "-m", "remote-only change")
	divergent := strings.TrimSpace(git(t, m, s.Folder, "rev-parse", "HEAD"))
	git(t, m, s.Folder, "checkout", "main")
	// Transfer the divergent fixture commit to the temporary bare repository only.
	git(t, m, s.Folder, "-c", "core.hooksPath="+m.hooks, "push", remote, divergent+":refs/heads/divergent")
	original := m.networkRunner
	m.networkRunner = func(ctx context.Context, p *pushSession, args ...string) ([]byte, error) {
		if args[0] == "push" {
			git(t, m, remote, "update-ref", "refs/heads/main", divergent)
		}
		return original(ctx, p, args...)
	}
	plan, err = m.PreparePush(t.Context(), s)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = m.ExecutePush(t.Context(), s, plan.Token); err == nil {
		t.Fatal("divergent race was forced")
	}
	if strings.TrimSpace(git(t, m, remote, "rev-parse", "main")) != divergent {
		t.Fatal("remote history overwritten")
	}
	m.networkRunner = original
	if _, err = m.PreparePush(t.Context(), s); err == nil {
		t.Fatal("diverged review accepted")
	}
	if *writes != 2 {
		t.Fatal("unexpected push attempts")
	}
}
func TestPushErrorsDoNotExposeGitOutput(t *testing.T) {
	m, s, _, _ := pushFixture(t)
	m.networkRunner = func(context.Context, *pushSession, ...string) ([]byte, error) {
		return nil, errors.New("PRIVATE_TOKEN_SENTINEL")
	}
	_, err := m.PreparePush(t.Context(), s)
	if err == nil || strings.Contains(err.Error(), "SENTINEL") {
		t.Fatal("raw network error leaked")
	}
}
