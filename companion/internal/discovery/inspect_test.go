package discovery

import (
	"strings"
	"testing"
)

func TestGitHeadLoosePackedAndDetached(t *testing.T) {
	root := t.TempDir()
	sha := strings.Repeat("a", 40)
	fixture(t, root, ".git/HEAD", "ref: refs/heads/main\n")
	fixture(t, root, ".git/refs/heads/main", sha+"\n")
	got, err := InspectGit(t.Context(), root)
	if err != nil || got.Commit != sha || got.Detached {
		t.Fatal("loose HEAD not resolved")
	}
	sha256 := strings.Repeat("b", 64)
	fixture(t, root, ".git/HEAD", sha256+"\n")
	got, err = InspectGit(t.Context(), root)
	if err != nil || !got.Detached || got.Commit != sha256 {
		t.Fatal("detached SHA-256 HEAD not detected")
	}
}
