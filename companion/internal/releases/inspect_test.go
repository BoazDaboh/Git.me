package releases

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func put(t *testing.T, root, name, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}
func TestReadOnlyInspectionAndMonorepoBounds(t *testing.T) {
	root := t.TempDir()
	service := filepath.Join(root, "web")
	if err := os.Mkdir(service, 0700); err != nil {
		t.Fatal(err)
	}
	sha := strings.Repeat("a", 40)
	put(t, root, ".git/HEAD", "ref: refs/heads/feat/release\n")
	put(t, root, ".git/packed-refs", sha+" refs/heads/feat/release\n")
	put(t, root, ".git/config", "[remote \"origin\"]\nurl = https://person:PRIVATE_KEY@github.com/example/repo.git?private=PRIVATE_KEY\n[core]\nfsmonitor = do-not-execute\n")
	put(t, root, ".env", strings.Repeat("b", 40))
	put(t, service, "package.json", `{"scripts":{"dev":"DO_NOT_RUN"}}`)
	if runtime.GOOS == "windows" {
		put(t, root, "tools/git.cmd", "@echo EXECUTED>"+filepath.Join(root, "executed")+"\r\n")
		put(t, root, "tools/vercel.cmd", "@echo EXECUTED>"+filepath.Join(root, "executed")+"\r\n")
		t.Setenv("PATH", filepath.Join(root, "tools"))
		t.Setenv("PATHEXT", ".CMD;.EXE")
	}
	result := Inspect(t.Context(), root, service)
	if !result.FolderReady || result.RepositoryFolder != root || result.Git.Commit != sha || result.Git.Branch != "feat/release" {
		t.Fatal("parent repository inspection failed", result)
	}
	if runtime.GOOS == "windows" && (!result.GitLocated || !result.VercelLocated) {
		t.Fatal("installed-tool presence was not detected")
	}
	if _, err := os.Stat(filepath.Join(root, "executed")); !os.IsNotExist(err) {
		t.Fatal("tool or project command was executed")
	}
	raw, _ := json.Marshal(result)
	if strings.Contains(string(raw), "PRIVATE_KEY") || strings.Contains(string(raw), "DO_NOT_RUN") {
		t.Fatal("private config or source contents escaped")
	}
	outside := t.TempDir()
	child := filepath.Join(outside, "service")
	os.Mkdir(child, 0700)
	put(t, outside, ".git/HEAD", sha)
	if other := Inspect(t.Context(), root, child); !other.FolderReady || other.Git.Found {
		t.Fatal("inspection climbed outside the saved project boundary")
	}
	if missing := Inspect(t.Context(), root, filepath.Join(root, "absent")); missing.FolderReady || missing.FolderError == "" {
		t.Fatal("missing folder was treated as available")
	}
	put(t, service, ".git/HEAD", "ref: refs/heads/../../.env\n")
	if other := Inspect(t.Context(), root, service); other.Git.Commit != "" {
		t.Fatal("unsafe reference read a file outside Git metadata")
	}
}

func TestWorkflowConfigurationValidation(t *testing.T) {
	for _, route := range []string{"github", "vercel-remote", "vercel-local"} {
		if err := Validate(json.RawMessage(`{"route":"` + route + `","target":"preview","branch":"feature/example"}`)); err != nil {
			t.Fatal(err)
		}
	}
	for _, raw := range []string{`null`, `{}`, `{"route":"shell","target":"preview","branch":"main"}`, `{"route":"github","target":"live","branch":"main"}`, `{"route":"github","target":["preview"],"branch":"main"}`, `{"route":"github","target":"preview","branch":"../main"}`, `{"route":"github","target":"preview","branch":"main","command":"git push"}`} {
		if err := Validate(json.RawMessage(raw)); err == nil {
			t.Fatal("invalid or executable configuration accepted")
		}
	}
}
