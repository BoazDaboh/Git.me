package localupload

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSourceExclusionsAndImmutableReview(t *testing.T) {
	root := t.TempDir()
	write := func(p, body string) {
		t.Helper()
		full := filepath.Join(root, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	for p, body := range map[string]string{"package.json": "{}", "src/page.tsx": "export default 1;", "public/logo.svg": "<svg/>", ".env.local": "SENTINEL_ENV_VALUE", ".npmrc": "SENTINEL_REGISTRY_TOKEN", ".git/config": "SENTINEL_GIT_TOKEN", "node_modules/mod/index.js": "dependency", ".next/cache/data": "generated", ".gitignore": "private/\n*.scratch\n!.env.local\n!private/\n", ".vercelignore": "src/omit.ts\n!src/omit.ts\n", "private/data.txt": "not public", "x.scratch": "ignored", "src/omit.ts": "ignored", "src/nested/.gitignore": "hidden.txt\n", "src/nested/hidden.txt": "nested ignore", "src/config.ts": "export const access_token = '" + strings.Repeat("x", 24) + "';", "db.sqlite": "private db"} {
		write(p, body)
	}
	source, err := Read(t.Context(), root, root)
	if err != nil {
		t.Fatal(err)
	}
	paths := map[string]bool{}
	for _, f := range source.Files {
		paths[f.Path] = true
		if len(f.SHA) != 40 || f.Size != len(f.Data) {
			t.Fatal("bad manifest")
		}
	}
	for _, p := range []string{"package.json", "src/page.tsx", "public/logo.svg"} {
		if !paths[p] {
			t.Fatal("source missing", p)
		}
	}
	for _, p := range []string{".env.local", ".npmrc", ".git/config", "node_modules/mod/index.js", "private/data.txt", "src/omit.ts", "src/nested/hidden.txt", "src/config.ts", "db.sqlite", "x.scratch"} {
		if paths[p] {
			t.Fatal("private file included", p)
		}
	}
	raw, _ := json.Marshal(source)
	if strings.Contains(string(raw), "SENTINEL_") || strings.Contains(string(raw), "export default") {
		t.Fatal("file contents exposed in review")
	}
	write(".env.local", "a changed private value")
	same, err := Read(t.Context(), root, root)
	if err != nil || same.Digest != source.Digest {
		t.Fatal("ignored changes altered source")
	}
	write("src/page.tsx", "export default 2;")
	changed, err := Read(t.Context(), root, root)
	if err != nil || changed.Digest == source.Digest {
		t.Fatal("source edit not detected")
	}
	write("src/new.ts", "new source")
	added, _ := Read(t.Context(), root, root)
	if added.Digest == changed.Digest {
		t.Fatal("added source not detected")
	}
	if _, err = Read(t.Context(), root, t.TempDir()); err == nil {
		t.Fatal("escaped service scope")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err = Read(ctx, root, root); err == nil {
		t.Fatal("cancel ignored")
	}
}
func TestSourceRejectsLargeFilesAndLinkedIgnore(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "index.js"), make([]byte, MaxFileBytes+1), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(t.Context(), root, root); err == nil {
		t.Fatal("oversized file accepted")
	}
	if err := os.WriteFile(filepath.Join(root, "index.js"), []byte("source"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte(`bad\pattern`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(t.Context(), root, root); err == nil {
		t.Fatal("unsupported ignore silently accepted")
	}
	if err := os.Remove(filepath.Join(root, ".gitignore")); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "data.txt")
	os.WriteFile(outside, []byte("SENTINEL_LINK_CONTENT"), 0600)
	if err := os.Symlink(outside, filepath.Join(root, "linked.txt")); err != nil {
		t.Log("symlink creation unavailable")
		return
	}
	source, err := Read(t.Context(), root, root)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range source.Files {
		if f.Path == "linked.txt" {
			t.Fatal("followed outside link")
		}
	}
	if err := os.Symlink(outside, filepath.Join(root, ".gitignore")); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(t.Context(), root, root); err == nil {
		t.Fatal("linked ignore accepted")
	}
}
func TestBoundedIgnoreMatching(t *testing.T) {
	for _, test := range []struct {
		pattern, name string
		want          bool
	}{{"**/draft/*.ts", "draft/a.ts", true}, {"**/draft/*.ts", "a/b/draft/a.ts", true}, {"**/draft/*.ts", "a/b/draft/a.js", false}, {"/**/draft/", "a/draft", true}, {"/root.txt", "a/root.txt", false}, {"*.log", "a/x.log", true}, {"x/**/**/**/z", "x/a/b/c/q", false}} {
		rs, err := rules([]byte(test.pattern), "")
		if err != nil {
			t.Fatal(err)
		}
		if got := ignored(rs, test.name, strings.HasSuffix(test.pattern, "/")); got != test.want {
			t.Fatal(test, got)
		}
	}
}
